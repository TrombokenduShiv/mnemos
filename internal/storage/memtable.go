package storage

import (
	"sort"
	"sync"
	"sync/atomic"
)

// Memtable is an in-memory sorted key-value store that holds recent writes.
// Once it exceeds a size threshold, it is frozen (made read-only) and flushed
// to an immutable SSTable while a new memtable accepts further writes.
type Memtable struct {
	mu       sync.RWMutex
	data     map[string]memEntry
	keys     []string // maintained in sorted order
	size     int64    // approximate byte size of all stored data
	frozen   atomic.Bool
	seqNum   uint64
}

// memEntry represents a key-value pair in the memtable, including tombstones.
type memEntry struct {
	Value     []byte
	Timestamp int64
	Deleted   bool // true = tombstone
}

// NewMemtable creates a new empty memtable.
func NewMemtable() *Memtable {
	return &Memtable{
		data: make(map[string]memEntry, 1024),
		keys: make([]string, 0, 1024),
	}
}

// Put inserts or updates a key-value pair in the memtable.
func (m *Memtable) Put(key, value []byte, timestamp int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := string(key)
	old, exists := m.data[k]

	entry := memEntry{
		Value:     make([]byte, len(value)),
		Timestamp: timestamp,
		Deleted:   false,
	}
	copy(entry.Value, value)

	m.data[k] = entry

	if !exists {
		// Insert into sorted keys slice
		idx := sort.SearchStrings(m.keys, k)
		m.keys = append(m.keys, "")
		copy(m.keys[idx+1:], m.keys[idx:])
		m.keys[idx] = k
		m.size += int64(len(key) + len(value))
	} else {
		// Update: adjust size delta
		m.size += int64(len(value) - len(old.Value))
	}

	m.seqNum++
}

// Delete inserts a tombstone for the given key.
func (m *Memtable) Delete(key []byte, timestamp int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	k := string(key)
	_, exists := m.data[k]

	entry := memEntry{
		Value:     nil,
		Timestamp: timestamp,
		Deleted:   true,
	}
	m.data[k] = entry

	if !exists {
		idx := sort.SearchStrings(m.keys, k)
		m.keys = append(m.keys, "")
		copy(m.keys[idx+1:], m.keys[idx:])
		m.keys[idx] = k
		m.size += int64(len(key))
	}

	m.seqNum++
}

// Get retrieves a value by key. Returns the value, whether it was found,
// and whether it's a tombstone (deleted).
func (m *Memtable) Get(key []byte) ([]byte, bool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.data[string(key)]
	if !ok {
		return nil, false, false
	}
	if entry.Deleted {
		return nil, true, true
	}
	result := make([]byte, len(entry.Value))
	copy(result, entry.Value)
	return result, true, false
}

// Size returns the approximate byte size of the memtable.
func (m *Memtable) Size() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.size
}

// Len returns the number of entries in the memtable.
func (m *Memtable) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// Freeze marks the memtable as read-only. After freezing, no new writes
// should be applied (callers are expected to check IsFrozen before writing).
func (m *Memtable) Freeze() {
	m.frozen.Store(true)
}

// IsFrozen returns true if the memtable has been frozen for flushing.
func (m *Memtable) IsFrozen() bool {
	return m.frozen.Load()
}

// Iterator returns all entries in sorted key order for flushing to an SSTable.
// The memtable should be frozen before calling this.
type MemIterator struct {
	keys    []string
	data    map[string]memEntry
	pos     int
}

// Entries returns a MemIterator over all entries in sorted order.
func (m *Memtable) Entries() *MemIterator {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Snapshot the keys
	keys := make([]string, len(m.keys))
	copy(keys, m.keys)

	return &MemIterator{
		keys: keys,
		data: m.data,
		pos:  0,
	}
}

// Next advances the iterator and returns the next key, value, deleted flag, and whether valid.
func (it *MemIterator) Next() (key string, value []byte, deleted bool, ok bool) {
	if it.pos >= len(it.keys) {
		return "", nil, false, false
	}
	k := it.keys[it.pos]
	entry := it.data[k]
	it.pos++
	return k, entry.Value, entry.Deleted, true
}

// Reset resets the iterator to the beginning.
func (it *MemIterator) Reset() {
	it.pos = 0
}
