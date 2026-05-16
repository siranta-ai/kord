package main

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// File represents a file to be streamed to XML.
// It has a path attribute and the content as a CDATA body.
type File struct {
	XMLName xml.Name `xml:"file"`
	Path    string   `xml:"path,attr"`
	Body    string   `xml:",cdata"`
}

func main() {
	// Parse CLI flags
	dirFlag := flag.String("dir", ".", "target directory")
	ignoreFlag := flag.String("ignore", ".gitignore", "custom ignore file")
	flag.Parse()

	// Initialize IgnoreEngine
	ignoreEngine := NewIgnoreEngine(*ignoreFlag)

	// Initialize XML encoder writing directly to stdout
	encoder := xml.NewEncoder(os.Stdout)
	encoder.Indent("", "  ")

	// Print XML header
	fmt.Print(xml.Header)

	// Define and encode the <repository> root tag
	rootStart := xml.StartElement{Name: xml.Name{Local: "repository"}}
	if err := encoder.EncodeToken(rootStart); err != nil {
		fmt.Fprintf(os.Stderr, "error writing root start token: %v\n", err)
		os.Exit(1)
	}

	// Run the directory traversal engine stub
	if err := traverseDirectory(*dirFlag, ignoreEngine, encoder); err != nil {
		fmt.Fprintf(os.Stderr, "error during directory traversal: %v\n", err)
		os.Exit(1)
	}

	// Close the <repository> root tag
	if err := encoder.EncodeToken(rootStart.End()); err != nil {
		fmt.Fprintf(os.Stderr, "error writing root end token: %v\n", err)
		os.Exit(1)
	}

	// Flush buffered data to stdout
	if err := encoder.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "error flushing xml encoder: %v\n", err)
		os.Exit(1)
	}

	// Add a trailing newline
	fmt.Println()
}

// traverseDirectory uses filepath.WalkDir to streamingly read and encode files
// without reading the entire directory into memory.
func traverseDirectory(targetDir string, engine *IgnoreEngine, encoder *xml.Encoder) error {
	return filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		// Skip unreadable paths safely without crashing the pipeline
		if err != nil {
			fmt.Fprintf(os.Stderr, "error accessing path %q: %v\n", path, err)
			return nil
		}

		if engine.IsIgnored(path, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Process only regular files
		if !d.IsDir() {
			f, readErr := os.Open(path)
			if readErr != nil {
				fmt.Fprintf(os.Stderr, "error opening file %q: %v\n", path, readErr)
				return nil // Continue the pipeline
			}

			// Ensure we only read a small prefix for binary detection
			prefix := make([]byte, 512)
			n, _ := f.Read(prefix)
			if n > 0 {
				if bytes.IndexByte(prefix[:n], 0) != -1 {
					f.Close()
					return nil // Skip binary files
				}
			}

			// Stream the file content to the XML encoder in chunks to avoid buffering large files
			start := xml.StartElement{Name: xml.Name{Local: "file"}, Attr: []xml.Attr{{Name: xml.Name{Local: "path"}, Value: path}}}
			if err := encoder.EncodeToken(start); err != nil {
				fmt.Fprintf(os.Stderr, "error writing file start %q: %v\n", path, err)
				f.Close()
				return nil
			}

			// Flush encoder so the start token is written before raw streaming
			if err := encoder.Flush(); err != nil {
				fmt.Fprintf(os.Stderr, "error flushing before raw write for %q: %v\n", path, err)
			}

			// Write CDATA start and stream raw bytes directly to stdout to avoid buffering
			if _, err := fmt.Fprint(os.Stdout, "<![CDATA["); err != nil {
				fmt.Fprintf(os.Stderr, "error writing cdata start for %q: %v\n", path, err)
			}

			// If we already read some prefix bytes, write them first
			if n > 0 {
				if _, err := os.Stdout.Write(prefix[:n]); err != nil {
					fmt.Fprintf(os.Stderr, "error writing prefix for %q: %v\n", path, err)
				}
			}

			// Continue streaming the rest of the file directly using a single preallocated buffer
			buf := make([]byte, 32*1024)
			if _, err := io.CopyBuffer(os.Stdout, f, buf); err != nil {
				if err != io.EOF {
					fmt.Fprintf(os.Stderr, "error copying file %q: %v\n", path, err)
				}
			}

			// Close CDATA and then write end element token via encoder to remain well-formed
			if _, err := fmt.Fprint(os.Stdout, "]]>"); err != nil {
				fmt.Fprintf(os.Stderr, "error writing cdata end for %q: %v\n", path, err)
			}

			if err := encoder.EncodeToken(start.End()); err != nil {
				fmt.Fprintf(os.Stderr, "error writing file end %q: %v\n", path, err)
			}

			f.Close()
		}

		return nil
	})
}

// IgnoreEngine parses and evaluates .gitignore rules without using regexp.
type IgnoreEngine struct {
	exactDirs  map[string]bool
	exactFiles map[string]bool
	suffixes   []string
	prefixes   []string
}

// NewIgnoreEngine reads a .gitignore file and categorizes rules into buckets.
func NewIgnoreEngine(ignoreFilePath string) *IgnoreEngine {
	engine := &IgnoreEngine{
		exactDirs:  make(map[string]bool),
		exactFiles: make(map[string]bool),
		suffixes:   make([]string, 0),
		prefixes:   make([]string, 0),
	}

	// Always hardcode bypasses
	engine.exactDirs[".git"] = true
	engine.exactDirs["node_modules"] = true
	engine.exactDirs["vendor"] = true

	content, err := os.ReadFile(ignoreFilePath)
	if err != nil {
		// If the ignore file doesn't exist or can't be read, return default engine
		return engine
	}

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Ignore comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		isDirRule := strings.HasSuffix(line, "/")
		if isDirRule {
			line = strings.TrimSuffix(line, "/")
		}

		// Categorize into the four buckets
		if strings.HasPrefix(line, "*") {
			engine.suffixes = append(engine.suffixes, strings.TrimPrefix(line, "*"))
		} else if strings.HasSuffix(line, "*") {
			engine.prefixes = append(engine.prefixes, strings.TrimSuffix(line, "*"))
		} else {
			if isDirRule {
				engine.exactDirs[line] = true
			} else {
				engine.exactFiles[line] = true
				engine.exactDirs[line] = true // Without trailing slash, can match both file and dir
			}
		}
	}

	return engine
}

// IsIgnored evaluates if a path should be skipped based on simple string comparisons.
func (ie *IgnoreEngine) IsIgnored(path string, isDir bool) bool {
	base := filepath.Base(path)

	// 1. Exact map lookups
	if isDir {
		if ie.exactDirs[base] {
			return true
		}
	} else {
		if ie.exactFiles[base] {
			return true
		}
	}

	// 2. Suffixes
	for _, sfx := range ie.suffixes {
		if strings.HasSuffix(base, sfx) {
			return true
		}
	}

	// 3. Prefixes
	for _, pfx := range ie.prefixes {
		if strings.HasPrefix(base, pfx) {
			return true
		}
	}

	return false
}
