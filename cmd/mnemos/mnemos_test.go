package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	exePath := filepath.Join(t.TempDir(), "mnemos_test.exe")
	
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", exePath, "./cmd/mnemos")
	cmd.Dir = "../../"
	
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build binary: %v\nOutput: %s", err, out)
	}
	return exePath
}

func TestMnemosCLI(t *testing.T) {
	exe := buildBinary(t)
	dataDir := filepath.Join(t.TempDir(), ".mnemos")

	// 1. Test Ingest
	corpusDir := filepath.Join("..", "..", "testdata", "corpus")
	cmd := exec.Command(exe, "ingest", "--data-dir", dataDir, corpusDir)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	
	if err := cmd.Run(); err != nil {
		t.Fatalf("Ingest failed: %v\nOutput: %s", err, out.String())
	}
	
	output := out.String()
	if !strings.Contains(output, "Ingestion complete") {
		t.Errorf("Expected success message, got: %s", output)
	}

	// 2. Test Query
	cmd = exec.Command(exe, "query", "--data-dir", dataDir, "machine learning algorithms")
	out.Reset()
	cmd.Stdout = &out
	cmd.Stderr = &out
	
	if err := cmd.Run(); err != nil {
		t.Fatalf("Query failed: %v\nOutput: %s", err, out.String())
	}
	
	output = out.String()
	if !strings.Contains(output, "Found") || !strings.Contains(output, "results in") {
		t.Errorf("Expected query results, got: %s", output)
	}

	// 3. Test Stats
	cmd = exec.Command(exe, "stats", "--data-dir", dataDir)
	out.Reset()
	cmd.Stdout = &out
	cmd.Stderr = &out
	
	if err := cmd.Run(); err != nil {
		t.Fatalf("Stats failed: %v\nOutput: %s", err, out.String())
	}
	
	output = out.String()
	if !strings.Contains(output, "Engine Statistics") {
		t.Errorf("Expected stats output, got: %s", output)
	}
}
