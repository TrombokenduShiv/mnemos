// Package ingest handles document ingestion: directory walking, UTF-8 validation,
// content-hash based idempotency, and feeding documents into the storage engine.
package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Document represents an ingested document.
type Document struct {
	ID       string // content-based SHA-256 hash
	Path     string // original file path
	Title    string // filename without extension
	Content  string // full text content
	Size     int64  // byte size
}

// IngestResult holds statistics from an ingestion run.
type IngestResult struct {
	TotalFiles    int
	IngestedFiles int
	SkippedFiles  int
	DuplicateFiles int
	ErrorFiles    int
	TotalBytes    int64
	Errors        []string
}

// supportedExtensions lists file extensions we ingest.
var supportedExtensions = map[string]bool{
	".txt": true,
	".md":  true,
	".markdown": true,
	".text": true,
	".rst": true,
	".log": true,
	".csv": true,
	".json": true,
	".xml": true,
	".html": true,
	".htm": true,
	".yaml": true,
	".yml": true,
	".toml": true,
	".cfg": true,
	".ini": true,
	".conf": true,
	".tex": true,
	".org": true,
	".wiki": true,
	".adoc": true,
}

// IngestDir walks a directory and returns all valid documents.
// Handles nested directories, skips non-UTF-8 files, detects duplicates via content hash.
func IngestDir(root string, existingHashes map[string]bool, progressFn func(path string, current, total int)) ([]Document, IngestResult, error) {
	result := IngestResult{}

	// First pass: count total files for progress reporting
	var totalFiles int
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip inaccessible entries
		}
		if !d.IsDir() && isSupportedFile(path) {
			totalFiles++
		}
		return nil
	})
	if err != nil {
		return nil, result, fmt.Errorf("ingest: walk for count: %w", err)
	}

	result.TotalFiles = totalFiles
	var docs []Document
	visited := make(map[string]bool) // track visited inodes/paths to detect symlink loops
	current := 0

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			result.ErrorFiles++
			result.Errors = append(result.Errors, fmt.Sprintf("access error: %s: %v", path, err))
			return nil // Continue walking
		}

		// Symlink loop detection via resolved path
		if d.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("symlink resolve: %s: %v", path, err))
				return nil
			}
			if visited[resolved] {
				result.Errors = append(result.Errors, fmt.Sprintf("symlink loop detected: %s → %s", path, resolved))
				return filepath.SkipDir
			}
			visited[resolved] = true
		}

		if d.IsDir() {
			return nil
		}

		if !isSupportedFile(path) {
			return nil
		}

		current++
		if progressFn != nil {
			progressFn(path, current, totalFiles)
		}

		// Read file
		data, err := os.ReadFile(path)
		if err != nil {
			result.ErrorFiles++
			result.Errors = append(result.Errors, fmt.Sprintf("read error: %s: %v", path, err))
			return nil
		}

		// Validate UTF-8
		if !utf8.Valid(data) {
			result.SkippedFiles++
			result.Errors = append(result.Errors, fmt.Sprintf("skipped non-UTF-8: %s", path))
			return nil
		}

		content := string(data)
		if strings.TrimSpace(content) == "" {
			result.SkippedFiles++
			return nil
		}

		// Content hash for idempotency
		hash := sha256.Sum256(data)
		hashStr := hex.EncodeToString(hash[:])

		if existingHashes != nil && existingHashes[hashStr] {
			result.DuplicateFiles++
			return nil
		}

		// Extract title from filename
		title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

		doc := Document{
			ID:      hashStr,
			Path:    path,
			Title:   title,
			Content: content,
			Size:    int64(len(data)),
		}

		docs = append(docs, doc)
		result.IngestedFiles++
		result.TotalBytes += doc.Size

		return nil
	})

	if err != nil {
		return docs, result, fmt.Errorf("ingest: walk: %w", err)
	}

	return docs, result, nil
}

func isSupportedFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return supportedExtensions[ext]
}


