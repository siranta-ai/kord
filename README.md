# Kord

**Stop LLM context starvation.** Kord packs your entire codebase into a clean, streamed XML context window in **0.2 seconds**. 

No bloated tokens. No regex latency. Just the pure code your LLM needs to actually understand your repository.

### Install Now (Zero Dependencies)

Kord is a compiled, standalone binary. You don't need Go, Node, or Python installed to run it. 

**Mac / Linux:**
```bash
curl -sL https://github.com/siranta-ai/kord/releases/latest/download/kord-linux-amd64 -o kord
chmod +x kord
sudo mv kord /usr/local/bin/
```
*(Swap `linux-amd64` with `darwin-arm64` for Apple Silicon)*

**Windows:**
Download `kord-windows-amd64.exe` directly from [GitHub Releases](https://github.com/siranta-ai/kord/releases) and drop it in your PATH.

*(Already have Go? Just run `go install github.com/siranta-ai/kord@latest`)*

---

## The Problem Kord Solves

Dumping code into Claude or GPT usually results in crashed context windows or massive token waste. You end up feeding the model useless SVG coordinates, compiled binaries, and `node_modules`. 

Kord is a zero-dependency CLI written in Go that acts as a streaming ingestion pipeline. It traverses your filesystem, filters out the garbage in O(1) time, and streams raw XML directly to stdout. Memory consumption remains virtually flat whether your repository is 10MB or 10GB.

## Usage

Generate your context payload in one command:
*(Note: If you are on Windows and haven't added Kord to your PATH, use `.\kord` instead of `kord`)*

```bash
# Specify a custom ignore file
kord -ignore custom.gitignore

# Output in JSON or Markdown format instead of XML
kord -format markdown -dir . > codebase.md

# Pipe the output directly to a file
kord -dir . > codebase.xml
```

**Core Flags:**

| Flag | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `-dir` | `string` | `.` | The target directory to traverse. |
| `-format`, `-f` | `string` | `xml` | Selection of layout formats. Options: `xml`, `json`, `markdown`. |
| `-ignore` | `string` | `.gitignore` | The path to the ignore rules file to parse. |
| `--max-file-size` | `string` | `1MB` | The maximum size allowed for standard file contents before they are omitted (supports B, KB, MB, GB). |

Want interactive hand-holding? Run:
```bash
kord start
```

---

## Why It's Fast (The Filters)

Kord drops the heavy regex engine for raw speed and uses smart filtering so you don't burn tokens on useless data.

* **Zero-Regex Ignore Engine:** Rules are bucketed (Exact, Prefix, Suffix) using rapid O(1) map lookups. We auto-ignore the heavy trash natively (e.g., `.git`, `node_modules`, `.venv`, `dist`, `.exe`).
* **Binary Execution Block:** Before reading *any* file, Kord pulls the first 512 bytes. If it hits a null byte (`0x00`), the file is classified as binary and dumped instantly.
* **SVG Optimization:** SVGs destroy context windows with coordinate bloat. Kord intercepts `.svg` files, prints the file path so the LLM knows it exists, and strips the vector data.
* **Smart Size Limits:** Standard files cap at your `-max-size`. Documentation files (`.md`, `.txt`, `.mdx`) automatically get a **10x** multiplier because human context matters.
* **Infinite Loop Prevention:** If you pipe output into the same directory (`kord > output.xml`), Kord compares file descriptors and ignores its own output stream.

---

## Output Schema

Clean XML. Safe `CDATA`. LLMs parse this natively with zero hallucination. 

```xml
<?xml version="1.0" encoding="UTF-8"?>
<repository>
  <!-- Standard file -->
  <file path="main.go"><![CDATA[package main...]]></file>
  
  <!-- Exceeds file size limit (content dropped to save context) -->
  <file path="large_data.json" omitted="size_limit_exceeded"></file>
  
  <!-- SVG bloat dropped -->
  <file path="assets/logo.svg" omitted="svg_bloat_omitted"></file>
</repository>
```

---

## Beyond the CLI: Enterprise Agent Memory

Kord is your local open-source workhorse. But when you need to scale this to production fleets, manage persistent enterprise agent memory, and keep vector databases synced across thousands of repos without manual CLI dumps—you need **Siranta Gateway**. 

[Learn more about Siranta Gateway here](#) *(Link to Gateway)*.
