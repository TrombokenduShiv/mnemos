// Package storage implements a complete log-structured storage engine.
// Engine is the top-level orchestrator that coordinates WAL, Memtable,
// SSTable, and Compaction into a coherent, crash-safe key-value store.
package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultMemtableThreshold is the byte size at which a memtable triggers a flush.
	DefaultMemtableThreshold int64 = 4 * 1024 * 1024 // 4 MB
	// DefaultCompactionThreshold is the number of L0 SSTables that trigger compaction.
	DefaultCompactionThreshold = 4
)

// EngineConfig holds configurable parameters for the storage engine.
type EngineConfig struct {
	DataDir              string
	MemtableThreshold    int64
	CompactionThreshold  int
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig(dataDir string) EngineConfig {
	return EngineConfig{
		DataDir:             dataDir,
		MemtableThreshold:   DefaultMemtableThreshold,
		CompactionThreshold: DefaultCompactionThreshold,
	}
}

// Manifest tracks the list of active SSTable files. It is persisted to disk
// so the engine can recover which SSTables are live after a restart.
type Manifest struct {
	SSTables []string `json:"sstables"` // ordered oldest → newest
	SeqNum   int      `json:"seq_num"`  // next SSTable sequence number
}

// Engine is the top-level storage engine that orchestrates all components.
type Engine struct {
	config    EngineConfig
	wal       *WAL
	memtable  *Memtable
	frozen    []*Memtable // frozen memtables awaiting flush
	sstables  []*SSTableReader
	manifest  Manifest
	compactor *Compactor

	mu        sync.RWMutex // protects memtable, frozen, sstables, manifest
	flushMu   sync.Mutex   // serializes flushes
	closed    atomic.Bool

	stats     EngineStats
}

// EngineStats tracks runtime statistics.
type EngineStats struct {
	PutsTotal       atomic.Int64
	GetsTotal       atomic.Int64
	DeletesTotal    atomic.Int64
	FlushesTotal    atomic.Int64
	CompactionsTotal atomic.Int64
	BytesWritten    atomic.Int64
}

// Stats returns a snapshot of engine statistics.
func (e *Engine) Stats() map[string]int64 {
	return map[string]int64{
		"puts_total":        e.stats.PutsTotal.Load(),
		"gets_total":        e.stats.GetsTotal.Load(),
		"deletes_total":     e.stats.DeletesTotal.Load(),
		"flushes_total":     e.stats.FlushesTotal.Load(),
		"compactions_total": e.stats.CompactionsTotal.Load(),
		"bytes_written":     e.stats.BytesWritten.Load(),
		"memtable_size":     e.memtable.Size(),
		"memtable_entries":  int64(e.memtable.Len()),
		"sstable_count":     int64(len(e.sstables)),
	}
}

// NewEngine creates and opens a storage engine at the specified directory.
// If the directory contains a previous engine's data, it performs crash recovery.
func NewEngine(config EngineConfig) (*Engine, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("engine: create data dir: %w", err)
	}

	e := &Engine{
		config:    config,
		memtable:  NewMemtable(),
		frozen:    make([]*Memtable, 0),
		sstables:  make([]*SSTableReader, 0),
		compactor: NewCompactor(config.DataDir),
	}

	// Load or create manifest
	if err := e.loadManifest(); err != nil {
		return nil, fmt.Errorf("engine: load manifest: %w", err)
	}

	// Open existing SSTables
	var validSSTables []string
	for _, name := range e.manifest.SSTables {
		path := filepath.Join(config.DataDir, filepath.Base(name))
		reader, err := OpenSSTable(path)
		if err != nil {
			// Log warning but continue — the SSTable may have been partially written or deleted
			fmt.Fprintf(os.Stderr, "warning: could not open SSTable %s: %v\n", path, err)
			continue
		}
		e.sstables = append(e.sstables, reader)
		validSSTables = append(validSSTables, filepath.Base(name))
	}
	e.manifest.SSTables = validSSTables

	// Open or create WAL
	walPath := filepath.Join(config.DataDir, "wal.log")
	wal, err := NewWAL(walPath)
	if err != nil {
		return nil, fmt.Errorf("engine: open WAL: %w", err)
	}
	e.wal = wal

	// Replay WAL for crash recovery
	replayed, err := e.wal.Replay(func(rec WALRecord) error {
		switch rec.Op {
		case OpPut:
			e.memtable.Put(rec.Key, rec.Value, rec.Timestamp)
		case OpDelete:
			e.memtable.Delete(rec.Key, rec.Timestamp)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("engine: WAL replay: %w", err)
	}
	if replayed > 0 {
		fmt.Fprintf(os.Stderr, "engine: recovered %d records from WAL\n", replayed)
	}

	return e, nil
}

// Put stores a key-value pair. The write is durable once this returns.
func (e *Engine) Put(key, value []byte) error {
	if e.closed.Load() {
		return fmt.Errorf("engine: closed")
	}

	// 1. Write to WAL first (durability guarantee)
	if err := e.wal.AppendPut(key, value); err != nil {
		return fmt.Errorf("engine: WAL put: %w", err)
	}

	// 2. Apply to memtable
	e.mu.Lock()
	e.memtable.Put(key, value, time.Now().UnixNano())
	memSize := e.memtable.Size()
	e.mu.Unlock()

	e.stats.PutsTotal.Add(1)
	e.stats.BytesWritten.Add(int64(len(key) + len(value)))

	// 3. Check if memtable needs flushing
	if memSize >= e.config.MemtableThreshold {
		go e.triggerFlush()
	}

	return nil
}

// Get retrieves the value for a key. Returns value, found.
// Checks memtable first, then frozen memtables, then SSTables (newest first).
func (e *Engine) Get(key []byte) ([]byte, bool, error) {
	if e.closed.Load() {
		return nil, false, fmt.Errorf("engine: closed")
	}

	e.stats.GetsTotal.Add(1)

	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Check active memtable
	value, found, deleted := e.memtable.Get(key)
	if found {
		if deleted {
			return nil, false, nil // Tombstone — key was deleted
		}
		return value, true, nil
	}

	// 2. Check frozen memtables (newest first)
	for i := len(e.frozen) - 1; i >= 0; i-- {
		value, found, deleted = e.frozen[i].Get(key)
		if found {
			if deleted {
				return nil, false, nil
			}
			return value, true, nil
		}
	}

	// 3. Check SSTables (newest first)
	for i := len(e.sstables) - 1; i >= 0; i-- {
		value, found, deleted, err := e.sstables[i].Get(string(key))
		if err != nil {
			return nil, false, fmt.Errorf("engine: SSTable get: %w", err)
		}
		if found {
			if deleted {
				return nil, false, nil
			}
			return value, true, nil
		}
	}

	return nil, false, nil
}

// Delete inserts a tombstone for the given key.
func (e *Engine) Delete(key []byte) error {
	if e.closed.Load() {
		return fmt.Errorf("engine: closed")
	}

	if err := e.wal.AppendDelete(key); err != nil {
		return fmt.Errorf("engine: WAL delete: %w", err)
	}

	e.mu.Lock()
	e.memtable.Delete(key, time.Now().UnixNano())
	e.mu.Unlock()

	e.stats.DeletesTotal.Add(1)
	return nil
}

// triggerFlush freezes the current memtable and flushes it to an SSTable.
func (e *Engine) triggerFlush() {
	e.flushMu.Lock()
	defer e.flushMu.Unlock()

	e.mu.Lock()

	// Double-check the threshold under lock
	if e.memtable.Size() < e.config.MemtableThreshold {
		e.mu.Unlock()
		return
	}

	// Freeze the current memtable and create a new one
	frozenMem := e.memtable
	frozenMem.Freeze()
	e.frozen = append(e.frozen, frozenMem)
	e.memtable = NewMemtable()

	e.mu.Unlock()

	// Flush the frozen memtable to an SSTable
	if err := e.flushMemtable(frozenMem); err != nil {
		fmt.Fprintf(os.Stderr, "engine: flush error: %v\n", err)
		return
	}

	// Check if compaction is needed
	e.mu.RLock()
	needsCompaction := len(e.sstables) >= e.config.CompactionThreshold
	e.mu.RUnlock()

	if needsCompaction {
		go e.triggerCompaction()
	}
}

// flushMemtable writes a frozen memtable to a new SSTable file.
func (e *Engine) flushMemtable(mem *Memtable) error {
	e.mu.Lock()
	seqNum := e.manifest.SeqNum
	e.manifest.SeqNum++
	e.mu.Unlock()

	sstPath := GenerateSSTablePath(e.config.DataDir, 0, seqNum)

	writer, err := NewSSTableWriter(sstPath)
	if err != nil {
		return fmt.Errorf("flush: create SSTable: %w", err)
	}

	it := mem.Entries()
	count := 0
	for {
		key, value, deleted, ok := it.Next()
		if !ok {
			break
		}
		if err := writer.Add(key, value, deleted); err != nil {
			return fmt.Errorf("flush: write record: %w", err)
		}
		count++
	}

	if err := writer.Finish(); err != nil {
		return fmt.Errorf("flush: finish SSTable: %w", err)
	}

	// Open the newly written SSTable for reading
	reader, err := OpenSSTable(sstPath)
	if err != nil {
		return fmt.Errorf("flush: open new SSTable: %w", err)
	}

	// Atomically update the engine state
	e.mu.Lock()

	// Add to sstables list
	e.sstables = append(e.sstables, reader)

	// Update manifest
	e.manifest.SSTables = append(e.manifest.SSTables, filepath.Base(sstPath))

	// Remove the frozen memtable
	newFrozen := make([]*Memtable, 0, len(e.frozen))
	for _, f := range e.frozen {
		if f != mem {
			newFrozen = append(newFrozen, f)
		}
	}
	e.frozen = newFrozen

	e.mu.Unlock()

	// Persist manifest
	if err := e.saveManifest(); err != nil {
		return fmt.Errorf("flush: save manifest: %w", err)
	}

	// Reset WAL (data is now durable in the SSTable)
	if err := e.wal.Reset(); err != nil {
		return fmt.Errorf("flush: reset WAL: %w", err)
	}

	e.stats.FlushesTotal.Add(1)
	fmt.Fprintf(os.Stderr, "engine: flushed %d records to %s\n", count, sstPath)
	return nil
}

// triggerCompaction merges L0 SSTables into a single compacted file.
func (e *Engine) triggerCompaction() {
	e.mu.RLock()
	if len(e.sstables) < e.config.CompactionThreshold {
		e.mu.RUnlock()
		return
	}

	// Collect paths and readers for all current SSTables
	paths := make([]string, len(e.manifest.SSTables))
	for i, name := range e.manifest.SSTables {
		paths[i] = filepath.Join(e.config.DataDir, name)
	}
	e.mu.RUnlock()

	// Generate output path
	e.mu.Lock()
	seqNum := e.manifest.SeqNum
	e.manifest.SeqNum++
	e.mu.Unlock()

	outputPath := GenerateSSTablePath(e.config.DataDir, 1, seqNum)

	// Close current readers before compaction
	e.mu.Lock()
	oldReaders := e.sstables
	for _, r := range oldReaders {
		r.Close()
	}
	e.sstables = nil
	e.mu.Unlock()

	// Run compaction
	compactedPath, recordsWritten, err := e.compactor.Compact(paths, outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engine: compaction error: %v\n", err)
		// Try to reopen old SSTables
		e.mu.Lock()
		for _, p := range paths {
			if r, err := OpenSSTable(p); err == nil {
				e.sstables = append(e.sstables, r)
			}
		}
		e.mu.Unlock()
		return
	}

	// Open compacted SSTable
	newReader, err := OpenSSTable(compactedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engine: open compacted SSTable: %v\n", err)
		return
	}

	// Atomic manifest swap
	e.mu.Lock()
	e.sstables = []*SSTableReader{newReader}
	e.manifest.SSTables = []string{filepath.Base(compactedPath)}
	e.mu.Unlock()

	if err := e.saveManifest(); err != nil {
		fmt.Fprintf(os.Stderr, "engine: save manifest after compaction: %v\n", err)
	}

	// Remove old SSTable files
	for _, p := range paths {
		if p != compactedPath {
			os.Remove(p)
		}
	}

	e.stats.CompactionsTotal.Add(1)
	fmt.Fprintf(os.Stderr, "engine: compacted %d SSTables into %s (%d records)\n",
		len(paths), compactedPath, recordsWritten)
}

// Compact manually triggers compaction. Called by `mnemos compact` CLI command.
func (e *Engine) Compact() error {
	e.mu.RLock()
	count := len(e.sstables)
	e.mu.RUnlock()

	if count < 2 {
		return fmt.Errorf("engine: not enough SSTables to compact (%d)", count)
	}

	e.triggerCompaction()
	return nil
}

// Flush manually triggers a memtable flush. Useful for testing and clean shutdown.
func (e *Engine) Flush() error {
	e.mu.Lock()
	if e.memtable.Len() == 0 {
		e.mu.Unlock()
		return nil
	}

	frozenMem := e.memtable
	frozenMem.Freeze()
	e.frozen = append(e.frozen, frozenMem)
	e.memtable = NewMemtable()
	e.mu.Unlock()

	return e.flushMemtable(frozenMem)
}

// Close flushes the memtable, syncs all state, and closes the engine.
func (e *Engine) Close() error {
	if e.closed.Swap(true) {
		return nil // already closed
	}

	// Flush any remaining memtable data
	if e.memtable.Len() > 0 {
		if err := e.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "engine: flush on close: %v\n", err)
		}
	}

	// Close WAL
	if err := e.wal.Close(); err != nil {
		return fmt.Errorf("engine: close WAL: %w", err)
	}

	// Close all SSTables
	e.mu.Lock()
	for _, r := range e.sstables {
		r.Close()
	}
	e.sstables = nil
	e.mu.Unlock()

	return nil
}

