package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
                                                  
                                                                 
Kord by Siranta - streaming repository to XML/JSON/Markdown
`

// ErrTokenLimitExceeded is returned when the token limit is exceeded.
var ErrTokenLimitExceeded = fmt.Errorf("token limit exceeded")

// ErrBinaryFile is returned when a binary file is detected.
var ErrBinaryFile = fmt.Errorf("binary file skipped")

// Config represents all CLI settings.
type Config struct {
	TargetDir         string
	Output            string
	Quiet             bool
	IgnorePatterns    string
	NoGitignore       bool
	DefaultIgnores    bool
	SirantaIgnoreFile string
	MaxFileSizeStr    string
	MaxTokens         int
	MinLines          int
	IncludeDiff       bool
	IncludeGitLog     bool
	IncludeSchemas    bool
	Format            string
	Compress          bool
	IncludeToc        bool

	// Internal fields
	MaxFileSize int64
}

// TokenCounter wraps an io.Writer and tracks bytes written.
type TokenCounter struct {
	W     io.Writer
	Count int
}

func (tc *TokenCounter) Write(p []byte) (n int, err error) {
	n, err = tc.W.Write(p)
	tc.Count += n
	return n, err
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "start" {
		printStartupBanner()
		runInteractiveWizard()
		return
	}

	config, ok := parseConfig()
	if !ok {
		return
	}

	if !config.Quiet {
		printStartupBanner()
	}

	var out io.Writer = os.Stdout
	if config.Output != "stdout" && config.Output != "" {
		f, err := os.Create(config.Output)
		if err != nil {
			if !config.Quiet {
				fmt.Fprintf(os.Stderr, "error creating output file: %v\n", err)
			}
			os.Exit(1)
		}
		defer f.Close()
		out = f
	}

	runCoreLogic(config, out)
}

func printStartupBanner() {
	fmt.Fprint(os.Stderr, startupBanner)
}

func parseConfig() (*Config, bool) {
	config := &Config{}

	var output, outputAlias string
	flag.StringVar(&output, "output", "stdout", "")
	flag.StringVar(&outputAlias, "o", "stdout", "")

	var help, helpAlias bool
	flag.BoolVar(&help, "help", false, "")
	flag.BoolVar(&helpAlias, "h", false, "")

	var quiet, quietAlias bool
	flag.BoolVar(&quiet, "quiet", false, "")
	flag.BoolVar(&quietAlias, "q", false, "")

	var ignorePatterns, ignorePatternsAlias string
	flag.StringVar(&ignorePatterns, "ignore", "", "")
	flag.StringVar(&ignorePatternsAlias, "i", "", "")

	flag.BoolVar(&config.NoGitignore, "no-gitignore", false, "")
	flag.BoolVar(&config.DefaultIgnores, "default-ignores", true, "")
	flag.StringVar(&config.SirantaIgnoreFile, "siranta-ignore", ".sirantaignore", "")

	var maxSizeOld int64
	flag.Int64Var(&maxSizeOld, "max-size", -1, "")
	flag.StringVar(&config.MaxFileSizeStr, "max-file-size", "1MB", "")
	flag.IntVar(&config.MaxTokens, "max-tokens", 0, "")
	flag.IntVar(&config.MinLines, "min-lines", 0, "")

	flag.BoolVar(&config.IncludeDiff, "include-diff", false, "")
	flag.BoolVar(&config.IncludeGitLog, "include-git-log", false, "")
	flag.BoolVar(&config.IncludeSchemas, "include-schemas", false, "")

	var format, formatAlias string
	flag.StringVar(&format, "format", "xml", "")
	flag.StringVar(&formatAlias, "f", "xml", "")

	flag.BoolVar(&config.Compress, "compress", false, "")
	flag.BoolVar(&config.IncludeToc, "include-toc", false, "")

	var dirFlag string
	flag.StringVar(&dirFlag, "dir", ".", "")

	flag.Usage = func() {
		printCustomHelp()
	}

	flag.Parse()

	if help || helpAlias {
		printCustomHelp()
		return nil, false
	}

	config.Output = output
	if outputAlias != "stdout" && output == "stdout" {
		config.Output = outputAlias
	}

	config.Quiet = quiet || quietAlias

	config.Format = strings.ToLower(format)
	if formatAlias != "xml" && format == "xml" {
		config.Format = strings.ToLower(formatAlias)
	}
	if config.Format != "xml" && config.Format != "json" && config.Format != "markdown" {
		fmt.Fprintf(os.Stderr, "Error: invalid format %q. Choices are: xml, json, markdown\n", config.Format)
		os.Exit(1)
	}

	config.IgnorePatterns = ignorePatterns
	if ignorePatternsAlias != "" && ignorePatterns == "" {
		config.IgnorePatterns = ignorePatternsAlias
	}

	if maxSizeOld != -1 {
		config.MaxFileSize = maxSizeOld
	} else {
		limit, err := parseSize(config.MaxFileSizeStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing max-file-size %q: %v\n", config.MaxFileSizeStr, err)
			os.Exit(1)
		}
		config.MaxFileSize = limit
	}

	config.TargetDir = dirFlag
	if flag.NArg() > 0 {
		config.TargetDir = flag.Arg(0)
	}

	// Normalize target directory path
	absDir, err := filepath.Abs(config.TargetDir)
	if err == nil {
		config.TargetDir = absDir
	}

	return config, true
}

func printCustomHelp() {
	helpText := `Kord - streaming repository parser

