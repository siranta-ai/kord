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

const startupBanner = `
░██                                           ░██ 
░██                                           ░██ 
░██    ░██    ░███████     ░██░████     ░████████ 
░██   ░██    ░██    ░██    ░███        ░██    ░██ 
░███████     ░██    ░██    ░██         ░██    ░██ 
░██   ░██    ░██    ░██    ░██         ░██   ░███ 
░██    ░██    ░███████     ░██          ░█████░██ 
                                                  
                                                                 
Kord by Siranta - streaming repository to XML
`

// File represents a file to be streamed to XML.
// It has a path attribute and the content as a CDATA body.
type File struct {
	XMLName xml.Name `xml:"file"`
	Path    string   `xml:"path,attr"`
	Body    string   `xml:",cdata"`
}

func main() {
	printStartupBanner()

	if len(os.Args) > 1 && os.Args[1] == "start" {
		runInteractiveWizard()
		return
	}

	// Parse CLI flags
	dirFlag := flag.String("dir", ".", "target directory")
	ignoreFlag := flag.String("ignore", ".gitignore", "custom ignore file")
	maxSize := flag.Int64("max-size", 50000, "Maximum file size in bytes to include content (default 50KB)")
	flag.Parse()

	runCoreLogic(*dirFlag, *ignoreFlag, *maxSize, os.Stdout)
}

func runInteractiveWizard() {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("Enter target directory [default .]: ")
	scanner.Scan()
	targetDir := strings.TrimSpace(scanner.Text())
	if targetDir == "" {
		targetDir = "."
	}

	fmt.Print("Enter output file name [default stdout]: ")
	scanner.Scan()
	outFileName := strings.TrimSpace(scanner.Text())

	var out io.Writer = os.Stdout
	if outFileName != "" {
		f, err := os.Create(outFileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		out = f
	}

	runCoreLogic(targetDir, ".gitignore", 50000, out)
}

func runCoreLogic(targetDir, ignoreFile string, maxSize int64, out io.Writer) {
	// Initialize IgnoreEngine
	ignoreEngine := NewIgnoreEngine(ignoreFile)

	// Initialize XML encoder writing directly to the output writer
	encoder := xml.NewEncoder(out)
	encoder.Indent("", "  ")

	// Print XML header
	fmt.Fprint(out, xml.Header)

	// Define and encode the <repository> root tag
	rootStart := xml.StartElement{Name: xml.Name{Local: "repository"}}
	if err := encoder.EncodeToken(rootStart); err != nil {
		fmt.Fprintf(os.Stderr, "error writing root start token: %v\n", err)
		os.Exit(1)
	}

	// Get output file's FileInfo to avoid reading our own output file
	var outFileInfo os.FileInfo
	if outFile, ok := out.(*os.File); ok {
		if info, err := outFile.Stat(); err == nil {
			outFileInfo = info
		}
	}

	// Run the directory traversal engine stub
	if err := traverseDirectory(targetDir, ignoreEngine, encoder, out, maxSize, outFileInfo); err != nil {
		fmt.Fprintf(os.Stderr, "error during directory traversal: %v\n", err)
		os.Exit(1)
	}

	// Close the <repository> root tag
	if err := encoder.EncodeToken(rootStart.End()); err != nil {
		fmt.Fprintf(os.Stderr, "error writing root end token: %v\n", err)
		os.Exit(1)
	}

	// Flush buffered data
	if err := encoder.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "error flushing xml encoder: %v\n", err)
		os.Exit(1)
	}

	// Add a trailing newline
	fmt.Fprintln(out)
}

func printStartupBanner() {
	fmt.Fprintln(os.Stderr, startupBanner)
}

