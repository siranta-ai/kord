package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestTokenCounterRace simulates maximum parallel workload on the TokenCounter
// to verify that atomic counters and Mutexes prevent data races.
func TestTokenCounterRace(t *testing.T) {
	var buf bytes.Buffer
	tc := &TokenCounter{W: &buf}

	var wg sync.WaitGroup
	// Simulate 1000 parallel workers slamming the counter
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tc.Write([]byte("mock_token_stream_"))
		}(i)
	}
	wg.Wait()

	expectedCount := int64(1000 * 18) // 18 bytes per payload
	if tc.Count != expectedCount {
		t.Errorf("Expected token count %d, got %d", expectedCount, tc.Count)
	}
}

// TestConcurrentTraversalRace executes the entire file ingestion pipeline
// over a generated directory structure to verify internal channels, maps, 
// and the atomic limiters are completely thread-safe under load.
func TestConcurrentTraversalRace(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create a nested file tree to simulate a repository
	for i := 0; i < 10; i++ {
		subDir := filepath.Join(tempDir, "module", "component")
		os.MkdirAll(subDir, 0755)
		for j := 0; j < 50; j++ {
			f, _ := os.CreateTemp(subDir, "file_*.txt")
			f.WriteString("mock plaintext code")
			f.Close()
		}
	}

	config := &Config{
		TargetDir:      tempDir,
		Output:         "stdout",
		Format:         "json",
		MaxFileSizeStr: "1MB",
		MaxFileSize:    1024 * 1024,
	}
	engine := NewIgnoreEngine(config)

	var buf bytes.Buffer
	tc := &TokenCounter{W: &buf}

	err := traverseDirectory(config, engine, tc, nil)
	if err != nil {
		t.Fatalf("traverseDirectory failed: %v", err)
	}
}