Usage: kord [directory] [flags]
       kord start (runs the interactive wizard)

Categories and Flags:

Basic Usage:
  -o, --output <string>       Writes the generated codebase context to a specific file target instead of printing it to stdout. (default: "stdout")
                              Example: kord . -o context.xml
  -h, --help                  Displays detailed documentation, CLI syntax helpers, and descriptions of all arguments.
  -q, --quiet                 Suppresses logging output, warnings, and file processing progress reports. Useful for scripts.
                              Example: kord . --quiet -o out.xml

Filtering & Ignore Rules:
  -i, --ignore <string>       Appends extra custom glob patterns (comma-separated) to the active exclude list. (default: "")
                              Example: kord . --ignore "*.test.js,docs/**,scripts/"
  --no-gitignore              Bypasses scanner lookup of local .gitignore files. Collects all files matching standard limits.
                              Example: kord . --no-gitignore
  --default-ignores           Toggles whether built-in global defaults (e.g. .git/, node_modules/, lockfiles) should be ignored. (default: true)
                              Example: kord . --default-ignores=false
  --siranta-ignore <string>   Specifies a custom configuration filename containing target files to exclude specifically from LLM prompt bundling. (default: ".sirantaignore")
                              Example: kord . --siranta-ignore config/pack.ignore

Limits & Thresholds:
  --max-file-size <string>    Excludes files whose sizes exceed the specified limit (supports units: B, KB, MB). Saves attention window. (default: "1MB")
                              Example: kord . --max-file-size 250KB
  --max-tokens <int>          Instructs the compiler to cease ingestion or trigger an error warning if estimated total tokens exceed threshold. (default: 0 (unlimited))
                              Example: kord . --max-tokens 800000
  --min-lines <int>           Excludes short files with line counts below the threshold (useful to skip boilerplate or small configs). (default: 0)
                              Example: kord . --min-lines 10

Metadata Injection:
  --include-diff              Appends local git modification changes inside file tags as attributes (e.g. status="modified").
                              Example: kord . --include-diff
  --include-git-log           Prepends a chronological log summary of the last 5 commits in the header of the output codebase document.
                              Example: kord . --include-git-log
  --include-schemas           Autodetects relational schemas (Prisma, SQL, GraphQL files) and bundles them in a dedicated <schema> wrapper.
                              Example: kord . --include-schemas

Format & Compression:
  -f, --format <string>       Selects output layout schema (choices: xml, json, markdown). XML is heavily recommended for modern LLMs. (default: "xml")
                              Example: kord . --format json
  --compress                  Removes unnecessary indent spaces and empty carriage returns inside packed code files to conserve token counts.
                              Example: kord . --compress
  --include-toc               Generates an XML directory hierarchy map at the beginning of the codebase tag for visual attention routing.
                              Example: kord . --include-toc
