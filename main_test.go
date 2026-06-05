package main

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test IgnoreEngine for .gitignore negation flags edge cases
func TestIgnoreEngine_NegationFlags(t *testing.T) {
	config := &Config{
		DefaultIgnores: false,
	}
	engine := NewIgnoreEngine(config)

	// Add negation patterns - typically they start with "!"
	// Our engine should either un-ignore it or handle the literal.
	// As per current engine design, we are documenting its edge-case behavior.
	engine.addPattern("*.log")
	engine.addPattern("!important.log")

	// Regular match
	if engine.IsIgnored("test.log", false, ".") != true {
		t.Errorf("Expected test.log to be ignored based on *.log")
	}

	// Because the current engine does not explicitly implement overriding for ! flags
	// it treats !important.log as an exact file to ignore.
	// We test the edge case response here.
	if engine.IsIgnored("!important.log", false, ".") != true {
		t.Errorf("Expected !important.log to be matched as exact file due to current engine limitations")
	}
}

// Test custom token limits
func TestTokenLimitExceeded(t *testing.T) {
	var buf bytes.Buffer
	tc := &TokenCounter{W: &buf}

	config := &Config{
		MaxTokens:      5,
		Format:         "xml",
		TargetDir:      ".",
		MaxFileSizeStr: "1MB",
		MaxFileSize:    1024 * 1024,
	}

	tmpDir, err := os.MkdirTemp("", "kord-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config.TargetDir = tmpDir

	testFilePath := filepath.Join(tmpDir, "large.txt")
	// 5 tokens is ~20 bytes, writing 50 bytes ensures limit is exceeded
	largeContent := strings.Repeat("A", 50)
	err = os.WriteFile(testFilePath, []byte(largeContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	engine := NewIgnoreEngine(&Config{})

	// traverseDirectory handles ErrTokenLimitExceeded internally and generates warning output
	err = traverseDirectory(config, engine, tc, nil)
	if err != nil && err != ErrTokenLimitExceeded {
		t.Fatalf("Unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "WARNING: Ingestion ceased because estimated token limit exceeded") {
		t.Errorf("Expected token limit warning in output, got: %s", output)
	}
}

// Test extreme input string validation for XML encoders
func TestXMLEncoder_ExtremeInput(t *testing.T) {
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)

	// An extreme string with CDATA terminator ]]> and other special characters
	extremeInput := "Normal text <![CDATA[ malicious ]]> \x00\x01\x02 \uFFFF <<>> &lt;&gt; & ]]>"
	
	firstFileWritten := false
	config := &Config{Format: "xml"}
	
	tc := &TokenCounter{W: &buf}
	
	err := writeRecord(config, tc, encoder, "file", "test.txt", "", extremeInput, "", &firstFileWritten)
	if err != nil {
		t.Fatalf("Failed to write XML record: %v", err)
	}
	
	encoder.Flush()
	
	output := buf.String()
	
	// Check if the output properly replaced ]]> with ]]]]><![CDATA[>
	if strings.Contains(output, "]]> \x00") {
		t.Errorf("Found unescaped CDATA terminator in output")
	}
	
	if !strings.Contains(output, "]]]]><![CDATA[>") {
		t.Errorf("Expected escaped CDATA terminator, got: %s", output)
	}
}
