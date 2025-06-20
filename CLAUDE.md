# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based CLI wrapper that provides file watching capabilities for interactive command-line tools. The wrapper uses PTY (pseudo-terminal) to maintain full terminal compatibility with the wrapped program while adding file system monitoring.

## Commands

### Build and Run
```bash
go run main.go <command> [args...]
```

Examples:
```bash
go run main.go python3
go run main.go node
go run main.go python3 --watch=/path/to/directory
```

### Dependencies
```bash
go mod tidy
go mod download
```

### Testing
```bash
go test ./...
```

## Architecture

### Core Components

**CLIWrapper struct** (`main.go:20-25`): Manages the wrapped process using PTY for terminal compatibility
- `cmd`: The wrapped command process
- `ptmx`: PTY master for terminal I/O
- `stdin`/`stdout`: I/O streams to the wrapped process

**File Watching** (`setupFileWatcher`, `main.go:66-117`): Uses fsnotify to monitor file changes
- Watches for `.py`, `.js`, `.go` file modifications
- Sends commands to wrapped CLI when files change
- Configurable watch directory via `--watch=` flag

**User Input Handling** (`handleUserInput`, `main.go:119-142`): Processes user commands
- Regular input passes through to wrapped CLI
- Special commands (prefixed with `:`) are handled locally
- Available commands: `:help`, `:reload`, `:status`, `:quit`

### Signal Flow
1. User starts wrapper with target CLI command
2. Wrapper creates PTY and starts target process
3. File watcher monitors specified directory
4. User input is forwarded to wrapped process
5. File changes trigger automated commands to wrapped CLI
6. Output from wrapped process displays in terminal

### Key Dependencies
- `github.com/creack/pty`: PTY creation and management
- `github.com/fsnotify/fsnotify`: Cross-platform file system notifications

## Development Notes

The wrapper is designed to be CLI-agnostic and works with any interactive command-line tool. File change detection is currently hardcoded for Python, JavaScript, and Go files but can be extended for other file types.

Special commands are handled locally without forwarding to the wrapped process, allowing for wrapper-specific functionality like status checks and graceful shutdown.