// Package storage implements a log-structured storage engine from first principles.
// WAL (Write-Ahead Log) provides durability guarantees: every write is appended
// to a sequential log and fsync'd before being acknowledged to the caller.
// On crash recovery, the WAL is replayed to reconstruct any state lost since
// the last SSTable flush.
package storage

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"
	"time"
)

// WAL operation types.
const (
	OpPut        byte = 1
	OpDelete     byte = 2
	OpCommitMark byte = 3
)

// WAL record header size: CRC32(4) + Length(4) + Op(1) + Timestamp(8) = 17 bytes.
const walHeaderSize = 17

// WALRecord represents a single entry in the write-ahead log.
type WALRecord struct {
	Op        byte
	Timestamp int64
	Key       []byte
	Value     []byte
}

// WAL is a write-ahead log that provides crash-recovery durability.
// Every mutation is appended here before being applied to the memtable.
type WAL struct {
	mu       sync.Mutex
	file     *os.File
	writer   *bufio.Writer
	path     string
	size     int64
	seqNum   uint64
	crcTable *crc32.Table
}

// NewWAL creates or opens a WAL file at the given path.
func NewWAL(path string) (*WAL, error) {
	// We do NOT use os.O_APPEND because on Windows, it prevents File.Truncate() 
	// from working (resulting in Access is denied). Since we use a Mutex for writes,
	// we manually manage the append offset.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("wal: open %s: %w", path, err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("wal: stat %s: %w", path, err)
	}

	return &WAL{
		file:     file,
		writer:   bufio.NewWriterSize(file, 64*1024), // 64KB write buffer
		path:     path,
		size:     info.Size(),
		crcTable: crc32.MakeTable(crc32.Castagnoli),
	}, nil
}

// Append writes a record to the WAL and fsyncs it to disk.
// The write is only considered durable after fsync completes.
func (w *WAL) Append(rec WALRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Encode the payload: key length (4) + key + value length (4) + value
	payloadSize := 4 + len(rec.Key) + 4 + len(rec.Value)
	payload := make([]byte, payloadSize)
	binary.LittleEndian.PutUint32(payload[0:4], uint32(len(rec.Key)))
	copy(payload[4:4+len(rec.Key)], rec.Key)
	binary.LittleEndian.PutUint32(payload[4+len(rec.Key):8+len(rec.Key)], uint32(len(rec.Value)))
	copy(payload[8+len(rec.Key):], rec.Value)

	// Build the checksummed data: Op + Timestamp + Payload
	checksumData := make([]byte, 1+8+len(payload))
	checksumData[0] = rec.Op
	binary.LittleEndian.PutUint64(checksumData[1:9], uint64(rec.Timestamp))
	copy(checksumData[9:], payload)

	checksum := crc32.Checksum(checksumData, w.crcTable)

	// Write header: CRC32 + Length + Op + Timestamp
	header := make([]byte, walHeaderSize)
	binary.LittleEndian.PutUint32(header[0:4], checksum)
	binary.LittleEndian.PutUint32(header[4:8], uint32(len(payload)))
	header[8] = rec.Op
	binary.LittleEndian.PutUint64(header[9:17], uint64(rec.Timestamp))

	// Write header + payload
	if _, err := w.writer.Write(header); err != nil {
		return fmt.Errorf("wal: write header: %w", err)
	}
	if _, err := w.writer.Write(payload); err != nil {
		return fmt.Errorf("wal: write payload: %w", err)
	}

	// Flush buffered writer and fsync for durability
	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("wal: flush: %w", err)
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("wal: sync: %w", err)
	}

	w.size += int64(walHeaderSize + len(payload))
	w.seqNum++
	return nil
}

// AppendPut is a convenience wrapper for appending a PUT record.
func (w *WAL) AppendPut(key, value []byte) error {
	return w.Append(WALRecord{
		Op:        OpPut,
		Timestamp: time.Now().UnixNano(),
		Key:       key,
		Value:     value,
	})
}

// AppendDelete is a convenience wrapper for appending a DELETE (tombstone) record.
func (w *WAL) AppendDelete(key []byte) error {
	return w.Append(WALRecord{
		Op:        OpDelete,
		Timestamp: time.Now().UnixNano(),
		Key:       key,
		Value:     nil,
	})
}