`
	fmt.Fprint(os.Stderr, helpText)
}

func parseSize(s string) (int64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	var multiplier int64 = 1
	if strings.HasSuffix(s, "KB") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	} else if strings.HasSuffix(s, "MB") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "GB") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	} else if strings.HasSuffix(s, "B") {
		multiplier = 1
		s = strings.TrimSuffix(s, "B")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("invalid size")
	}
	var val int64
	_, err := fmt.Sscan(s, &val)
	if err != nil {
		return 0, err
	}
	return val * multiplier, nil
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
	if outFileName == "" {
		outFileName = "stdout"
	}

	config := &Config{
		TargetDir:         targetDir,
		Output:            outFileName,
		Format:            "xml",
		DefaultIgnores:    true,
		MaxFileSizeStr:    "1MB",
		MaxFileSize:       1024 * 1024,
		SirantaIgnoreFile: ".sirantaignore",
	}

	var out io.Writer = os.Stdout
	if outFileName != "stdout" {
		if !strings.HasSuffix(strings.ToLower(outFileName), ".xml") {
			outFileName += ".xml"
		}
		f, err := os.Create(outFileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		out = f
	}

	runCoreLogic(config, out)
}

func runCoreLogic(config *Config, out io.Writer) {
	ignoreEngine := NewIgnoreEngine(config)

	var outFileInfo os.FileInfo
	if outFile, ok := out.(*os.File); ok {
		if info, err := outFile.Stat(); err == nil {
			outFileInfo = info
		}
	}

	tc := &TokenCounter{W: out}

	if err := traverseDirectory(config, ignoreEngine, tc, outFileInfo); err != nil {
		if !config.Quiet {
			fmt.Fprintf(os.Stderr, "error during traversal: %v\n", err)
		}
		os.Exit(1)
	}

	if !config.Quiet {
		fmt.Fprintf(os.Stderr, "Kord: %s conversion completed successfully!\n", strings.ToUpper(config.Format))
	}
}

func traverseDirectory(config *Config, engine *IgnoreEngine, tc *TokenCounter, outFileInfo os.FileInfo) error {
	var firstFileWritten bool
	var encoder *xml.Encoder

	if config.Format == "xml" {
		encoder = xml.NewEncoder(tc)
		encoder.Indent("", "  ")
		fmt.Fprint(tc, xml.Header)
		rootStart := xml.StartElement{Name: xml.Name{Local: "repository"}}
		if err := encoder.EncodeToken(rootStart); err != nil {
			return err
		}
		
		if config.IncludeToc {
			tocText := generateTOC(config, engine)
			if err := writeXMLFile(tc, encoder, "toc", "", "", tocText, ""); err != nil {
				return err
			}
		}
		if config.IncludeGitLog {
			gitLogText := getGitLog(config.TargetDir)
			if gitLogText != "" {
				if err := writeXMLFile(tc, encoder, "git_log", "", "", gitLogText, ""); err != nil {
					return err
				}
			}
		}
	} else if config.Format == "json" {
		fmt.Fprint(tc, "{\n  \"repository\": {\n")
		var fields []string
		if config.IncludeToc {
			tocText := generateTOC(config, engine)
			tocJSON, _ := json.Marshal(tocText)
			fields = append(fields, fmt.Sprintf("    \"toc\": %s", string(tocJSON)))
		}
		if config.IncludeGitLog {
			gitLogText := getGitLog(config.TargetDir)
			if gitLogText != "" {
				gitLogJSON, _ := json.Marshal(gitLogText)
				fields = append(fields, fmt.Sprintf("    \"git_log\": %s", string(gitLogJSON)))
			}
		}
		for _, field := range fields {
			fmt.Fprint(tc, field+",\n")
		}
		fmt.Fprint(tc, "    \"files\": [\n")
	} else if config.Format == "markdown" {
		fmt.Fprintln(tc, "# Repository Context")
		fmt.Fprintln(tc)
		if config.IncludeToc {
			fmt.Fprintln(tc, "## Table of Contents")
			fmt.Fprintln(tc, "```")
			fmt.Fprint(tc, generateTOC(config, engine))
			fmt.Fprintln(tc, "```")
			fmt.Fprintln(tc)
		}
		if config.IncludeGitLog {
			fmt.Fprintln(tc, "## Git Log")
			fmt.Fprintln(tc, "```")
			fmt.Fprint(tc, getGitLog(config.TargetDir))
			fmt.Fprintln(tc, "```")
			fmt.Fprintln(tc)
		}
	}

	var gitStatusMap map[string]string
	if config.IncludeDiff {
		gitStatusMap = getGitStatusMap(config.TargetDir)
	}

	err := filepath.WalkDir(config.TargetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if !config.Quiet {
				fmt.Fprintf(os.Stderr, "error accessing path %q: %v\n", path, err)
			}
			return nil
		}

		if config.MaxTokens > 0 && (tc.Count/4) >= config.MaxTokens {
			return ErrTokenLimitExceeded
		}

		if engine.IsIgnored(path, d.IsDir(), config.TargetDir) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				if !config.Quiet {
					fmt.Fprintf(os.Stderr, "error getting info for %q: %v\n", path, err)
				}
				return nil
			}

			if outFileInfo != nil && os.SameFile(info, outFileInfo) {
				if !config.Quiet {
					fmt.Fprintf(os.Stderr, "Kord: Skipping output file %s\n", path)
				}
				return nil
			}

			relPath, _ := filepath.Rel(config.TargetDir, path)
			relPath = filepath.ToSlash(relPath)

			if config.MinLines > 0 {
				lines, err := countLines(path)
				if err != nil {
					if !config.Quiet {
						fmt.Fprintf(os.Stderr, "error counting lines for %q: %v\n", path, err)
					}
					return nil
				}
				if lines < config.MinLines {
					return nil
				}
			}

			tagName := "file"
			if config.IncludeSchemas && isSchemaFile(path) {
				tagName = "schema"
			}

			var status string
			if config.IncludeDiff {
				absPath, err := filepath.Abs(path)
				if err == nil {
					status = gitStatusMap[absPath]
				}
			}

			ext := strings.ToLower(filepath.Ext(path))
			currentLimit := config.MaxFileSize
			if ext == ".md" || ext == ".txt" || ext == ".mdx" {
				currentLimit *= 10
			}

			if info.Size() > currentLimit {
				if !config.Quiet {
					fmt.Fprintf(os.Stderr, "Kord: Skipping content of %s (Size: %d bytes exceeds limit)\n", path, info.Size())
				}
				return writeRecord(config, tc, encoder, tagName, relPath, status, "", "size_limit_exceeded", &firstFileWritten)
			}

			if ext == ".svg" {
				if !config.Quiet {
					fmt.Fprintf(os.Stderr, "Kord: Skipping content of %s (SVG bloat omitted)\n", path)
				}
				return writeRecord(config, tc, encoder, tagName, relPath, status, "", "svg_bloat_omitted", &firstFileWritten)
			}

			content, err := getFileContent(path, config.Compress)
			if err != nil {
				if err == ErrBinaryFile {
					return nil
				}
				if !config.Quiet {
					fmt.Fprintf(os.Stderr, "error reading file %q: %v\n", path, err)
				}
				return nil
			}

			return writeRecord(config, tc, encoder, tagName, relPath, status, content, "", &firstFileWritten)
		}

		return nil
	})

	if err != nil && err != ErrTokenLimitExceeded {
		return err
	}

	if config.Format == "xml" {
		if err == ErrTokenLimitExceeded {
			encoder.Flush()
			fmt.Fprintf(tc, "\n  <!-- WARNING: Ingestion ceased because estimated token limit exceeded: %d tokens -->\n", config.MaxTokens)
			fmt.Fprintln(tc, "</repository>")
		} else {
			rootEnd := xml.EndElement{Name: xml.Name{Local: "repository"}}
			if err := encoder.EncodeToken(rootEnd); err != nil {
				return err
			}
			if err := encoder.Flush(); err != nil {
				return err
			}
			fmt.Fprintln(tc)
		}
	} else if config.Format == "json" {
		if err == ErrTokenLimitExceeded {
			fmt.Fprintf(tc, "\n    ],\n    \"token_limit_exceeded\": true,\n    \"max_tokens_limit\": %d\n  }\n}\n", config.MaxTokens)
		} else {
			fmt.Fprint(tc, "\n    ]\n  }\n}\n")
		}
	} else if config.Format == "markdown" {
		if err == ErrTokenLimitExceeded {
			fmt.Fprintf(tc, "\n> [!WARNING]\n> Ingestion ceased because estimated token limit exceeded: %d tokens.\n", config.MaxTokens)
		}
	}

	return nil
}

