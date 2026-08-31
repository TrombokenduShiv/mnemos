package storage

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sort"
)

// SSTable file format:
//
//   +------------------+------------------+---------+
//   |  Data Block(s)   |  Index Block     | Footer  |
//   +------------------+------------------+---------+
//
// Data Block: sequence of records, each:
//   KeyLen(4) | Key | ValueLen(4) | Value | Deleted(1) | CRC32(4)
//
// Index Block: sequence of index entries:
//   KeyLen(4) | Key | Offset(8)
//
// Footer (fixed 24 bytes):
//   IndexOffset(8) | IndexSize(8) | MagicNumber(8)

const (
	sstMagicNumber uint64 = 0x4D4E454D4F535354 // "MNEMOSST"
	footerSize            = 24
)

// SSTableWriter creates an SSTable file from sorted key-value pairs.
type SSTableWriter struct {
	file     *os.File
	writer   *bufio.Writer
	crcTable *crc32.Table
	index    []indexEntry
	offset   int64
	path     string
	count    int
}

type indexEntry struct {
	Key    string
	Offset int64
}

// NewSSTableWriter creates a new SSTable file for writing.
func NewSSTableWriter(path string) (*SSTableWriter, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("sstable: create %s: %w", path, err)
	}
	return &SSTableWriter{
		file:     file,
		writer:   bufio.NewWriterSize(file, 64*1024),
		crcTable: crc32.MakeTable(crc32.Castagnoli),
		index:    make([]indexEntry, 0, 256),
		path:     path,
	}, nil
}

// Add writes a key-value pair (or tombstone) to the SSTable.
// Keys MUST be added in sorted order.
func (w *SSTableWriter) Add(key string, value []byte, deleted bool) error {
	// Record this key's offset in the sparse index (every entry for now;
	// could be every Nth for larger files, but correctness first)
	w.index = append(w.index, indexEntry{Key: key, Offset: w.offset})

	keyBytes := []byte(key)

	// Compute CRC32 over key + value + deleted flag
	var deletedByte byte
	if deleted {
		deletedByte = 1
	}
	crcData := make([]byte, len(keyBytes)+len(value)+1)
	copy(crcData, keyBytes)
	copy(crcData[len(keyBytes):], value)
	crcData[len(crcData)-1] = deletedByte
	checksum := crc32.Checksum(crcData, w.crcTable)

	// Write: KeyLen(4) | Key | ValueLen(4) | Value | Deleted(1) | CRC32(4)
	recordSize := 4 + len(keyBytes) + 4 + len(value) + 1 + 4

	buf := make([]byte, recordSize)
	pos := 0

	binary.LittleEndian.PutUint32(buf[pos:pos+4], uint32(len(keyBytes)))
	pos += 4
	copy(buf[pos:pos+len(keyBytes)], keyBytes)
	pos += len(keyBytes)
	binary.LittleEndian.PutUint32(buf[pos:pos+4], uint32(len(value)))
	pos += 4
	copy(buf[pos:pos+len(value)], value)
	pos += len(value)
	buf[pos] = deletedByte
	pos++
	binary.LittleEndian.PutUint32(buf[pos:pos+4], checksum)

	n, err := w.writer.Write(buf)
	if err != nil {
		return fmt.Errorf("sstable: write record: %w", err)
	}
	w.offset += int64(n)
	w.count++
	return nil
}

// Finish writes the index block and footer, then flushes and closes the file.
func (w *SSTableWriter) Finish() error {
	indexOffset := w.offset

	// Write index block
	for _, entry := range w.index {
		keyBytes := []byte(entry.Key)
		buf := make([]byte, 4+len(keyBytes)+8)
		binary.LittleEndian.PutUint32(buf[0:4], uint32(len(keyBytes)))
		copy(buf[4:4+len(keyBytes)], keyBytes)
		binary.LittleEndian.PutUint64(buf[4+len(keyBytes):], uint64(entry.Offset))

		n, err := w.writer.Write(buf)
		if err != nil {
			return fmt.Errorf("sstable: write index entry: %w", err)
		}
		w.offset += int64(n)
	}

	indexSize := w.offset - indexOffset

	// Write footer: IndexOffset(8) | IndexSize(8) | MagicNumber(8)
	footer := make([]byte, footerSize)
	binary.LittleEndian.PutUint64(footer[0:8], uint64(indexOffset))
	binary.LittleEndian.PutUint64(footer[8:16], uint64(indexSize))
	binary.LittleEndian.PutUint64(footer[16:24], sstMagicNumber)

	if _, err := w.writer.Write(footer); err != nil {
		return fmt.Errorf("sstable: write footer: %w", err)
	}

	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("sstable: flush: %w", err)
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("sstable: sync: %w", err)
	}
	return w.file.Close()
}

// Count returns the number of records written.
func (w *SSTableWriter) Count() int {
	return w.count
}

