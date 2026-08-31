package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWAL_AppendAndReplay(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mnemos-wal-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	walPath := filepath.Join(tempDir, "wal.log")
	
	// Create and write to WAL
	wal, err := NewWAL(walPath)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}

	err = wal.AppendPut([]byte("key1"), []byte("value1"))
	if err != nil {
		t.Fatalf("Failed to append put: %v", err)
	}

	err = wal.AppendDelete([]byte("key2"))
	if err != nil {
		t.Fatalf("Failed to append delete: %v", err)
	}

	err = wal.Close()
	if err != nil {
		t.Fatalf("Failed to close WAL: %v", err)
	}

	// Reopen and replay WAL
	wal2, err := NewWAL(walPath)
	if err != nil {
		t.Fatalf("Failed to reopen WAL: %v", err)
	}
	defer wal2.Close()

	var records []WALRecord
	count, err := wal2.Replay(func(r WALRecord) error {
		records = append(records, r)
		return nil
	})

	if err != nil {
		t.Fatalf("Failed to replay WAL: %v", err)
	}
	if count != 2 {
		t.Fatalf("Expected 2 records replayed, got %d", count)
	}

	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}

	if string(records[0].Key) != "key1" || string(records[0].Value) != "value1" || records[0].Op != OpPut {
		t.Errorf("Unexpected first record: %+v", records[0])
	}
	if string(records[1].Key) != "key2" || records[1].Op != OpDelete {
		t.Errorf("Unexpected second record: %+v", records[1])
	}
}
