#!/bin/bash

set -e

echo "Starting cross-platform compilation for Kord..."

# Ensure output directory exists
mkdir -p bin

# Enforce static compilation across all platforms
export CGO_ENABLED=0

# Strip debug symbols to reduce binary size
LDFLAGS="-s -w"

# 1. Linux (amd64)
echo "Building for Linux (amd64)..."
GOOS=linux GOARCH=amd64 go build -ldflags="$LDFLAGS" -o bin/kord-linux-amd64 main.go

# 2. Linux (arm64)
echo "Building for Linux (arm64)..."
GOOS=linux GOARCH=arm64 go build -ldflags="$LDFLAGS" -o bin/kord-linux-arm64 main.go

# 3. macOS (amd64)
echo "Building for macOS (amd64)..."
GOOS=darwin GOARCH=amd64 go build -ldflags="$LDFLAGS" -o bin/kord-darwin-amd64 main.go

# 4. macOS (arm64 / Apple Silicon)
echo "Building for macOS (arm64)..."
GOOS=darwin GOARCH=arm64 go build -ldflags="$LDFLAGS" -o bin/kord-darwin-arm64 main.go

# 5. Windows (amd64)
echo "Building for Windows (amd64)..."
GOOS=windows GOARCH=amd64 go build -ldflags="$LDFLAGS" -o bin/kord-windows-amd64.exe main.go

echo "✅ Build complete! Binaries are located in the ./bin/ directory."
