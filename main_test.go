package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		hasError bool
	}{
		{"1MB", 1024 * 1024, false},
		{"250KB", 250 * 1024, false},
		{"100B", 100, false},
		{"5B", 5, false},
		{"  500  ", 500, false},
		{"", 0, true},
		{"invalid", 0, true},
	}

	for _, tc := range tests {
		res, err := parseSize(tc.input)
		if tc.hasError {
			if err == nil {
				t.Errorf("expected error for %q, got nil", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tc.input, err)
			}
			if res != tc.expected {
				t.Errorf("expected %d for %q, got %d", tc.expected, tc.input, res)
			}
		}
	}
}

func TestIgnoreEngineRules(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "kord_test_ignore")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a dummy .gitignore file
	gitignoreContent := `
*.log
temp*
`
	err = os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignoreContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create a dummy .sirantaignore file
	sirantaContent := `
secrets.txt
`
	err = os.WriteFile(filepath.Join(tmpDir, ".sirantaignore"), []byte(sirantaContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	config := &Config{
		TargetDir:         tmpDir,
		DefaultIgnores:    true,
		NoGitignore:       false,
		SirantaIgnoreFile: ".sirantaignore",
		IgnorePatterns:    "*.test.js,docs/**,scripts/",
	}

	engine := NewIgnoreEngine(config)

	// Test default ignores
	if !engine.IsIgnored(filepath.Join(tmpDir, "node_modules"), true, tmpDir) {
		t.Error("expected node_modules to be ignored")
	}
	if !engine.IsIgnored(filepath.Join(tmpDir, "image.png"), false, tmpDir) {
		t.Error("expected image.png to be ignored")
	}

	// Test gitignore rules
	if !engine.IsIgnored(filepath.Join(tmpDir, "app.log"), false, tmpDir) {
		t.Error("expected app.log to be ignored via .gitignore")
	}
	if !engine.IsIgnored(filepath.Join(tmpDir, "temp_data.csv"), false, tmpDir) {
		t.Error("expected temp_data.csv to be ignored via .gitignore")
	}

	// Test siranta ignore rules
	if !engine.IsIgnored(filepath.Join(tmpDir, "secrets.txt"), false, tmpDir) {
		t.Error("expected secrets.txt to be ignored via .sirantaignore")
	}

	// Test custom comma-separated patterns
	if !engine.IsIgnored(filepath.Join(tmpDir, "app.test.js"), false, tmpDir) {
		t.Error("expected app.test.js to be ignored via custom patterns")
	}
	if !engine.IsIgnored(filepath.Join(tmpDir, "docs", "file.txt"), false, tmpDir) {
		t.Error("expected files in docs/ to be ignored via custom patterns")
	}
	if !engine.IsIgnored(filepath.Join(tmpDir, "scripts", "run.sh"), false, tmpDir) {
		t.Error("expected files in scripts/ to be ignored via custom patterns")
	}

	// Test allowed file
	if engine.IsIgnored(filepath.Join(tmpDir, "main.go"), false, tmpDir) {
		t.Error("expected main.go to NOT be ignored")
	}
}

func TestMinLines(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "kord_test_lines")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	shortFile := filepath.Join(tmpDir, "short.txt")
	longFile := filepath.Join(tmpDir, "long.txt")

	os.WriteFile(shortFile, []byte("line1\nline2"), 0644)       // 2 lines
	os.WriteFile(longFile, []byte("1\n2\n3\n4\n5\n6\n"), 0644) // 6 lines

	config := &Config{
		TargetDir:      tmpDir,
		DefaultIgnores: false,
		NoGitignore:    true,
		MinLines:       5,
		Format:         "xml",
		MaxFileSize:    50000,
	}

	engine := NewIgnoreEngine(config)
	var buf bytes.Buffer
	tc := &TokenCounter{W: &buf}

	err = traverseDirectory(config, engine, tc, nil)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "long.txt") {
		t.Error("expected long.txt to be included in the output")
	}
	if strings.Contains(output, "short.txt") {
		t.Error("expected short.txt to be excluded due to min-lines threshold")
	}
}