// Replay reads the WAL from the beginning and calls fn for each valid record.
// Corrupt or truncated trailing records (from a crash mid-write) are silently
// discarded — the WAL is truncated at the last valid record boundary.
func (w *WAL) Replay(fn func(WALRecord) error) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Seek to beginning for replay
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("wal: seek to start: %w", err)
	}

	reader := bufio.NewReaderSize(w.file, 64*1024)
	var validRecords int
	var lastValidOffset int64

	for {
		// Read header
		header := make([]byte, walHeaderSize)
		_, err := io.ReadFull(reader, header)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break // End of file or truncated header — stop replay
			}
			return validRecords, fmt.Errorf("wal: read header: %w", err)
		}

		storedCRC := binary.LittleEndian.Uint32(header[0:4])
		payloadLen := binary.LittleEndian.Uint32(header[4:8])
		op := header[8]
		timestamp := int64(binary.LittleEndian.Uint64(header[9:17]))

		// Sanity check payload length to avoid huge allocations from corrupt data
		if payloadLen > 256*1024*1024 { // 256 MB limit
			break // Corrupt record — stop replay
		}

		// Read payload
		payload := make([]byte, payloadLen)
		_, err = io.ReadFull(reader, payload)
		if err != nil {
			break // Truncated payload — stop replay, discard this record
		}

		// Verify checksum
		checksumData := make([]byte, 1+8+len(payload))
		checksumData[0] = op
		binary.LittleEndian.PutUint64(checksumData[1:9], uint64(timestamp))
		copy(checksumData[9:], payload)

		computedCRC := crc32.Checksum(checksumData, w.crcTable)
		if computedCRC != storedCRC {
			break // Corrupt record — stop replay
		}

		// Parse payload into key/value
		if len(payload) < 4 {
			break // Corrupt
		}
		keyLen := binary.LittleEndian.Uint32(payload[0:4])
		if int(4+keyLen+4) > len(payload) {
			break // Corrupt
		}
		key := payload[4 : 4+keyLen]

		valLen := binary.LittleEndian.Uint32(payload[4+keyLen : 8+keyLen])
		if int(8+keyLen+valLen) > len(payload) {
			break // Corrupt
		}
		value := payload[8+keyLen : 8+keyLen+valLen]

		rec := WALRecord{
			Op:        op,
			Timestamp: timestamp,
			Key:       key,
			Value:     value,
		}

		if err := fn(rec); err != nil {
			return validRecords, fmt.Errorf("wal: replay callback: %w", err)
		}

		validRecords++
		lastValidOffset += int64(walHeaderSize) + int64(payloadLen)
	}

	// Seek to the truncation point first (Windows requires this before Truncate)
	if _, err := w.file.Seek(lastValidOffset, io.SeekStart); err != nil {
		return validRecords, fmt.Errorf("wal: seek to truncation point: %w", err)
	}

	// Truncate the WAL at the last valid record to remove any corrupt tail
	if err := w.file.Truncate(lastValidOffset); err != nil {
		// On Windows, truncation may fail with "Access is denied" if another
		// handle or buffered reader holds a reference. In that case, just seek
		// to end and continue — the corrupt tail will be detected on next replay.
		fmt.Fprintf(os.Stderr, "wal: truncate warning (non-fatal): %v\n", err)
	}

	// Seek to end for future appends
	if _, err := w.file.Seek(0, io.SeekEnd); err != nil {
		return validRecords, fmt.Errorf("wal: seek to end: %w", err)
	}

	// Reset the buffered writer to use the repositioned file
	w.writer.Reset(w.file)
	w.size = lastValidOffset

	return validRecords, nil
}

// Size returns the current WAL file size in bytes.
func (w *WAL) Size() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.size
}

// Reset truncates the WAL (called after a successful SSTable flush).
func (w *WAL) Reset() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.file.Truncate(0); err != nil {
		return fmt.Errorf("wal: truncate: %w", err)
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("wal: seek: %w", err)
	}
	w.writer.Reset(w.file)
	w.size = 0
	w.seqNum = 0
	return nil
}

// Close flushes and closes the WAL file.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("wal: flush on close: %w", err)
	}
	return w.file.Close()
}
