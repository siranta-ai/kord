# Kord By Siranta

**Kord** is an ultra-fast, zero-dependency CLI utility written in Go, designed to package entire codebases into a single streamed XML output. It is engineered explicitly to process massive repositories with minimal memory usage and high throughput, making it the perfect tool to prepare codebase context for Large Language Models (LLMs) or vector databases.

---

## Architecture Overview

Kord operates as a streaming ingestion pipeline. Unlike traditional tools that load entire directories or large file arrays into memory before outputting, Kord traverses the filesystem and writes XML tokens sequentially to the output stream. This design ensures that memory consumption remains virtually constant, regardless of the repository size.

```
[Filesystem Walk] ➔ [Ignore Engine] ➔ [Filters (Binary/SVG/Size)] ➔ [Stream XML Encoder] ➔ [Output / stdout]
```

---

## Key Features

1.  **Streaming XML Output**: Processes and encodes files on-the-fly.
2.  **Zero-Regex Ignore Engine**: Groups rules into high-performance buckets evaluated using rapid $O(1)$ map lookups and simple string comparisons instead of heavy regular expression patterns.
3.  **Binary File Detection**: Reads the first 512 bytes of files to check for a null byte (`0x00`), skipping binary assets dynamically without bloating the stream.
4.  **Flexible Size Constraints**: Skips content for files exceeding configured limits, but prints their file path tag with an explanation, keeping the LLM informed of their existence.
5.  **Interactive Setup Wizard**: Provides a step-by-step interactive CLI mode.
6.  **Static Compilation & No Dependencies**: Relies solely on the Go standard library, compiling down to a single standalone binary.

---

## Output XML Schema

The output is wrapped in a `<repository>` root tag. Each file is encapsulated in a `<file>` tag, with its relative file path stored in the `path` attribute, and its raw content wrapped safely in a `CDATA` block.

```xml
<?xml version="1.0" encoding="UTF-8"?>
<repository>
  <!-- Normal file with content -->
  <file path="main.go"><![CDATA[package main...]]></file>
  
  <!-- Exceeds file size limit (content skipped to save context space) -->
  <file path="large_data.json" omitted="size_limit_exceeded"></file>
  
  <!-- SVG file (vector coordinates skipped to prevent LLM bloat/hallucination) -->
  <file path="assets/logo.svg" omitted="svg_bloat_omitted"></file>
</repository>
```

---

## Usage

Kord can be executed directly as a command-line tool, running against the current directory by default, or configured via flags.

### 1. Basic CLI Commands

```bash
# Run against the current directory and output to stdout
kord

# Run against a specific directory
kord -dir /path/to/project

# Specify a custom size limit (e.g., 20KB base size limit instead of 50KB)
kord -max-size 20000

# Specify a custom ignore file
kord -ignore custom.gitignore

# Pipe the output directly to a file
kord -dir . > codebase.xml
```

### 2. Interactive Wizard Mode

To run an interactive guide that prompts you for the target directory and output location, run:

```bash
kord start
```

### Command Flags

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `-dir` | `string` | `.` | The target directory to traverse. |
| `-ignore` | `string` | `.gitignore` | The path to the ignore rules file to parse. |
| `-max-size` | `int64` | `50000` | The maximum size (in bytes) allowed for standard file contents before they are omitted. |

---

## The Ignore Engine

To ensure maximum walk speed, Kord uses a specialized `IgnoreEngine` that categorizes rules into four quick-check buckets:
*   **Exact Directories**: Directories skipped immediately during the walk (prevents traversing subdirectories).
*   **Exact Files**: Exact file names to skip.
*   **Suffixes**: Matches patterns starting with `*` (e.g., `*.log`, `*.png`).
*   **Prefixes**: Matches patterns ending with `*` (e.g., `temp*`).

### Hardcoded Default Ignores

The engine automatically ignores heavy, metadata, and compilation folders/files even if they are not listed in your `.gitignore` file:

*   **Directories**: `.git`, `node_modules`, `vendor`, `.next`, `dist`, `build`, `.gradle`, `venv`, `.venv`, `target`, `obj`, `__pycache__`, `.dart_tool`, `Pods`.
*   **Suffixes**: `.png`, `.jpg`, `.jpeg`, `.gif`, `.ico`, `.webp`, `.lock`, `go.sum`, `.min.js`, `.min.css`, `.map`, `.exe`, `.dll`, `.so`, `.dylib`, `.bin`, `.zip`, `.tar.gz`, `.rar`, `.7z`, `.pdf`, `.pyc`, `.class`.

### Ignore Engine Limitations

> [!NOTE]
> Since the ignore engine does not use complex regular expressions or absolute-path resolution to stay fast:
> 1. Matches are performed against the **base name** (filename or directory name) of each path.
> 2. Complex nested glob rules (like `app/src/*.test.js` or `dir/**/*.log`) are not fully evaluated. Only the base name components or prefixes/suffixes (e.g. `*.test.js`) are evaluated.

---

## Traversal & Filtering Logic

When traversing a directory, Kord applies the following filters to each path:

### 1. Loop Prevention
If you output the streaming XML to a file located inside the directory being traversed (e.g. `kord > output.xml`), Kord compares the file descriptor (`FileInfo`) of the output writer with each walked file. It automatically skips reading its own output file to prevent an infinite recursive write loop.

### 2. Suffix Size Multipliers
For human-readable documentation files, Kord automatically extends the file size limit by **10x** of the configured `-max-size` flag:
*   **Markdown/Text Files** (`.md`, `.txt`, `.mdx`): Limit = `-max-size` $\times$ 10.
*   **All Other Files**: Limit = `-max-size`.

Files exceeding this threshold are written as empty tags with `omitted="size_limit_exceeded"`.

### 3. SVG Optimization
SVG images contain extensive XML coordinates which are unreadable by LLMs and inflate context length. Kord intercepts `.svg` files, skips reading their contents, and writes them with `omitted="svg_bloat_omitted"` to ensure the LLM knows the file is present without receiving the bloat.

### 4. Binary File Check
Before reading any file, Kord pulls the first 512 bytes. If a null byte (`0x00`) is found, the file is classified as binary (e.g., compressed data, compiled binaries, non-UTF-8 assets) and is completely excluded from the XML output.

---

## Developer Guide

### Prerequisites
*   Go 1.25 or higher

### Running Tests

We maintain a comprehensive suite of tests covering ignore rules, XML streaming format, size limit multipliers, binary filters, and self-loop detection.

```bash
# Run all tests in verbose mode
go test -v ./...
```

### Compilation

You can cross-compile Kord binaries statically across multiple platforms. Run the provided script:

```bash
# Run build script (Unix/WSL/Git Bash)
./build.sh
```

This compiles statically linked binaries without debug symbols (`-ldflags="-s -w"`) to minimize size, saving output files to the `bin/` directory:
*   `bin/kord-linux-amd64`
*   `bin/kord-linux-arm64`
*   `bin/kord-darwin-amd64`
*   `bin/kord-darwin-arm64`
*   `bin/kord-windows-amd64.exe`