func TestTokenLimit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "kord_test_tokens")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create several files with content
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("this is some content that is relatively long"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("more content that will exceed the token threshold quickly"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file3.txt"), []byte("final content to skip"), 0644)

	config := &Config{
		TargetDir:      tmpDir,
		DefaultIgnores: false,
		NoGitignore:    true,
		MaxTokens:      10, // Very low token limit (approx 40 bytes)
		Format:         "xml",
		MaxFileSize:    50000,
	}

	engine := NewIgnoreEngine(config)
	var buf bytes.Buffer
	tc := &TokenCounter{W: &buf}

	err = traverseDirectory(config, engine, tc, nil)
	// We handle token limit gracefully in traverseDirectory without returning the error up to main
	if err != nil {
		t.Fatalf("expected graceful handling, got error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "token limit exceeded") {
		t.Error("expected output to contain token limit warning message")
	}
}

func TestCompress(t *testing.T) {
	content := "  line1  \n\n\n  line2  \n"
	expected := "line1\nline2\n"

	res := compressString(content)
	if res != expected {
		t.Errorf("expected compressed content %q, got %q", expected, res)
	}
}

func TestFormats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "kord_test_formats")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "schema.sql"), []byte("CREATE TABLE users;"), 0644)

	// 1. Test XML Format with Schema wrap
	configXML := &Config{
		TargetDir:      tmpDir,
		DefaultIgnores: false,
		NoGitignore:    true,
		Format:         "xml",
		IncludeSchemas: true,
		MaxFileSize:    50000,
	}
	engineXML := NewIgnoreEngine(configXML)
	var bufXML bytes.Buffer
	tcXML := &TokenCounter{W: &bufXML}

	err = traverseDirectory(configXML, engineXML, tcXML, nil)
	if err != nil {
		t.Fatal(err)
	}
	outputXML := bufXML.String()
	if !strings.Contains(outputXML, "<schema path=\"schema.sql\"") {
		t.Error("expected XML output to contain schema wrapper for schema.sql")
	}
	if !strings.Contains(outputXML, "<file path=\"main.go\"") {
		t.Error("expected XML output to contain file wrapper for main.go")
	}

	// 2. Test JSON Format
	configJSON := &Config{
		TargetDir:      tmpDir,
		DefaultIgnores: false,
		NoGitignore:    true,
		Format:         "json",
		MaxFileSize:    50000,
	}
	engineJSON := NewIgnoreEngine(configJSON)
	var bufJSON bytes.Buffer
	tcJSON := &TokenCounter{W: &bufJSON}

	err = traverseDirectory(configJSON, engineJSON, tcJSON, nil)
	if err != nil {
		t.Fatal(err)
	}
	outputJSON := bufJSON.String()
	if !strings.HasPrefix(outputJSON, "{") || !strings.HasSuffix(outputJSON, "}\n") {
		t.Errorf("expected valid JSON structure, got: %q", outputJSON)
	}
	if !strings.Contains(outputJSON, `"path": "main.go"`) {
		t.Error("expected JSON output to list main.go")
	}

	// 3. Test Markdown Format
	configMD := &Config{
		TargetDir:      tmpDir,
		DefaultIgnores: false,
		NoGitignore:    true,
		Format:         "markdown",
		MaxFileSize:    50000,
	}
	engineMD := NewIgnoreEngine(configMD)
	var bufMD bytes.Buffer
	tcMD := &TokenCounter{W: &bufMD}

	err = traverseDirectory(configMD, engineMD, tcMD, nil)
	if err != nil {
		t.Fatal(err)
	}
	outputMD := bufMD.String()
	if !strings.Contains(outputMD, "## File: main.go") {
		t.Error("expected Markdown output to contain heading for main.go")
	}
}

func TestTraverseLoopPrevention(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "kord_test_loop")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	outputFile := filepath.Join(tmpDir, "output.xml")
	f, err := os.Create(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	config := &Config{
		TargetDir:      tmpDir,
		DefaultIgnores: false,
		NoGitignore:    true,
		Format:         "xml",
		MaxFileSize:    50000,
	}

	engine := NewIgnoreEngine(config)
	var buf bytes.Buffer
	tc := &TokenCounter{W: &buf}

	// Output file info passed as loop prevention target
	err = traverseDirectory(config, engine, tc, info)
	if err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if strings.Contains(output, "output.xml") {
		// Output file should have been skipped entirely to prevent infinite recursion loop
		t.Error("expected output.xml to be skipped during traversal")
	}
}