// traverseDirectory uses filepath.WalkDir to streamingly read and encode files
// without reading the entire directory into memory.
func traverseDirectory(targetDir string, engine *IgnoreEngine, encoder *xml.Encoder, out io.Writer, maxSize int64, outFileInfo os.FileInfo) error {
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
			// Get file info for size check
			info, err := d.Info()
			if err != nil {
				fmt.Fprintf(os.Stderr, "error getting info for %q: %v\n", path, err)
				return nil
			}

			// Prevent infinite feedback loop by skipping the output file itself
			if outFileInfo != nil && os.SameFile(info, outFileInfo) {
				fmt.Fprintf(os.Stderr, "Kord: Skipping output file %s\n", path)
				return nil
			}

			// 1. The hard size limit
			ext := strings.ToLower(filepath.Ext(path))
			currentLimit := maxSize
			if ext == ".md" || ext == ".txt" || ext == ".mdx" {
				currentLimit *= 10
			}

			if info.Size() > currentLimit {
				// Write the path, but explicitly mark the content as omitted to save context
				// Do NOT read the file into memory.
				fmt.Fprintf(os.Stderr, "Kord: Skipping content of %s (Size: %d bytes exceeds limit)\n", path, info.Size())

				// Emit an empty tag so the LLM knows the file exists, but doesn't waste tokens on it
				err = encoder.EncodeToken(xml.StartElement{
					Name: xml.Name{Local: "file"},
					Attr: []xml.Attr{
						{Name: xml.Name{Local: "path"}, Value: path},
						{Name: xml.Name{Local: "omitted"}, Value: "size_limit_exceeded"},
					},
				})
				if err == nil {
					encoder.EncodeToken(xml.EndElement{Name: xml.Name{Local: "file"}})
				}
				return nil
			}

			// 2. Skip SVG file contents
			if strings.ToLower(filepath.Ext(path)) == ".svg" {
				fmt.Fprintf(os.Stderr, "Kord: Skipping content of %s (SVG bloat omitted)\n", path)

				err = encoder.EncodeToken(xml.StartElement{
					Name: xml.Name{Local: "file"},
					Attr: []xml.Attr{
						{Name: xml.Name{Local: "path"}, Value: path},
						{Name: xml.Name{Local: "omitted"}, Value: "svg_bloat_omitted"},
					},
				})
				if err == nil {
					encoder.EncodeToken(xml.EndElement{Name: xml.Name{Local: "file"}})
				}
				return nil
			}

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

			// Write CDATA start and stream raw bytes directly to out to avoid buffering
			if _, err := fmt.Fprint(out, "<![CDATA["); err != nil {
				fmt.Fprintf(os.Stderr, "error writing cdata start for %q: %v\n", path, err)
			}

			// If we already read some prefix bytes, write them first
			if n > 0 {
				if _, err := out.Write(prefix[:n]); err != nil {
					fmt.Fprintf(os.Stderr, "error writing prefix for %q: %v\n", path, err)
				}
			}

			// Continue streaming the rest of the file directly using a single preallocated buffer
			buf := make([]byte, 32*1024)
			if _, err := io.CopyBuffer(out, f, buf); err != nil {
				if err != io.EOF {
					fmt.Fprintf(os.Stderr, "error copying file %q: %v\n", path, err)
				}
			}

			// Close CDATA and then write end element token via encoder to remain well-formed
			if _, err := fmt.Fprint(out, "]]>"); err != nil {
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
	engine.exactDirs[".next"] = true
	engine.exactDirs["dist"] = true
	engine.exactDirs["build"] = true
	engine.exactDirs[".gradle"] = true
	engine.exactDirs["venv"] = true
	engine.exactDirs[".venv"] = true
	engine.exactDirs["target"] = true
	engine.exactDirs["obj"] = true
	engine.exactDirs["__pycache__"] = true
	engine.exactDirs[".dart_tool"] = true
	engine.exactDirs["Pods"] = true

	// Exclude token-trap files by default
	engine.suffixes = append(engine.suffixes,
		".svg", ".png", ".jpg", ".jpeg", ".gif", ".ico", ".webp",
		".lock", "go.sum", ".min.js", ".min.css", ".map",
		".exe", ".dll", ".so", ".dylib", ".bin", ".zip", ".tar.gz", ".rar", ".7z", ".pdf", ".pyc", ".class",
	)

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