func writeXMLFile(out io.Writer, encoder *xml.Encoder, tagName string, path string, status string, content string, omittedReason string) error {
	var attrs []xml.Attr
	if path != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "path"}, Value: path})
	}
	if status != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "status"}, Value: status})
	}
	if omittedReason != "" {
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "omitted"}, Value: omittedReason})
	}

	start := xml.StartElement{Name: xml.Name{Local: tagName}, Attr: attrs}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}

	if omittedReason == "" && content != "" {
		if err := encoder.Flush(); err != nil {
			return err
		}
		if _, err := fmt.Fprint(out, "<![CDATA["); err != nil {
			return err
		}
		if _, err := fmt.Fprint(out, content); err != nil {
			return err
		}
		if _, err := fmt.Fprint(out, "]]>"); err != nil {
			return err
		}
	}

	if err := encoder.EncodeToken(start.End()); err != nil {
		return err
	}
	return encoder.Flush()
}

func writeRecord(config *Config, tc *TokenCounter, encoder *xml.Encoder, tagName string, path string, status string, content string, omittedReason string, firstFileWritten *bool) error {
	if config.Format == "xml" {
		return writeXMLFile(tc, encoder, tagName, path, status, content, omittedReason)
	} else if config.Format == "json" {
		if *firstFileWritten {
			fmt.Fprint(tc, ",\n")
		} else {
			*firstFileWritten = true
		}

		jsonFile := struct {
			Path    string `json:"path"`
			Tag     string `json:"tag"`
			Status  string `json:"status,omitempty"`
			Content string `json:"content,omitempty"`
			Omitted string `json:"omitted,omitempty"`
		}{
			Path:    path,
			Tag:     tagName,
			Status:  status,
			Content: content,
			Omitted: omittedReason,
		}

		data, err := json.MarshalIndent(jsonFile, "      ", "  ")
		if err != nil {
			return err
		}
		_, err = tc.Write(data)
		return err
	} else if config.Format == "markdown" {
		fmt.Fprintf(tc, "## %s: %s\n", strings.Title(tagName), path)
		fmt.Fprintf(tc, "Path: %s\n", path)
		if status != "" {
			fmt.Fprintf(tc, "Status: %s\n", status)
		}
		if omittedReason != "" {
			fmt.Fprintf(tc, "Omitted: %s\n", omittedReason)
			fmt.Fprintln(tc)
		} else {
			fmt.Fprintln(tc)
			lang := getMarkdownLang(path)
			fmt.Fprintf(tc, "```%s\n", lang)
			fmt.Fprint(tc, content)
			if !strings.HasSuffix(content, "\n") {
				fmt.Fprintln(tc)
			}
			fmt.Fprintln(tc, "```")
			fmt.Fprintln(tc)
		}
	}
	return nil
}