// Manifest persistence

func (e *Engine) manifestPath() string {
	return filepath.Join(e.config.DataDir, "MANIFEST.json")
}

func (e *Engine) loadManifest() error {
	path := e.manifestPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			e.manifest = Manifest{
				SSTables: make([]string, 0),
				SeqNum:   0,
			}
			return nil // Fresh start
		}
		return fmt.Errorf("load manifest: %w", err)
	}

	if err := json.Unmarshal(data, &e.manifest); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	return nil
}

func (e *Engine) saveManifest() error {
	e.mu.RLock()
	data, err := json.MarshalIndent(e.manifest, "", "  ")
	e.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	// Write to temp file first, then atomic rename
	tmpPath := e.manifestPath() + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := os.Rename(tmpPath, e.manifestPath()); err != nil {
		return fmt.Errorf("rename manifest: %w", err)
	}
	return nil
}

// AllKeys returns all live keys in the engine (for debugging/stats).
func (e *Engine) AllKeys() ([]string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	keySet := make(map[string]bool)
	deleted := make(map[string]bool)

	// Active memtable
	it := e.memtable.Entries()
	for {
		key, _, isDel, ok := it.Next()
		if !ok {
			break
		}
		if isDel {
			deleted[key] = true
		} else {
			keySet[key] = true
		}
	}

	// Frozen memtables
	for _, mem := range e.frozen {
		it := mem.Entries()
		for {
			key, _, isDel, ok := it.Next()
			if !ok {
				break
			}
			if _, already := keySet[key]; !already {
				if _, alreadyDel := deleted[key]; !alreadyDel {
					if isDel {
						deleted[key] = true
					} else {
						keySet[key] = true
					}
				}
			}
		}
	}

	// SSTables (newest first)
	for i := len(e.sstables) - 1; i >= 0; i-- {
		ssIt, err := e.sstables[i].NewIterator()
		if err != nil {
			return nil, err
		}
		for {
			key, _, isDel, ok, err := ssIt.Next()
			if err != nil {
				return nil, err
			}
			if !ok {
				break
			}
			if _, already := keySet[key]; !already {
				if _, alreadyDel := deleted[key]; !alreadyDel {
					if isDel {
						deleted[key] = true
					} else {
						keySet[key] = true
					}
				}
			}
		}
	}

	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

// DataDir returns the data directory path.
func (e *Engine) DataDir() string {
	return e.config.DataDir
}
