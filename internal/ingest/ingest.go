// Package ingest handles document ingestion: directory walking, UTF-8 validation,
// content-hash based idempotency, and feeding documents into the storage engine.
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
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

// job represents a single file to process
type job struct {
	path string
}

// jobResult represents the outcome of processing a file
type jobResult struct {
	doc       *Document
	errStr    string
	skipped   bool
	duplicate bool
}

func IngestDir(root string, existingHashes map[string]bool, progressFn func(path string, current, total int)) ([]Document, IngestResult, error) {
	var result IngestResult
	var docs []Document

	// First pass: count total files for progress reporting
	var totalFiles int
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
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

	jobs := make(chan job, 100)
	results := make(chan jobResult, 100)
	var wg sync.WaitGroup
	
	// Start workers based on CPU count (or a fixed number like 8)
	numWorkers := runtime.NumCPU()
	if numWorkers < 4 {
		numWorkers = 4
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				// Sandboxing: Process each job in an isolated function with panic recovery
				res := processJobSandboxed(j.path, existingHashes)
				results <- res
			}
		}()
	}

	// Start result collector
	done := make(chan struct{})
	go func() {
		current := 0
		for r := range results {
			current++
			if r.errStr != "" {
				result.ErrorFiles++
				result.Errors = append(result.Errors, r.errStr)
			} else if r.skipped {
				result.SkippedFiles++
			} else if r.duplicate {
				result.DuplicateFiles++
			} else if r.doc != nil {
				docs = append(docs, *r.doc)
				result.IngestedFiles++
				result.TotalBytes += r.doc.Size
			}
			
			if progressFn != nil {
				progressFn("processing", current, totalFiles)
			}
		}
		close(done)
	}()

	// Producer: feed files into the jobs channel
	visited := make(map[string]bool)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			results <- jobResult{errStr: fmt.Sprintf("access error: %s: %v", path, err)}
			return nil
		}

		if d.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				results <- jobResult{errStr: fmt.Sprintf("symlink resolve: %s: %v", path, err)}
				return nil
			}
			if visited[resolved] {
				results <- jobResult{errStr: fmt.Sprintf("symlink loop detected: %s → %s", path, resolved)}
				return filepath.SkipDir
			}
			visited[resolved] = true
		}

		if !d.IsDir() && isSupportedFile(path) {
			jobs <- job{path: path}
		}
		return nil
	})

	close(jobs)
	wg.Wait()
	close(results)
	<-done // Wait for collector

	if err != nil {
		return docs, result, fmt.Errorf("ingest: walk: %w", err)
	}

	return docs, result, nil
}

// processJobSandboxed safely processes a single document, defending against panics and resource exhaustion.
func processJobSandboxed(path string, existingHashes map[string]bool) (res jobResult) {
	// Panic recovery (Sandboxing)
	defer func() {
		if r := recover(); r != nil {
			res.errStr = fmt.Sprintf("sandbox panic on %s: %v", path, r)
		}
	}()

	// Impose a soft timeout on processing (Simulated via context if this were network, here we just do basic size checks)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Enforce file size limits to prevent OOM
	info, err := os.Stat(path)
	if err != nil {
		return jobResult{errStr: fmt.Sprintf("stat error: %s: %v", path, err)}
	}
	if info.Size() > 50*1024*1024 { // 50MB limit
		return jobResult{errStr: fmt.Sprintf("file too large (max 50MB): %s", path)}
	}

	// Read file (respecting context deadline is harder for os.ReadFile without custom readers, 
	// but the size limit prevents most hanging issues)
	data, err := os.ReadFile(path)
	if err != nil {
		return jobResult{errStr: fmt.Sprintf("read error: %s: %v", path, err)}
	}

	// Check context
	if err := ctx.Err(); err != nil {
		return jobResult{errStr: fmt.Sprintf("timeout processing: %s", path)}
	}

	if !utf8.Valid(data) {
		return jobResult{skipped: true, errStr: fmt.Sprintf("skipped non-UTF-8: %s", path)}
	}

	content := string(data)
	if strings.TrimSpace(content) == "" {
		return jobResult{skipped: true}
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	if existingHashes != nil && existingHashes[hashStr] {
		return jobResult{duplicate: true}
	}

	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	return jobResult{
		doc: &Document{
			ID:      hashStr,
			Path:    path,
			Title:   title,
			Content: content,
			Size:    int64(len(data)),
		},
	}
}

func isSupportedFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return supportedExtensions[ext]
}


