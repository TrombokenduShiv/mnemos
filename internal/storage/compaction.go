package storage

import (
	"container/heap"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Compactor merges multiple SSTables into fewer, larger ones,
// dropping obsolete versions and resolved tombstones to bound
// read amplification and reclaim disk space.
type Compactor struct {
	mu      sync.Mutex
	dataDir string
}

// NewCompactor creates a compactor for the given data directory.
func NewCompactor(dataDir string) *Compactor {
	return &Compactor{dataDir: dataDir}
}

// compactHeapItem is a single item in the merge priority queue.
type compactHeapItem struct {
	key     string
	value   []byte
	deleted bool
	tableID int // which SSTable this came from
}

// compactHeap implements container/heap for k-way merge.
type compactHeap []compactHeapItem

func (h compactHeap) Len() int           { return len(h) }
func (h compactHeap) Less(i, j int) bool {
	if h[i].key == h[j].key {
		return h[i].tableID < h[j].tableID
	}
	return h[i].key < h[j].key
}
func (h compactHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *compactHeap) Push(x interface{}) {
	*h = append(*h, x.(compactHeapItem))
}

func (h *compactHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// Compact merges the given SSTable files into a single new SSTable.
// Returns the path to the new SSTable file.
// The caller is responsible for swapping the manifest and removing old files.
func (c *Compactor) Compact(sstPaths []string, outputPath string) (string, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(sstPaths) == 0 {
		return "", 0, fmt.Errorf("compaction: no SSTables to compact")
	}

	if len(sstPaths) == 1 {
		// Nothing to merge — just return the existing file
		return sstPaths[0], 0, nil
	}

	// Open all SSTables and create iterators
	readers := make([]*SSTableReader, len(sstPaths))
	iterators := make([]*SSTableIterator, len(sstPaths))
	defer func() {
		for _, r := range readers {
			if r != nil {
				r.Close()
			}
		}
	}()

	for i, path := range sstPaths {
		r, err := OpenSSTable(path)
		if err != nil {
			return "", 0, fmt.Errorf("compaction: open %s: %w", path, err)
		}
		readers[i] = r

		it, err := r.NewIterator()
		if err != nil {
			return "", 0, fmt.Errorf("compaction: iterator %s: %w", path, err)
		}
		iterators[i] = it
	}

	// Initialize the merge heap with the first record from each SSTable
	h := &compactHeap{}
	heap.Init(h)

	for i, it := range iterators {
		key, value, deleted, ok, err := it.Next()
		if err != nil {
			return "", 0, fmt.Errorf("compaction: read first record from table %d: %w", i, err)
		}
		if ok {
			heap.Push(h, compactHeapItem{
				key:     key,
				value:   value,
				deleted: deleted,
				tableID: i,
			})
		}
	}

	// Create output SSTable
	writer, err := NewSSTableWriter(outputPath)
	if err != nil {
		return "", 0, fmt.Errorf("compaction: create output: %w", err)
	}

	recordsWritten := 0
	var lastKey string
	var lastValue []byte
	var lastDeleted bool
	firstRecord := true

	for h.Len() > 0 {
		item := heap.Pop(h).(compactHeapItem)

		// Advance the iterator that produced this item
		key, value, deleted, ok, err := iterators[item.tableID].Next()
		if err != nil {
			return "", 0, fmt.Errorf("compaction: read next from table %d: %w", item.tableID, err)
		}
		if ok {
			heap.Push(h, compactHeapItem{
				key:     key,
				value:   value,
				deleted: deleted,
				tableID: item.tableID,
			})
		}

		if firstRecord {
			lastKey = item.key
			lastValue = item.value
			lastDeleted = item.deleted
			firstRecord = false
			continue
		}

		if item.key == lastKey {
			// Duplicate key — keep the one from the newer (higher-index) table
			// Since we process SSTables from oldest to newest, later duplicates win
			lastValue = item.value
			lastDeleted = item.deleted
			continue
		}

		// Write the previous key (skip tombstones during compaction to reclaim space)
		if !lastDeleted {
			if err := writer.Add(lastKey, lastValue, false); err != nil {
				return "", 0, fmt.Errorf("compaction: write record: %w", err)
			}
			recordsWritten++
		}

		lastKey = item.key
		lastValue = item.value
		lastDeleted = item.deleted
	}

	// Write the final key
	if !firstRecord && !lastDeleted {
		if err := writer.Add(lastKey, lastValue, false); err != nil {
			return "", 0, fmt.Errorf("compaction: write final record: %w", err)
		}
		recordsWritten++
	}

	if err := writer.Finish(); err != nil {
		return "", 0, fmt.Errorf("compaction: finish: %w", err)
	}

	return outputPath, recordsWritten, nil
}

// RemoveSSTable safely removes an SSTable file from disk.
func RemoveSSTable(path string) error {
	return os.Remove(path)
}

// GenerateSSTablePath generates a unique SSTable filename.
func GenerateSSTablePath(dataDir string, level, seq int) string {
	return filepath.Join(dataDir, fmt.Sprintf("L%d_%06d.sst", level, seq))
}