// IgnoreEngine parses and evaluates .gitignore rules.
type IgnoreEngine struct {
	exactDirs      map[string]bool
	exactFiles     map[string]bool
	suffixes       []string
	prefixes       []string
	customPatterns []string
}

// NewIgnoreEngine creates and populates IgnoreEngine.
func NewIgnoreEngine(config *Config) *IgnoreEngine {
	engine := &IgnoreEngine{
		exactDirs:  make(map[string]bool),
		exactFiles: make(map[string]bool),
		suffixes:   make([]string, 0),
		prefixes:   make([]string, 0),
	}

	if config.DefaultIgnores {
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

		engine.suffixes = append(engine.suffixes,
			".png", ".jpg", ".jpeg", ".gif", ".ico", ".webp",
			".lock", "go.sum", ".min.js", ".min.css", ".map",
			".exe", ".dll", ".so", ".dylib", ".bin", ".zip", ".tar.gz", ".rar", ".7z", ".pdf", ".pyc", ".class",
			".pem", ".key",
		)

		engine.prefixes = append(engine.prefixes, ".env")
	}

	if !config.NoGitignore {
		engine.loadIgnoreFile(filepath.Join(config.TargetDir, ".gitignore"))
	}

	if config.SirantaIgnoreFile != "" {
		engine.loadIgnoreFile(filepath.Join(config.TargetDir, config.SirantaIgnoreFile))
	}

	if config.IgnorePatterns != "" {
		patterns := strings.Split(config.IgnorePatterns, ",")
		for _, pat := range patterns {
			pat = strings.TrimSpace(pat)
			if pat != "" {
				engine.addPattern(pat)
			}
		}
	}

	return engine
}