// SSTableReader reads an immutable SSTable file.
type SSTableReader struct {
	path     string
	file     *os.File
	index    []indexEntry
	crcTable *crc32.Table
	size     int64
}

// OpenSSTable opens an existing SSTable file for reading.
func OpenSSTable(path string) (*SSTableReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("sstable: open %s: %w", path, err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("sstable: stat: %w", err)
	}

	if info.Size() < footerSize {
		file.Close()
		return nil, fmt.Errorf("sstable: file too small (%d bytes)", info.Size())
	}

	// Read footer
	footer := make([]byte, footerSize)
	if _, err := file.ReadAt(footer, info.Size()-int64(footerSize)); err != nil {
		file.Close()
		return nil, fmt.Errorf("sstable: read footer: %w", err)
	}

	indexOffset := int64(binary.LittleEndian.Uint64(footer[0:8]))
	indexSize := int64(binary.LittleEndian.Uint64(footer[8:16]))
	magic := binary.LittleEndian.Uint64(footer[16:24])

	if magic != sstMagicNumber {
		file.Close()
		return nil, fmt.Errorf("sstable: invalid magic number %x", magic)
	}

	// Read index block
	indexData := make([]byte, indexSize)
	if _, err := file.ReadAt(indexData, indexOffset); err != nil {
		file.Close()
		return nil, fmt.Errorf("sstable: read index: %w", err)
	}

	index, err := parseIndex(indexData)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("sstable: parse index: %w", err)
	}

	return &SSTableReader{
		path:     path,
		file:     file,
		index:    index,
		crcTable: crc32.MakeTable(crc32.Castagnoli),
		size:     info.Size(),
	}, nil
}

func parseIndex(data []byte) ([]indexEntry, error) {
	entries := make([]indexEntry, 0, 256)
	pos := 0

	for pos < len(data) {
		if pos+4 > len(data) {
			return nil, fmt.Errorf("truncated index at offset %d", pos)
		}
		keyLen := int(binary.LittleEndian.Uint32(data[pos : pos+4]))
		pos += 4

		if pos+keyLen+8 > len(data) {
			return nil, fmt.Errorf("truncated index key at offset %d", pos)
		}
		key := string(data[pos : pos+keyLen])
		pos += keyLen

		offset := int64(binary.LittleEndian.Uint64(data[pos : pos+8]))
		pos += 8

		entries = append(entries, indexEntry{Key: key, Offset: offset})
	}

	return entries, nil
}

// Get looks up a key in the SSTable using the sparse index + binary search.
// Returns value, found, deleted.
func (r *SSTableReader) Get(key string) ([]byte, bool, bool, error) {
	// Binary search the index for the correct position
	idx := sort.Search(len(r.index), func(i int) bool {
		return r.index[i].Key >= key
	})

	// Check for exact match in the index
	if idx < len(r.index) && r.index[idx].Key == key {
		return r.readRecordAt(r.index[idx].Offset, key)
	}

	// Check the block before (key might be between index entries)
	if idx > 0 {
		// Scan from the previous index entry
		startOffset := r.index[idx-1].Offset
		var endOffset int64
		if idx < len(r.index) {
			endOffset = r.index[idx].Offset
		} else {
			// Read up to the index block offset (from footer)
			footer := make([]byte, footerSize)
			if _, err := r.file.ReadAt(footer, r.size-int64(footerSize)); err != nil {
				return nil, false, false, err
			}
			endOffset = int64(binary.LittleEndian.Uint64(footer[0:8]))
		}
		return r.scanRange(startOffset, endOffset, key)
	}

	return nil, false, false, nil
}

// readRecordAt reads a single record at the given file offset.
func (r *SSTableReader) readRecordAt(offset int64, targetKey string) ([]byte, bool, bool, error) {
	// Read key length
	header := make([]byte, 4)
	if _, err := r.file.ReadAt(header, offset); err != nil {
		return nil, false, false, err
	}
	keyLen := int(binary.LittleEndian.Uint32(header))

	// Read full record
	recordHeader := make([]byte, 4+keyLen+4)
	if _, err := r.file.ReadAt(recordHeader, offset); err != nil {
		return nil, false, false, err
	}

	recordKey := string(recordHeader[4 : 4+keyLen])
	if recordKey != targetKey {
		return nil, false, false, nil
	}

	valueLen := int(binary.LittleEndian.Uint32(recordHeader[4+keyLen : 8+keyLen]))

	// Read value + deleted flag + CRC
	tailSize := valueLen + 1 + 4
	tail := make([]byte, tailSize)
	if _, err := r.file.ReadAt(tail, offset+int64(4+keyLen+4)); err != nil {
		return nil, false, false, err
	}

	value := tail[:valueLen]
	deleted := tail[valueLen] == 1
	storedCRC := binary.LittleEndian.Uint32(tail[valueLen+1:])

	// Verify checksum
	crcData := make([]byte, keyLen+valueLen+1)
	copy(crcData, []byte(recordKey))
	copy(crcData[keyLen:], value)
	crcData[len(crcData)-1] = tail[valueLen]
	computedCRC := crc32.Checksum(crcData, r.crcTable)

	if computedCRC != storedCRC {
		return nil, false, false, fmt.Errorf("sstable: CRC mismatch at offset %d", offset)
	}

	if deleted {
		return nil, true, true, nil
	}

	result := make([]byte, len(value))
	copy(result, value)
	return result, true, false, nil
}

