package main

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
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
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				fmt.Fprintf(os.Stderr, "error reading file %q: %v\n", path, readErr)
				return nil // Continue the pipeline
			}

			// Extremely fast binary detection: check for null byte
			if bytes.IndexByte(content, 0) != -1 {
				return nil // Skip binary files
			}

			file := File{
				Path: path,
				Body: string(content),
			}

			if encErr := encoder.Encode(file); encErr != nil {
				// Handle encoding errors silently via os.Stderr without killing the stream
				fmt.Fprintf(os.Stderr, "error encoding file %q: %v\n", path, encErr)
			}
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