func (ie *IgnoreEngine) loadIgnoreFile(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ie.addPattern(line)
	}
}

func (ie *IgnoreEngine) addPattern(pattern string) {
	isDirRule := strings.HasSuffix(pattern, "/")
	if isDirRule {
		pattern = strings.TrimSuffix(pattern, "/")
	}

	if strings.ContainsAny(pattern, "*?[]") || strings.Contains(pattern, "/") {
		ie.customPatterns = append(ie.customPatterns, pattern)
	} else {
		if strings.HasPrefix(pattern, "*") {
			ie.suffixes = append(ie.suffixes, strings.TrimPrefix(pattern, "*"))
		} else if strings.HasSuffix(pattern, "*") {
			ie.prefixes = append(ie.prefixes, strings.TrimSuffix(pattern, "*"))
		} else {
			if isDirRule {
				ie.exactDirs[pattern] = true
			} else {
				ie.exactFiles[pattern] = true
				ie.exactDirs[pattern] = true
			}
		}
	}
}

func (ie *IgnoreEngine) IsIgnored(path string, isDir bool, targetDir string) bool {
	base := filepath.Base(path)

	if isDir {
		if ie.exactDirs[base] {
			return true
		}
	} else {
		if ie.exactFiles[base] {
			return true
		}
	}

	for _, sfx := range ie.suffixes {
		if strings.HasSuffix(base, sfx) {
			return true
		}
	}

	for _, pfx := range ie.prefixes {
		if strings.HasPrefix(base, pfx) {
			return true
		}
	}

	relPath, err := filepath.Rel(targetDir, path)
	if err != nil {
		relPath = path
	}
	relPath = filepath.ToSlash(relPath)

	// Check if any parent directory component of the relative path is in ie.exactDirs
	parts := strings.Split(relPath, "/")
	for i := 0; i < len(parts)-1; i++ {
		if ie.exactDirs[parts[i]] {
			return true
		}
	}

	for _, pat := range ie.customPatterns {
		cleanPat := strings.TrimSuffix(pat, "/**")
		if strings.HasSuffix(pat, "/**") {
			if strings.HasPrefix(relPath, cleanPat+"/") || relPath == cleanPat {
				return true
			}
		}
		if strings.HasSuffix(pat, "/") {
			cleanPat = strings.TrimSuffix(pat, "/")
			if strings.HasPrefix(relPath, cleanPat+"/") || relPath == cleanPat {
				return true
			}
		}
		if matched, _ := filepath.Match(pat, base); matched {
			return true
		}
		if matched, _ := filepath.Match(pat, relPath); matched {
			return true
		}
	}

	return false
}

func isSchemaFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".prisma" || ext == ".sql" || ext == ".graphql" || ext == ".gql"
}

