package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"os"
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

// traverseDirectory is a stub for the directory traversal engine.
// It uses a streaming pipeline to write files to the encoder
// without reading the entire directory into memory.
func traverseDirectory(targetDir, ignoreFile string, encoder *xml.Encoder) error {
	// TODO: Implement streaming directory traversal using filepath.WalkDir
	// Example of encoding a file streamingly:
	//
	// file := File{
	// 	Path: "stub/path.txt",
	// 	Body: "stub content...",
	// }
	// if err := encoder.Encode(file); err != nil {
	// 	return err
	// }

	return nil
}
