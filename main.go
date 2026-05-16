package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	if err := traverseDirectory(*dirFlag, *ignoreFlag, encoder); err != nil {
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
func traverseDirectory(targetDir, ignoreFile string, encoder *xml.Encoder) error {
	return filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		// Skip unreadable paths safely without crashing the pipeline
		if err != nil {
			fmt.Fprintf(os.Stderr, "error accessing path %q: %v\n", path, err)
			return nil
		}

		if isIgnored(path, d) {
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

// isIgnored is a stub function to determine if a path should be skipped.
func isIgnored(path string, d fs.DirEntry) bool {
	// TODO: implement logic to parse and match .gitignore rules
	return false
}