func getMarkdownLang(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "tsx"
	case ".jsx":
		return "jsx"
	case ".py":
		return "python"
	case ".json":
		return "json"
	case ".md":
		return "markdown"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".sh":
		return "bash"
	case ".yml", ".yaml":
		return "yaml"
	case ".sql":
		return "sql"
	case ".prisma":
		return "prisma"
	case ".graphql", ".gql":
		return "graphql"
	default:
		return strings.TrimPrefix(ext, ".")
	}
}

func getGitStatusMap(targetDir string) map[string]string {
	statusMap := make(map[string]string)
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = targetDir
	output, err := cmd.Output()
	if err != nil {
		return statusMap
	}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 3 {
			continue
		}
		status := strings.TrimSpace(line[:2])
		filePath := strings.TrimSpace(line[3:])
		absPath, err := filepath.Abs(filepath.Join(targetDir, filePath))
		if err != nil {
			continue
		}
		var statusText string
		switch {
		case strings.Contains(status, "M"):
			statusText = "modified"
		case strings.Contains(status, "A"):
			statusText = "added"
		case strings.Contains(status, "?"):
			statusText = "untracked"
		default:
			statusText = "modified"
		}
		statusMap[absPath] = statusText
	}
	return statusMap
}

func getGitLog(targetDir string) string {
	cmd := exec.Command("git", "log", "-n", "5", "--oneline")
	cmd.Dir = targetDir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(output)
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	buf := make([]byte, 32*1024)
	count := 0
	hasContent := false
	for {
		n, err := f.Read(buf)
		if n > 0 {
			hasContent = true
			for i := 0; i < n; i++ {
				if buf[i] == '\n' {
					count++
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, err
		}
	}
	if hasContent && count == 0 {
		count = 1
	}
	return count, nil
}

func compressString(content string) string {
	var sb strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func getFileContent(path string, compress bool) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if bytes.IndexByte(data, 0) != -1 {
		return "", ErrBinaryFile
	}
	content := string(data)
	if compress {
		content = compressString(content)
	}
	return content, nil
}

func generateTOC(config *Config, ignoreEngine *IgnoreEngine) string {
	var files []string
	filepath.WalkDir(config.TargetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ignoreEngine.IsIgnored(path, d.IsDir(), config.TargetDir) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			rel, err := filepath.Rel(config.TargetDir, path)
			if err == nil {
				files = append(files, rel)
			}
		}
		return nil
	})

	return buildASCIIHierarchy(files)
}

type Node struct {
	Name     string
	Children map[string]*Node
	IsDir    bool
}

func buildASCIIHierarchy(paths []string) string {
	root := &Node{Name: ".", Children: make(map[string]*Node), IsDir: true}
	for _, p := range paths {
		parts := strings.Split(filepath.ToSlash(p), "/")
		curr := root
		for i, part := range parts {
			isLast := i == len(parts)-1
			if curr.Children[part] == nil {
				curr.Children[part] = &Node{
					Name:     part,
					Children: make(map[string]*Node),
					IsDir:    !isLast,
				}
			}
			curr = curr.Children[part]
		}
	}

	var sb strings.Builder
	var printNode func(n *Node, indent string, isLastChild bool)
	printNode = func(n *Node, indent string, isLastChild bool) {
		if n.Name != "." {
			sb.WriteString(indent)
			if isLastChild {
				sb.WriteString("└── ")
			} else {
				sb.WriteString("├── ")
			}
			sb.WriteString(n.Name)
			if n.IsDir {
				sb.WriteString("/")
			}
			sb.WriteString("\n")
		}

		var keys []string
		for k := range n.Children {
			keys = append(keys, k)
		}
		sortKeys(n.Children, keys)

		nextIndent := indent
		if n.Name != "." {
			if isLastChild {
				nextIndent += "    "
			} else {
				nextIndent += "│   "
			}
		}

		for idx, key := range keys {
			last := idx == len(keys)-1
			printNode(n.Children[key], nextIndent, last)
		}
	}

	printNode(root, "", true)
	return sb.String()
}

func sortKeys(children map[string]*Node, keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		ni := children[keys[i]]
		nj := children[keys[j]]
		if ni.IsDir != nj.IsDir {
			return ni.IsDir
		}
		return keys[i] < keys[j]
	})
}