// scanRange scans records between startOffset and endOffset looking for targetKey.
func (r *SSTableReader) scanRange(startOffset, endOffset int64, targetKey string) ([]byte, bool, bool, error) {
	blockSize := endOffset - startOffset
	if blockSize <= 0 {
		return nil, false, false, nil
	}

	block := make([]byte, blockSize)
	if _, err := r.file.ReadAt(block, startOffset); err != nil {
		return nil, false, false, err
	}

	pos := 0
	for pos < len(block) {
		if pos+4 > len(block) {
			break
		}
		keyLen := int(binary.LittleEndian.Uint32(block[pos : pos+4]))
		pos += 4

		if pos+keyLen+4 > len(block) {
			break
		}
		key := string(block[pos : pos+keyLen])
		pos += keyLen

		valueLen := int(binary.LittleEndian.Uint32(block[pos : pos+4]))
		pos += 4

		if pos+valueLen+1+4 > len(block) {
			break
		}

		value := block[pos : pos+valueLen]
		pos += valueLen

		deleted := block[pos] == 1
		pos++ // deleted flag

		storedCRC := binary.LittleEndian.Uint32(block[pos : pos+4])
		pos += 4

		if key == targetKey {
			// Verify CRC
			crcData := make([]byte, keyLen+valueLen+1)
			copy(crcData, []byte(key))
			copy(crcData[keyLen:], value)
			if deleted {
				crcData[len(crcData)-1] = 1
			}
			computedCRC := crc32.Checksum(crcData, r.crcTable)
			if computedCRC != storedCRC {
				return nil, false, false, fmt.Errorf("sstable: CRC mismatch for key %s", key)
			}

			if deleted {
				return nil, true, true, nil
			}
			result := make([]byte, len(value))
			copy(result, value)
			return result, true, false, nil
		}

		// Keys are sorted — if we've passed the target, stop
		if key > targetKey {
			break
		}
	}

	return nil, false, false, nil
}

// Iterator returns all records in the SSTable in sorted order.
type SSTableIterator struct {
	reader *io.SectionReader
	crcTab *crc32.Table
	endOffset int64
	pos    int64
}

// NewIterator creates an iterator over all data records.
func (r *SSTableReader) NewIterator() (*SSTableIterator, error) {
	// Determine where data ends (= index offset from footer)
	footer := make([]byte, footerSize)
	if _, err := r.file.ReadAt(footer, r.size-int64(footerSize)); err != nil {
		return nil, err
	}
	indexOffset := int64(binary.LittleEndian.Uint64(footer[0:8]))

	return &SSTableIterator{
		reader:    io.NewSectionReader(r.file, 0, indexOffset),
		crcTab:    r.crcTable,
		endOffset: indexOffset,
		pos:       0,
	}, nil
}

// Next reads the next record. Returns key, value, deleted, ok.
func (it *SSTableIterator) Next() (string, []byte, bool, bool, error) {
	if it.pos >= it.endOffset {
		return "", nil, false, false, nil
	}

	// Read key length
	header := make([]byte, 4)
	if _, err := it.reader.ReadAt(header, it.pos); err != nil {
		if err == io.EOF {
			return "", nil, false, false, nil
		}
		return "", nil, false, false, err
	}
	keyLen := int(binary.LittleEndian.Uint32(header))
	it.pos += 4

	// Read key
	keyBuf := make([]byte, keyLen)
	if _, err := it.reader.ReadAt(keyBuf, it.pos); err != nil {
		return "", nil, false, false, err
	}
	key := string(keyBuf)
	it.pos += int64(keyLen)

	// Read value length
	if _, err := it.reader.ReadAt(header, it.pos); err != nil {
		return "", nil, false, false, err
	}
	valueLen := int(binary.LittleEndian.Uint32(header))
	it.pos += 4

	// Read value + deleted + CRC
	tail := make([]byte, valueLen+1+4)
	if _, err := it.reader.ReadAt(tail, it.pos); err != nil {
		return "", nil, false, false, err
	}
	it.pos += int64(len(tail))

	value := tail[:valueLen]
	deleted := tail[valueLen] == 1

	result := make([]byte, len(value))
	copy(result, value)

	return key, result, deleted, true, nil
}

// Close closes the SSTable reader.
func (r *SSTableReader) Close() error {
	return r.file.Close()
}

// Path returns the file path of this SSTable.
func (r *SSTableReader) Path() string {
	return r.path
}
