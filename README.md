# Kord By Siranta

**Kord** is an ultra-fast, zero-dependency CLI utility designed to package entire codebases into a single streamed XML output. It is built natively in Go using only the standard library and engineered explicitly to process massive repositories without destroying memory or performance.

## What is Kord?

Kord is an ingestion preparation tool. It traverses a directory tree, respects your `.gitignore` logic, automatically drops binary files, and funnels your text-based source code into an XML structure. This structure is highly optimized for feeding large codebases into Large Language Models (LLMs) or vector databases (like the Siranta Graph Engine).

### Key Features
- **Streaming Architecture**: Kord processes and outputs files iteratively. It never loads the entire repository into memory, ensuring minimal RAM consumption.
- **Zero-Regex Ignore Engine**: Evaluates `.gitignore` rules using rapid O(1) map lookups and simple string comparisons instead of slow regular expressions.
- **Auto-Pruning**: Hardcoded bypasses for heavy, irrelevant directories (`.git`, `node_modules`, `vendor`) to guarantee lightning-fast filesystem walks.
- **Binary File Detection**: Implements extremely fast null-byte (`0x00`) detection during reads to automatically skip compiled assets and executables without bloating the output stream.
- **Zero Dependencies**: Built 100% on the Go standard library.
- **Pure Static Binaries**: Compiles into highly minimized, statically-linked executables that run anywhere.

## Output Format

Kord outputs a stream directly to `stdout` wrapped in a `<repository>` root tag. Individual files are enclosed in `<file>` tags with their relative path as an attribute, and their contents safely escaped in `CDATA` blocks.

```xml
<?xml version="1.0" encoding="UTF-8"?>
<repository>
  <file path="main.go"><![CDATA[package main...]]></file>
  <file path="README.md"><![CDATA[# Kord By Siranta...]]></file>
</repository>
```

## Usage

Run Kord directly via Go or use a compiled binary.

```bash
# Run against the current directory
kord

# Specify a target directory
kord -dir /path/to/project

# Specify a custom ignore file
kord -ignore custom.gitignore

# Pipe the output directly to a file
kord > codebase.xml
```

### Flags
- `-dir`: The target directory to traverse. (Default: `.`)
- `-ignore`: The path to the custom ignore rules file. (Default: `.gitignore`)

## Building

Kord provides a `build.sh` script to easily cross-compile static binaries for multiple operating systems and architectures. 

To compile Kord, run:
```bash
./build.sh
```

This sets `CGO_ENABLED=0` and strips debug symbols to minimize file size, outputting standalone binaries into the `bin/` directory:
- `kord-linux-amd64`
- `kord-linux-arm64`
- `kord-darwin-amd64` (macOS Intel)
- `kord-darwin-arm64` (macOS Apple Silicon)
- `kord-windows-amd64.exe`
