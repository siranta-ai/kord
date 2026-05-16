package main

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// createHostileRepo generates a synthetic monorepo in a temp dir.
// It returns the root path containing the generated structure.
func createHostileRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Deep nesting (12 levels)
	cur := root
	for i := 0; i < 12; i++ {
		cur = filepath.Join(cur, fmt.Sprintf("deep%d", i))
		if err := os.MkdirAll(cur, 0o755); err != nil {
			t.Fatalf("failed to mkdir %q: %v", cur, err)
		}
	}

	// 1,100+ small files distributed across nested dirs
	total := 1100
	for i := 0; i < total; i++ {
		d := filepath.Join(root, fmt.Sprintf("pkg%d", i%50))
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdirall failed: %v", err)
		}
		p := filepath.Join(d, fmt.Sprintf("file_%d.txt", i))
		f, err := os.Create(p)
		if err != nil {
			t.Fatalf("create file failed: %v", err)
		}
		if _, err := f.WriteString(fmt.Sprintf("small content %d\n", i)); err != nil {
			f.Close()
			t.Fatalf("write failed: %v", err)
		}
		f.Close()
	}

	// Massive multi-megabyte dummy text file (6 MiB)
	bigPath := filepath.Join(root, "BIGFILE.txt")
	bf, err := os.Create(bigPath)
	if err != nil {
		t.Fatalf("create big file: %v", err)
	}
	w := bufio.NewWriter(bf)
	chunk := make([]byte, 64*1024)
	for i := range chunk {
		chunk[i] = 'A'
	}
	for i := 0; i < (6*1024*1024)/(64*1024); i++ {
		if _, err := w.Write(chunk); err != nil {
			bf.Close()
			t.Fatalf("write bigfile: %v", err)
		}
	}
	w.Flush()
	bf.Close()

	// Dummy binary files mixed in
	for i := 0; i < 20; i++ {
		p := filepath.Join(root, fmt.Sprintf("bin_%d.bin", i))
		f, err := os.Create(p)
		if err != nil {
			t.Fatalf("create bin: %v", err)
		}
		// write some zeros and random data
		buf := make([]byte, 1024)
		for j := range buf {
			if j%16 == 0 {
				buf[j] = 0
			} else {
				buf[j] = byte(rand.Intn(256))
			}
		}
		f.Write(buf)
		f.Close()
	}

	// Complex .gitignore
	ignore := filepath.Join(root, ".gitignore")
	g, err := os.Create(ignore)
	if err != nil {
		t.Fatalf("create gitignore: %v", err)
	}
	w2 := bufio.NewWriter(g)
	// exact
	w2.WriteString("file_7.txt\n")
	// prefix
	w2.WriteString("temp*\n")
	// suffix
	w2.WriteString("*.log\n")
	// dir rule
	w2.WriteString("vendor/\n")
	// many other rules
	for i := 0; i < 40; i++ {
		if i%3 == 0 {
			w2.WriteString(fmt.Sprintf("exactname%d\n", i))
		} else if i%3 == 1 {
			w2.WriteString(fmt.Sprintf("pre%d*\n", i))
		} else {
			w2.WriteString(fmt.Sprintf("*suf%d\n", i))
		}
	}
	w2.Flush()
	g.Close()

	return root
}

// TestKordStreamingMemory runs Kord against the synthetic monorepo and ensures
// the XML output is valid, and memory does not spike linearly with the big file.
func TestKordStreamingMemory(t *testing.T) {
	dir := createHostileRepo(t)

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}

	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()
	os.Stdout = w

	// Run XML decoder concurrently on the read end
	dec := xml.NewDecoder(r)
	errCh := make(chan error, 1)
	go func() {
		for {
			_, e := dec.Token()
			if e != nil {
				if e == io.EOF {
					errCh <- nil
					return
				}
				errCh <- e
				return
			}
		}
	}()

	// Run main with arguments pointing at our temp dir and ignore file
	os.Args = []string{"cmd", "-dir", dir, "-ignore", filepath.Join(dir, ".gitignore")}
	// execute main; it writes XML to stdout (the pipe writer)
	main()

	// close writer to signal EOF to decoder
	w.Close()

	select {
	case e := <-errCh:
		if e != nil {
			t.Fatalf("xml decode error: %v", e)
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("timeout waiting for xml decoder")
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	// Use HeapAlloc (live heap) to detect if large content was buffered in memory.
	delta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if delta > 10*1024*1024 {
		t.Fatalf("heap allocation spike too large: %d bytes > 10MB", delta)
	}
}

// BenchmarkIgnoreEngine stresses the ignore engine with 50 rules and 10k paths
// and asserts zero heap allocations per operation.
func BenchmarkIgnoreEngine(b *testing.B) {
	// prepare temp ignore file
	root := b.TempDir()
	ignore := filepath.Join(root, ".gitignore")
	f, err := os.Create(ignore)
	if err != nil {
		b.Fatalf("create ignore: %v", err)
	}
	w := bufio.NewWriter(f)
	for i := 0; i < 50; i++ {
		if i%3 == 0 {
			w.WriteString(fmt.Sprintf("exact%d\n", i))
		} else if i%3 == 1 {
			w.WriteString(fmt.Sprintf("pre%d*\n", i))
		} else {
			w.WriteString(fmt.Sprintf("*suf%d\n", i))
		}
	}
	w.Flush()
	f.Close()

	engine := NewIgnoreEngine(ignore)

	// generate 10k random paths
	paths := make([]string, 10000)
	rand.Seed(1)
	for i := 0; i < len(paths); i++ {
		paths[i] = fmt.Sprintf("a/b/c/path_%d_file_%d.suf%d", rand.Intn(10000), i, i%50)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(paths); j++ {
			engine.IsIgnored(paths[j], false)
		}
	}

	// Prove zero allocations per operation by sampling allocations over many runs.
	avgAllocs := testing.AllocsPerRun(5, func() {
		for j := 0; j < len(paths); j++ {
			engine.IsIgnored(paths[j], false)
		}
	})
	perOp := avgAllocs / float64(len(paths))
	if perOp > 0.0001 {
		b.Fatalf("IsIgnored allocates on average %f heap objects per op", perOp)
	}
}
