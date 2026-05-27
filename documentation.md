# Kord: Developer & Command Reference Documentation

**Kord** is an ultra-fast, zero-dependency command-line utility written in Go. It is designed to traverse, filter, and package entire directories into a single structured, streamed output (XML, JSON, or Markdown). 

Engineered explicitly for feeding complete repository contexts into Large Language Models (LLMs) or vector databases, Kord prioritizes high throughput, constant memory usage, and zero external runtime dependencies.

---

## Table of Contents
1. [Core Design & Architecture](#1-core-design--architecture)
2. [Getting Started](#2-getting-started)
3. [CLI Reference & Commands](#3-cli-reference--commands)
4. [The Ignore Engine & Filtering Rules](#4-the-ignore-engine--filtering-rules)
5. [Traversal Logic & Optimizations](#5-traversal-logic--optimizations)
6. [Output Formats & Schema Specifications](#6-output-formats--schema-specifications)
7. [Developer Reference (Codebase Architecture)](#7-developer-reference-codebase-architecture)

---

## 1. Core Design & Architecture

Unlike traditional codebase packagers that buffer full directories or file arrays in RAM, Kord operates as a **streaming ingestion pipeline**. Filesystem traversal is coupled directly to the output encoder.

```
[Filesystem Walk] ➔ [Ignore Engine] ➔ [Filters (Binary/SVG/Size)] ➔ [Stream Encoder] ➔ [Output Stream]
```

### Architectural Key Concepts
*   **Constant Memory Footprint**: Memory consumption remains flat regardless of repository size. XML and JSON elements are serialized token-by-token directly to standard output or the targeted file.
*   **Zero-Dependency Engine**: Kord is constructed entirely with the Go standard library (`encoding/xml`, `path/filepath`, `io`, etc.), keeping compilation trivial and compile-time artifacts clean.
*   **Stream Encoding & Formatting**: Output contents (specifically source code and plain text files) are automatically wrapped inside `CDATA` blocks for XML or string-encoded for JSON to prevent escaping issues and ensure parsing fidelity by downstream LLM endpoints.

---

## 2. Getting Started

### System Requirements
*   **Go Compiler**: Version `1.22` or newer is required to compile.
*   **Git**: Required for git-based metadata features (such as `--include-diff` or `--include-git-log`).

### Compilation & Installation
Kord supports static linking and cross-compilation out of the box. A build shell script (`build.sh`) is provided for automated multi-platform compilation.

```bash
# Clone the repository and compile statically
git clone https://github.com/siranta-ai/Kord.git
cd Kord
chmod +x build.sh
./build.sh
```

The script builds statically linked binaries (`CGO_ENABLED=0`) with all debug tables and symbols stripped (`-ldflags="-s -w"`) to minimize size. The binaries are placed under `./bin/`:
*   `bin/kord-linux-amd64` (Linux x86_64)
*   `bin/kord-linux-arm64` (Linux ARM 64-bit)
*   `bin/kord-darwin-amd64` (macOS Intel)
*   `bin/kord-darwin-arm64` (macOS Apple Silicon)
*   `bin/kord-windows-amd64.exe` (Windows 64-bit)

To build for your current local platform only:
```bash
go build -ldflags="-s -w" -o kord main.go
```

---

## 3. CLI Reference & Commands

### Basic Command Invocations
*(Note: If you are on Windows and haven't added Kord to your PATH, use `.\kord` instead of `kord`)*

```bash
# Basic execution: Traverse the current directory and output XML directly to stdout
kord

# Traverse a specific target directory
kord -dir /path/to/project

# Pipe output straight to a file target (retains constant memory)
kord -dir . > codebase.xml

# Explicitly write output using the output flag (handles infinite recursion loop prevention)
kord -o codebase.xml
```

### Interactive Setup Wizard
For guided configuration, Kord provides an interactive command-line setup wizard.

```bash
kord start
```
This wizard prompts the user through:
1. **Target Directory**: The directory to analyze (defaults to `.`).
2. **Output Location**: Output file target name (defaults to `stdout`). If a custom filename is entered, Kord appends `.xml` if the suffix is missing, establishes the file handle, and begins processing.

---

### Command Flags

| Flag (Long / Short) | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `--dir` | `string` | `.` | The root directory to walk during execution. |
| `-o`, `--output` | `string` | `stdout` | Writes the generated context to a specific file instead of `stdout`. |
| `-h`, `--help` | `bool` | `false` | Displays help information, category divisions, syntax examples, and flags. |
| `-q`, `--quiet` | `bool` | `false` | Suppresses stderr status logs, warnings, startup banner, and completion notices. |
| `-i`, `--ignore` | `string` | `""` | Appends custom comma-separated patterns to the active exclude list. |
| `--no-gitignore` | `bool` | `false` | Disables scanning and loading rules from local `.gitignore` files. |
| `--default-ignores`| `bool` | `true` | Enables/disables the hardcoded built-in global exclude defaults. |
| `--siranta-ignore` | `string` | `.sirantaignore` | Custom configuration file containing target files to exclude specifically from LLM prompt bundling. |
| `--max-file-size` | `string` | `1MB` | Excludes content of files exceeding this threshold (supports suffix multipliers: `B`, `KB`, `MB`, `GB`). |
| `-max-size` | `int64` | `-1` | *Deprecated*. Backwards compatible integer limit in bytes. Superceded by `--max-file-size`. |
| `--max-tokens` | `int` | `0` | Estimated token boundary limit. Halts traversal or outputs a warning if estimated token count exceeds threshold. `0` indicates unlimited. |
| `--min-lines` | `int` | `0` | Excludes files containing fewer lines than this value (ignores small configs or boilerplate). |
| `--include-diff` | `bool` | `false` | Calls Git status internally to mark files in XML/JSON tags as `status="modified"`, `status="added"`, or `status="untracked"`. |
| `--include-git-log`| `bool` | `false` | Prepends the last 5 commit messages in a chronological git log summary header. |
| `--include-schemas`| `bool` | `false` | Automatically wraps relational schemas (SQL, Prisma, GraphQL) in a specialized `<schema>` tag. |
| `-f`, `--format` | `string` | `xml` | Selection of layout formats. Options: `xml`, `json`, `markdown`. |
| `--compress` | `bool` | `false` | Compresses spaces, carriage returns, and blank lines to optimize context token counts. |
| `--include-toc` | `bool` | `false` | Inserts an ASCII-based directory tree hierarchy structure at the beginning of the context. |

---

## 4. The Ignore Engine & Filtering Rules

Kord uses a zero-regex, highly optimized matching engine to screen paths during directory traversal. This ensures high-performance walks even in directories containing millions of nested items.

### Precedence of Exclude Rules
When Kord evaluates whether to scan or skip a file/directory path, it applies rules in the following order:
1. **Built-in Global Defaults** (if `--default-ignores` is enabled)
2. **Local `.gitignore` File** (if present and not bypassed via `--no-gitignore`)
3. **Custom `.sirantaignore` Config** (if present)
4. **Command line values** passed to `-i` / `--ignore`

### Built-in Default Ignores
By default (`--default-ignores=true`), Kord automatically excludes common heavy directories, compiled build outputs, environment configs, lockfiles, and media formats:

*   **Exact Directories**: `.git`, `node_modules`, `vendor`, `.next`, `dist`, `build`, `.gradle`, `venv`, `.venv`, `target`, `obj`, `__pycache__`, `.dart_tool`, `Pods`
*   **Suffixes (Binary / Media / Locks)**: `.png`, `.jpg`, `.jpeg`, `.gif`, `.ico`, `.webp`, `.lock`, `go.sum`, `.min.js`, `.min.css`, `.map`, `.exe`, `.dll`, `.so`, `.dylib`, `.bin`, `.zip`, `.tar.gz`, `.rar`, `.7z`, `.pdf`, `.pyc`, `.class`, `.pem`, `.key`
*   **Prefixes**: `.env`

### Rule Classification Buckets
Rules parsed from configuration sources are bucketed for $O(1)$ quick check evaluations instead of spinning up expensive regular expression engines:
*   **Exact Directories (`exactDirs`)**: Matches specific directory base names (e.g., `node_modules`, `vendor`).
*   **Exact Files (`exactFiles`)**: Matches exact file names (e.g., `go.sum`, `package-lock.json`).
*   **Suffixes (`suffixes`)**: Parsed from patterns starting with wildcard asterisks (e.g., `*.log`, `*.png`).
*   **Prefixes (`prefixes`)**: Parsed from patterns ending with wildcard asterisks (e.g., `temp*`, `.env*`).
*   **Custom Glob Patterns (`customPatterns`)**: Evaluated using standard `filepath.Match` if the rule contains nested directory indicators (`/`) or wildcard patterns not matching simple prefixes/suffixes. Supports directory boundary definitions:
    *   `pat/**` matches the target directory and all descendants.
    *   `pat/` matches the directory and descendants.

---

## 5. Traversal Logic & Optimizations

Kord incorporates multiple heuristic optimizations during its directory traversal pipeline:

### 1. Loop Prevention
If a user runs Kord and redirects the output stream directly into a file stored within the target directory (e.g. `kord -o output.xml` inside `.`), Kord obtains the unique file descriptor (`FileInfo`) of the output writer. During traversal, it compares the `FileInfo` of each walked file against the output target. If it matches, the file is skipped to prevent an infinite recursive write loop.

### 2. Suffix-Based Size Multiplier
To prevent omitting critical configuration and context files while avoiding massive logs, Kord applies a **10x multiplier** to the configured `--max-file-size` threshold for common human-readable documentation formats:
*   **Files with extensions** `.md`, `.txt`, and `.mdx` are allowed up to **10 times** the limit.
*   **All other files** use the configured limit (default: `1MB`).

### 3. SVG Optimization
Vectors contain extensive coordinate structures that bloat prompt context sizes and trigger LLM model hallucination. Kord identifies `.svg` files, skips reading their coordinates, and outputs an empty tag marked with:
```xml
omitted="svg_bloat_omitted"
```
This lets the LLM know the file's presence and paths without consuming valuable attention window tokens.

### 4. Direct Binary Bypass
When parsing a file, Kord scans its contents for a null byte (`0x00`). If a null byte is found, the file is tagged as binary (compressed files, compiled binaries, non-UTF-8 assets) and is omitted completely.

### 5. Content Compression
If `--compress` is activated, Kord removes unnecessary leading spaces, trailing carriage returns, and blank lines from code files before streaming them.

---

## 6. Output Formats & Schema Specifications

Kord generates output formatted in **XML**, **JSON**, or **Markdown**.

### A. XML Format (Recommended)
XML is the recommended layout for modern LLM prompting because tags clearly delimit file names, content, and metadata attributes.

```xml
<?xml version="1.0" encoding="UTF-8"?>
<repository>
  <!-- Generated Table of Contents (if --include-toc is passed) -->
  <toc><![CDATA[.
├── main.go
├── main_test.go
└── config/
    └── config.json
]]></toc>

  <!-- Last 5 Git Commits (if --include-git-log is passed) -->
  <git_log><![CDATA[a2f5b3d feat: optimize stream layout
f31e9c2 fix: handle empty ignore file line
]]></git_log>

  <!-- Normal File with status/metadata -->
  <file path="main.go" status="modified"><![CDATA[package main
...
]]></file>

  <!-- Relational Schema (if --include-schemas is passed and extension matches SQL/Prisma/GraphQL) -->
  <schema path="schema.prisma"><![CDATA[model User {
  id    Int    @id @default(autoincrement())
}
]]></schema>

  <!-- Skipped due to Size Limit -->
  <file path="large_data.json" omitted="size_limit_exceeded"></file>

  <!-- Skipped due to SVG Optimization -->
  <file path="logo.svg" omitted="svg_bloat_omitted"></file>
  
  <!-- If --max-tokens is hit during traversal, ingestion ceases and writes a warning tag -->
  <!-- WARNING: Ingestion ceased because estimated token limit exceeded: 80000 tokens -->
</repository>
```

---

### B. JSON Format
JSON output is formatted as a single object containing metadata and a structured list of files.

```json
{
  "repository": {
    "toc": ".\n├── main.go\n└── config.json\n",
    "git_log": "a2f5b3d feat: optimize stream layout\n",
    "files": [
      {
        "path": "main.go",
        "tag": "file",
        "status": "modified",
        "content": "package main\n...\n"
      },
      {
        "path": "large_data.json",
        "tag": "file",
        "omitted": "size_limit_exceeded"
      }
    ]
  }
}
```

---

### C. Markdown Format
Markdown output formats files as markdown headings with standard code fences:

```markdown
# Repository Context

## Table of Contents
\`\`\`
.
├── main.go
└── schema.sql
\`\`\`

## Git Log
\`\`\`
a2f5b3d feat: optimize stream layout
\`\`\`

## File: main.go
Path: main.go
Status: modified

```go
package main
import "fmt"
...
```

## Schema: schema.sql
Path: schema.sql

```sql
CREATE TABLE users (id INT);
```

## File: large_data.json
Path: large_data.json
Omitted: size_limit_exceeded
```

---

## 7. Developer Reference (Codebase Architecture)

Kord is contained within a clean, single-package architecture. Here is an overview of the key data structures and entry points.

### Configuration Struct (`Config`)
Defined in [main.go](file:///d:/Siranta/Kord/Kord/main.go), this struct collects command-line parameters:
```go
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
	MaxFileSize       int64 // Computed internally in bytes
}
```

### Main Pipeline Stages
1.  **Config Extraction (`parseConfig()`)**: Reads parameters using Go’s built-in `flag` package, mapping aliases (e.g. `-o` to `--output`, `-f` to `--format`), and parsing human-readable file sizes (like `250KB` or `2MB` into bytes).
2.  **Wizard Initiation (`runInteractiveWizard()`)**: Triggered if the first CLI argument is `start`. Gathers targets using standard keyboard scans and begins traversal.
3.  **Traverse Execution (`traverseDirectory()`)**: Employs Go's `filepath.WalkDir` to traverse filesystems in a non-recursive path-accumulation loop.
4.  **Token Estimation (`TokenCounter`)**: A tracking wrapper around the output `io.Writer` that monitors total accumulated bytes. To optimize parsing speed, tokens are estimated as `bytes / 4`. If this estimate exceeds `--max-tokens`, traversal is halted immediately.


