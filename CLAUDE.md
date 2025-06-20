# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based CLI wrapper that provides file watching capabilities for interactive command-line tools. The wrapper uses PTY (pseudo-terminal) to maintain full terminal compatibility with the wrapped program while adding file system monitoring and AI comment processing.

## Commands

### Build and Run
```bash
go run main.go <command> [args...]
```

Examples:
```bash
go run main.go claude
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

**CLIWrapper struct** (`main.go:31-37`): Manages the wrapped process using PTY for terminal compatibility
- `cmd`: The wrapped command process
- `ptmx`: PTY master for terminal I/O
- `stdin`/`stdout`: I/O streams to the wrapped process
- `outputBuffer`: Throttled output management with adaptive refresh rates

**File Watching** (`files.go`): Uses fsnotify to monitor file changes recursively
- Watches for `.py`, `.js`, `.go` file modifications in current directory and subdirectories
- Automatically adds new directories to watcher
- Ignores common directories like `.git`, `node_modules`, `__pycache__`
- Configurable watch directory via `--watch=` flag (defaults to current directory)

**AI Comment Processing** (`comment.go`): Extracts and processes AI-related comments from source files
- Detects comments ending with `AI?` (questions) or `AI!` (commands)
- Supports single-line (`//`, `#`) and multiline (`/* */`, `"""`) comment patterns
- Groups consecutive single-line comments into blocks
- Generates content hashes to avoid reprocessing the same comments
- Provides context lines around detected comments

**User Input Handling** (`main.go:696-748`): Processes user input with special key mappings
- `Ctrl+/`: Triggers manual AI comment search across all files
- `Ctrl+N`: Maps to down arrow (for vim-like navigation)
- `Ctrl+P`: Maps to up arrow (for vim-like navigation)  
- `Ctrl+J`: Sends reliable Enter key (bypasses INSERT mode detection)
- INSERT mode detection: automatically sends `\` before Enter in vim INSERT mode
- Key repeat detection for held Enter keys

### Signal Flow
1. User starts wrapper with target CLI command (typically `claude`)
2. Wrapper creates PTY and starts target process in raw terminal mode
3. File watcher monitors specified directory recursively
4. When files change, AI comments are extracted and processed
5. Unprocessed AI comments are sent as prompts to wrapped CLI
6. User input is processed for special keys and forwarded to wrapped process
7. Output from wrapped process displays with adaptive throttling

### Key Dependencies
- `github.com/creack/pty`: PTY creation and management
- `github.com/fsnotify/fsnotify`: Cross-platform file system notifications
- `golang.org/x/term`: Terminal control and raw mode handling

## AI Comment Format

Comments are detected with these patterns (case-insensitive):
- Questions: `// What should this do AI?` or `# How does this work AI?`
- Commands: `// Fix this function AI!` or `# Optimize this code AI!`

AI markers must appear at the start or end of comments, not in the middle. Consecutive single-line comments are automatically grouped into multi-line blocks.

## Development Notes

The wrapper is specifically designed to work with Claude CLI but supports any interactive command-line tool. The AI comment system allows developers to embed questions and requests directly in source code, which are automatically processed when files change.

Output throttling uses adaptive refresh rates (60fps when typing, 30fps when idle) to reduce terminal flicker. Terminal resize events are handled automatically to maintain proper display.