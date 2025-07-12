package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

const version = "v0.1.0"

// NO_CLAWDE - This file is part of the CLI wrapper and should be excluded from comment detection

// Global logger instance
var logger *slog.Logger

// initLogging initializes the logging system based on configuration
func initLogging(config *Config) (*slog.Logger, *os.File, error) {
	// Parse log level
	var level slog.Level
	switch strings.ToLower(config.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// If LogFile is empty, create logger that writes to io.Discard
	if config.LogFile == "" {
		handler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
			Level: level,
		})
		logger := slog.New(handler)
		return logger, nil, nil
	}

	// Open log file
	logFile, err := os.OpenFile(config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Create slog handler with the specified level
	handler := slog.NewTextHandler(logFile, &slog.HandlerOptions{
		Level: level,
	})

	// Create and return the logger
	logger := slog.New(handler)
	return logger, logFile, nil
}

type CLIWrapper struct {
	cmd          *exec.Cmd
	ptmx         *os.File
	stdin        io.Writer
	stdout       io.Reader
	outputBuffer *outputBuffer
	config       *Config
}

type outputBuffer struct {
	data         []byte
	timer        *time.Timer
	mutex        sync.Mutex
	delay        time.Duration
	fastDelay    time.Duration // When typing (60fps)
	slowDelay    time.Duration // When idle (15fps)
	lastInput    time.Time
	inputTimeout time.Duration // How long to wait before switching to slow mode

	// Terminal state analysis
	currentLine      []byte       // Buffer to track the current line being built
	lastNonEmptyLine []byte       // Buffer to track the last non-empty line
	isInsertMode     bool         // Whether we're currently in INSERT mode
	insertMutex      sync.RWMutex // Separate mutex for insert mode state
}

func NewCLIWrapper(config *Config, command string, args ...string) (*CLIWrapper, error) {
	cmd := exec.Command(command, args...)

	// Set environment variables for the wrapped program
	cmd.Env = append(os.Environ(), cmd.Env...)
	if config.ForceAnsi {
		cmd.Env = append(cmd.Env, "COLORTERM=ansi", "TERM=xterm")
	}

	// Set up process group for proper job control
	// Setsid creates a new session and process group for the wrapped program.
	//
	// Why we want this:
	// - Ensures signals (like Ctrl+C) are delivered to the wrapped program and its children
	// - Provides proper job control isolation (wrapped program behaves like it would in shell)
	// - Prevents signals intended for wrapped program from affecting our wrapper
	//
	// Why we might not want this:
	// - PTY already provides some process isolation
	// - Adds complexity to signal handling
	// - May interfere with certain programs that expect to share process group with parent
	//
	// Overall: Setting this is the "correct" behavior for a shell-like wrapper
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	// Start the command with a pty
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start command with pty: %w", err)
	}

	wrapper := &CLIWrapper{
		cmd:    cmd,
		ptmx:   ptmx,
		stdin:  ptmx,
		stdout: ptmx,
		config: config,
		outputBuffer: &outputBuffer{
			fastDelay:    16 * time.Millisecond,            // 60fps when typing
			slowDelay:    33 * time.Millisecond,            // 30fps when idle
			delay:        33 * time.Millisecond,            // Start in slow mode
			inputTimeout: 2 * time.Second,                  // Switch to slow after 2s of no input
			lastInput:    time.Now().Add(-3 * time.Second), // Start as "old" input
		},
	}

	// Set initial terminal size
	if size, err := pty.GetsizeFull(os.Stdout); err == nil {
		pty.Setsize(ptmx, size)
	}

	// Handle terminal resize events
	// NOTE: before we added this I was getting some weird flickering (even more than usual)
	// when typing when there was already previous messages above (ie. on my second prompt).
	// this seemed to stop when we added the resize support.
	wrapper.setupResizeHandler()

	return wrapper, nil
}

func (w *CLIWrapper) SendCommand(command string) error {
	// Send the command text first
	_, err := w.stdin.Write([]byte(command))
	if err != nil {
		return err
	}

	// Add a pause before sending Enter key to submit.
	// it seems that Claude requires both the pause and sending the byte like this (rather than \n),
	// otherwise it just inserts the newline -- probably part of how it implements paste handling?
	time.Sleep(100 * time.Millisecond)
	_, err = w.stdin.Write([]byte{13}) // ASCII 13 = Enter key
	return err
}

// renderCommentPrompt creates a prompt for AI question comments
func renderCommentPrompt(comment AIComment, contextComments []AIComment) string {
	var locationStr string
	if comment.EndLine == 0 || comment.EndLine == comment.LineNumber {
		// Single-line comment
		locationStr = fmt.Sprintf("at line %d", comment.LineNumber)
	} else {
		// Multiline comment
		locationStr = fmt.Sprintf("at lines %d-%d", comment.LineNumber, comment.EndLine)
	}

	var prompt string
	switch comment.ActionType {
	case "!":
		prompt = fmt.Sprintf("See %s %s and surrounding context. Make the appropriate changes. YOU MUST replace the AI! marker with [ai] when done.",
			comment.FilePath, locationStr)
	case "?":
		prompt = fmt.Sprintf("See %s %s and surrounding context. Answer the question(s), but DO NOT MAKE CHANGES. Replace the AI? marker with [ai] when done.",
			comment.FilePath, locationStr)
	case ":":
		prompt = fmt.Sprintf("Extra from code comment at %s %s",
			comment.FilePath, locationStr)
	default:
		// TODO: should never happen, log?
		prompt = fmt.Sprintf("See %s %s and surrounding context.",
			comment.FilePath, locationStr)
	}

	// Add context comments if present
	if len(contextComments) > 0 {
		prompt += "\n\nRelated context:\n"
		for _, ctx := range contextComments {
			var ctxLocationStr string
			if ctx.EndLine == 0 || ctx.EndLine == ctx.LineNumber {
				ctxLocationStr = fmt.Sprintf("- line %d", ctx.LineNumber)
			} else {
				ctxLocationStr = fmt.Sprintf("- lines %d-%d", ctx.LineNumber, ctx.EndLine)
			}
			prompt += fmt.Sprintf("\n%s at %s:\n  %s\n", ctx.FilePath, ctxLocationStr, ctx.Content)
		}
	}

	return prompt
}

// TODO: refactor this so that we use the logic from the single one, but it becomes one function
//
// renderMultipleCommentsPrompt creates a prompt for multiple AI comments (file watcher only)
func renderMultipleCommentsPrompt(comments []AIComment, contextComments []AIComment) string {
	// File watcher only processes ? and ! comments, so we don't need to separate by type
	// Determine action type based on precedence (! takes precedence over ?)
	hasAction := false
	for _, comment := range comments {
		if comment.ActionType == "!" {
			hasAction = true
			break
		}
	}

	var prompt strings.Builder
	if hasAction {
		prompt.WriteString("Read the following locations and surrounding context, and make the appropriate changes mentioned in the comments. Replace the AI! markers with [ai] when done:\n\n")
	} else {
		prompt.WriteString("Read the following locations and surrounding context, and answer the question(s) in the comments. DO NOT MAKE CHANGES. Replace the AI? markers with [ai] when done:\n\n")
	}

	// Add bullet points for each comment
	for _, comment := range comments {
		var locationStr string
		if comment.EndLine == 0 || comment.EndLine == comment.LineNumber {
			locationStr = fmt.Sprintf("line %d", comment.LineNumber)
		} else {
			locationStr = fmt.Sprintf("lines %d-%d", comment.LineNumber, comment.EndLine)
		}

		prompt.WriteString(fmt.Sprintf("• %s at %s\n", comment.FilePath, locationStr))
	}

	// Add context comments if present
	if len(contextComments) > 0 {
		prompt.WriteString("\nAdditional context:\n")
		for _, ctx := range contextComments {
			var ctxLocationStr string
			if ctx.EndLine == 0 || ctx.EndLine == ctx.LineNumber {
				ctxLocationStr = fmt.Sprintf("line %d", ctx.LineNumber)
			} else {
				ctxLocationStr = fmt.Sprintf("lines %d-%d", ctx.LineNumber, ctx.EndLine)
			}
			prompt.WriteString(fmt.Sprintf("\n%s at %s:\n%s", ctx.FilePath, ctxLocationStr, ctx.Content))
		}
	}

	return prompt.String()
}

// renderContextPrompt creates a prompt for single AI context comment
func renderContextPrompt(comment AIComment) string {
	var locationStr string
	if comment.EndLine == 0 || comment.EndLine == comment.LineNumber {
		locationStr = fmt.Sprintf("line %d", comment.LineNumber)
	} else {
		locationStr = fmt.Sprintf("lines %d-%d", comment.LineNumber, comment.EndLine)
	}

	return fmt.Sprintf("Context from %s at %s:\n%s", comment.FilePath, locationStr, comment.Content)
}

// renderMultipleContextPrompt creates a prompt for multiple AI context comments
func renderMultipleContextPrompt(comments []AIComment) string {
	var prompt strings.Builder

	for i, comment := range comments {
		var locationStr string
		if comment.EndLine == 0 || comment.EndLine == comment.LineNumber {
			locationStr = fmt.Sprintf("line %d", comment.LineNumber)
		} else {
			locationStr = fmt.Sprintf("lines %d-%d", comment.LineNumber, comment.EndLine)
		}

		if i > 0 {
			prompt.WriteString("\n\n")
		}
		prompt.WriteString(fmt.Sprintf("%s at %s:\n%s", comment.FilePath, locationStr, comment.Content))
	}

	return prompt.String()
}

func (w *CLIWrapper) Close() error {
	if w.ptmx != nil {
		w.ptmx.Close()
	}
	if w.cmd != nil && w.cmd.Process != nil {
		return w.cmd.Process.Kill()
	}
	return nil
}

func (w *CLIWrapper) CopyOutput() {
	if w.config.EnableOutputThrottling {
		// Start throttled output copying
		go w.startThrottledOutput()
	} else {
		// Simple direct copy
		go func() {
			io.Copy(os.Stdout, w.stdout)
		}()
	}
}

func (w *CLIWrapper) startThrottledOutput() {
	buf := w.outputBuffer
	buffer := make([]byte, 4096)

	for {
		n, err := w.stdout.Read(buffer)
		if err != nil {
			// Handle any remaining data when reader finishes
			buf.mutex.Lock()
			if len(buf.data) > 0 {
				os.Stdout.Write(buf.data)
			}
			buf.mutex.Unlock()
			return
		}

		if n > 0 {
			buf.mutex.Lock()
			// Add the raw bytes to buffer (preserves \r, ANSI codes, etc.)
			buf.data = append(buf.data, buffer[:n]...)

			// Analyze the new data for INSERT mode detection
			w.updateInsertMode(buffer[:n])

			// Update delay based on recent input activity
			now := time.Now()
			if now.Sub(buf.lastInput) < buf.inputTimeout {
				buf.delay = buf.fastDelay // Recent input, use fast refresh
			} else {
				buf.delay = buf.slowDelay // No recent input, use slow refresh
			}

			// Reset timer for debouncing
			if buf.timer != nil {
				buf.timer.Stop()
			}

			buf.timer = time.AfterFunc(buf.delay, func() {
				buf.mutex.Lock()
				if len(buf.data) > 0 {
					os.Stdout.Write(buf.data)
					buf.data = buf.data[:0] // Reset buffer
				}
				buf.mutex.Unlock()
			})
			buf.mutex.Unlock()
		}
	}
}

// markUserInput updates the timestamp for user input activity
func (w *CLIWrapper) markUserInput() {
	w.outputBuffer.mutex.Lock()
	w.outputBuffer.lastInput = time.Now()
	w.outputBuffer.mutex.Unlock()
}

// isInInsertMode safely checks if we're currently in INSERT mode
func (w *CLIWrapper) isInInsertMode() bool {
	w.outputBuffer.insertMutex.RLock()
	defer w.outputBuffer.insertMutex.RUnlock()
	return w.outputBuffer.isInsertMode
}

// updateInsertMode analyzes the output buffer and updates INSERT mode state
func (w *CLIWrapper) updateInsertMode(newData []byte) {
	w.outputBuffer.insertMutex.Lock()
	defer w.outputBuffer.insertMutex.Unlock()

	// Update the current line buffer by processing new data
	for _, b := range newData {
		if b == '\n' || b == '\r' {
			// New line detected - if current line is non-empty, save it as last non-empty line
			if len(w.outputBuffer.currentLine) > 0 {
				// Make a copy of the current line to store as last non-empty line
				w.outputBuffer.lastNonEmptyLine = make([]byte, len(w.outputBuffer.currentLine))
				copy(w.outputBuffer.lastNonEmptyLine, w.outputBuffer.currentLine)
			}
			// Reset the current line buffer
			w.outputBuffer.currentLine = w.outputBuffer.currentLine[:0]
		} else if b >= 32 && b <= 126 {
			// Printable ASCII character, add to current line
			w.outputBuffer.currentLine = append(w.outputBuffer.currentLine, b)
		}
		// Note: We ignore ANSI escape sequences and other control characters
		// for simplicity, but this might need refinement for more robust detection
	}

	// Check if the last non-empty line contains '-- INSERT --'
	lastLineStr := string(w.outputBuffer.lastNonEmptyLine)
	// oldInsertMode := w.outputBuffer.isInsertMode
	w.outputBuffer.isInsertMode = strings.Contains(lastLineStr, "-- INSERT ")

	// Log mode changes for debugging
	// if oldInsertMode != w.outputBuffer.isInsertMode {
	// 	if w.outputBuffer.isInsertMode {
	// 		// log.Printf("INSERT mode detected: %q", lastLineStr)
	// 		log.Printf("INSERT mode detected")
	// 	} else {
	// 		log.Printf("INSERT mode ended")
	// 		// log.Printf("INSERT mode ended: %q", lastLineStr)
	// 	}
	// }
}

// setupResizeHandler handles terminal window resize events
func (w *CLIWrapper) setupResizeHandler() {
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)

	go func() {
		for range sigwinch {
			// Get current terminal size
			if size, err := pty.GetsizeFull(os.Stdout); err == nil {
				// Forward the new size to the wrapped program's PTY
				pty.Setsize(w.ptmx, size)
				logger.Info("Terminal resized", "cols", size.Cols, "rows", size.Rows)
			} else {
				logger.Warn("Failed to get terminal size on resize", "error", err)
			}
		}
	}()
}

// handleFileChange processes file changes and extracts AI comments
func handleFileChange(filePath string, wrapper *CLIWrapper) {
	logger.Info("Processing file change", "file", filePath)

	// Extract AI comments from the changed file
	comments, err := ExtractAIComments(filePath)
	if err != nil {
		logger.Error("Failed to extract AI comments", "file", filePath, "error", err)
		return
	}

	if len(comments) == 0 {
		logger.Info("No AI comments found", "file", filePath)
		return
	}

	logger.Info("AI comments found", "file", filePath)

	// Gather all unprocessed comments first
	var unprocessedComments []AIComment
	for i, comment := range comments {
		logger.Info("AI comment found",
			"comment_number", i+1,
			"file_path", comment.FilePath,
			"line_number", comment.LineNumber,
			"content", comment.Content,
			"action_type", comment.ActionType,
			"hash", comment.Hash,
			"full_line", comment.FullLine,
			"context_lines_count", len(comment.ContextLines))
		for _, contextLine := range comment.ContextLines {
			logger.Debug("Context line", "line", contextLine)
		}

		// File watcher only processes ? and ! comments (quick actions)
		if comment.ActionType == "?" || comment.ActionType == "!" {
			if !isCommentProcessed(comment) {
				logger.Info("Found new AI comment", "action_type", comment.ActionType, "hash", comment.Hash)
				unprocessedComments = append(unprocessedComments, comment)
			} else {
				logger.Debug("Skipping already processed AI comment", "hash", comment.Hash)
			}
		} else if comment.ActionType == ":" {
			// AI comments are ignored by file watcher (manual invocation only)
			logger.Debug("Ignoring AI context comment - use manual search to access", "hash", comment.Hash)
		} else {
			logger.Warn("Skipping AI comment with unsupported action type", "action_type", comment.ActionType)
		}
	}

	// Process all unprocessed comments together
	if len(unprocessedComments) > 0 {
		// Collect all context comments from the codebase
		contextComments := collectAllContextComments(".")

		var prompt string
		if len(unprocessedComments) == 1 {
			// Single comment - use existing template
			prompt = renderCommentPrompt(unprocessedComments[0], contextComments)
		} else {
			// Multiple comments - use new template
			prompt = renderMultipleCommentsPrompt(unprocessedComments, contextComments)
		}

		logger.Info("Sending prompt to underlying program", "prompt", prompt)

		// Send the combined prompt to the wrapped program
		if err := wrapper.SendCommand(prompt); err != nil {
			logger.Error("Failed to send prompt to wrapped program", "error", err)
		} else {
			// Mark all processed comments as processed
			for _, comment := range unprocessedComments {
				markCommentProcessed(comment)
			}
			logger.Info("Successfully sent prompt and marked comments as processed", "comment_count", len(unprocessedComments))
		}
	}

	logger.Debug("=== END AI COMMENTS ===\n")
}

// collectAllContextComments finds all : (context) comments in the codebase
func collectAllContextComments(rootDir string) []AIComment {
	logger.Debug("Collecting all context comments", "root_dir", rootDir)

	// Create git ignore cache for this search
	gitIgnore := NewGitIgnoreCache(rootDir)

	// Find all files with AI comments
	files, err := FindFilesWithAIComments(rootDir, gitIgnore)
	if err != nil {
		logger.Error("Failed to search for AI comments", "error", err)
		return nil
	}

	if len(files) == 0 {
		logger.Debug("No files with AI comments found")
		return nil
	}

	var contextComments []AIComment

	for _, filePath := range files {
		// Extract AI comments from the file
		comments, err := ExtractAIComments(filePath)
		if err != nil {
			logger.Error("Failed to extract AI comments", "file", filePath, "error", err)
			continue
		}

		for _, comment := range comments {
			// Only collect : comments (context)
			if comment.ActionType == ":" {
				contextComments = append(contextComments, comment)
			}
		}
	}

	logger.Debug("Found context comments", "count", len(contextComments))
	return contextComments
}

// triggerAICommentSearch manually searches for files with AI comments and processes them
func triggerAICommentSearch(rootDir string, wrapper *CLIWrapper) {
	logger.Info("=== MANUAL AI COMMENT SEARCH TRIGGERED ===")

	// Create git ignore cache for this search
	gitIgnore := NewGitIgnoreCache(rootDir)

	// Find all files with AI comments
	files, err := FindFilesWithAIComments(rootDir, gitIgnore)
	if err != nil {
		logger.Error("Failed to search for AI comments", "error", err)
		return
	}

	if len(files) == 0 {
		logger.Info("No files with AI comments found", "root_dir", rootDir)
		return
	}

	logger.Info("Found AI comments in files", "file_count", len(files))

	// Gather all unprocessed comments from all files
	var allUnprocessedComments []AIComment

	for _, filePath := range files {
		logger.Debug("Processing file", "file_path", filePath)

		// Extract AI comments from the file
		comments, err := ExtractAIComments(filePath)
		if err != nil {
			logger.Error("Failed to extract AI comments", "file", filePath, "error", err)
			continue
		}

		for i, comment := range comments {
			logger.Debug("Processing comment",
				"comment_number", i+1,
				"file_path", comment.FilePath,
				"line_number", comment.LineNumber,
				"end_line", comment.EndLine,
				"content", comment.Content,
				"action_type", comment.ActionType,
				"hash", comment.Hash)

			// Manual invocation only processes : comments (context)
			if comment.ActionType == ":" {
				// AI comments are included for context in manual search
				logger.Debug("Status: CONTEXT - will include")
				allUnprocessedComments = append(allUnprocessedComments, comment)
			} else if comment.ActionType == "?" || comment.ActionType == "!" {
				// ? and ! comments are ignored by manual search (file watcher only)
				logger.Debug("Status: QUICK ACTION - ignored by manual search")
			} else {
				logger.Debug("Status: UNSUPPORTED ACTION TYPE - skipping")
			}
			logger.Debug("---")
		}
	}

	// Process context comments (manual invocation only handles : comments)
	if len(allUnprocessedComments) > 0 {
		// All comments should be : (context) type for manual invocation
		var prompt string
		if len(allUnprocessedComments) == 1 {
			// Single context comment
			prompt = renderContextPrompt(allUnprocessedComments[0])
		} else {
			// Multiple context comments
			prompt = renderMultipleContextPrompt(allUnprocessedComments)
		}

		logger.Info("Sending context prompt to underlying program", "prompt", prompt)

		// Send the context (without final newline to avoid auto-sending)
		if _, err := wrapper.stdin.Write([]byte(prompt)); err != nil {
			logger.Error("Failed to send context to wrapped program", "error", err)
		} else {
			logger.Info("Successfully sent context (no auto-submit)", "comment_count", len(allUnprocessedComments))
		}
	} else {
		logger.Info("No unprocessed AI comments found")
	}

	logger.Debug("=== END MANUAL AI COMMENT SEARCH ===")
}

func setupFileWatcher(watchDir string, wrapper *CLIWrapper) (*FileWatcher, error) {
	logger.Info("Starting file watcher setup", "directory", watchDir)

	// Create callback function that captures wrapper
	onFileChange := func(filePath string) {
		handleFileChange(filePath, wrapper)
	}

	// Create and start the file watcher
	fileWatcher, err := NewFileWatcher(watchDir, onFileChange)
	if err != nil {
		return nil, err
	}

	err = fileWatcher.Start()
	if err != nil {
		fileWatcher.Close()
		return nil, err
	}

	return fileWatcher, nil
}

// KeyRepeatDetector tracks key press patterns to detect held keys
type KeyRepeatDetector struct {
	consecutiveCount int
	lastKeyTime      time.Time
	isHeld           bool
	mutex            sync.Mutex
	threshold        int           // Number of rapid presses to consider "held"
	maxInterval      time.Duration // Max time between presses to consider consecutive
	pendingTimer     *time.Timer   // Timer for deferred sending
	pendingCallback  func()        // Callback to execute when timer expires
}

// NewKeyRepeatDetector creates a new key repeat detector
func NewKeyRepeatDetector(threshold int, maxInterval time.Duration) *KeyRepeatDetector {
	return &KeyRepeatDetector{
		threshold:   threshold,
		maxInterval: maxInterval,
	}
}

// CheckHeld determines if the key is being held based on timing and repetition
func (k *KeyRepeatDetector) CheckHeld() bool {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	now := time.Now()

	// If this is the first press or it's been too long since last press, reset
	if k.consecutiveCount == 0 || now.Sub(k.lastKeyTime) > k.maxInterval {
		k.consecutiveCount = 1
		k.isHeld = false
	} else {
		// Rapid successive key presses
		k.consecutiveCount++

		// Consider it "held" after threshold rapid presses
		if k.consecutiveCount >= k.threshold {
			k.isHeld = true
		}
	}

	k.lastKeyTime = now
	return k.isHeld
}

// SetPendingAction sets up a deferred action with a timer
func (k *KeyRepeatDetector) SetPendingAction(callback func()) {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	// Cancel any existing timer
	if k.pendingTimer != nil {
		k.pendingTimer.Stop()
	}

	k.pendingCallback = callback
	k.pendingTimer = time.AfterFunc(k.maxInterval, func() {
		k.mutex.Lock()
		defer k.mutex.Unlock()
		if k.pendingCallback != nil {
			k.pendingCallback()
			k.pendingCallback = nil
		}
	})
}

// CancelPending cancels any pending deferred action
func (k *KeyRepeatDetector) CancelPending() {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	if k.pendingTimer != nil {
		k.pendingTimer.Stop()
		k.pendingTimer = nil
	}
	k.pendingCallback = nil
}

// Reset resets the key state when other keys are pressed
func (k *KeyRepeatDetector) Reset() {
	k.mutex.Lock()
	defer k.mutex.Unlock()

	k.consecutiveCount = 0
	k.isHeld = false

	// Cancel any pending action and execute it immediately (flush)
	if k.pendingTimer != nil {
		k.pendingTimer.Stop()
		k.pendingTimer = nil
	}
	if k.pendingCallback != nil {
		callback := k.pendingCallback
		k.pendingCallback = nil
		// Execute outside the lock to avoid deadlock
		go callback()
	}
}

// Global detector for Enter key (3 rapid presses within 500ms)
var enterDetector = NewKeyRepeatDetector(3, 500*time.Millisecond)

// Need a way to send deferred output to the wrapped program
var deferredOutputChannel = make(chan []byte, 100)

// processUserInput handles special key combinations and processes enter keys
func processUserInput(input []byte, n int, wrapper *CLIWrapper) []byte {
	processedInput := make([]byte, 0, n*2) // Allow space for potential expansion

	for i := 0; i < n; i++ {
		// Check for Ctrl+/ (ASCII 31) - trigger AI comment search
		if input[i] == 31 {
			logger.Info("Ctrl+/ detected - triggering AI comment search")
			go func() {
				triggerAICommentSearch(".", wrapper)
			}()
			// Don't add this to processedInput (consume the key)
			continue
		}
		// Check for Ctrl+N (ASCII 14) - map to down arrow
		if input[i] == 14 {
			processedInput = append(processedInput, '\x1b', '[', 'B')
			continue
		}
		// Check for Ctrl+P (ASCII 16) - map to up arrow
		if input[i] == 16 {
			processedInput = append(processedInput, '\x1b', '[', 'A')
			continue
		}
		// Check for Ctrl+J (ASCII 10) - reliable way to send actual Enter
		if input[i] == 10 {
			// Ctrl+J: send actual enter
			processedInput = append(processedInput, 13)
		} else if input[i] == 13 {
			// Check INSERT mode status when Enter is pressed
			insertMode := wrapper.isInInsertMode()
			logger.Debug("Enter key pressed", "insert_mode", insertMode)

			if insertMode {
				// In INSERT mode: use backslash+enter behavior
				if wrapper.config.EnableHeldEnterDetection {
					// Check if this is a held Enter key
					shouldSendRawEnter := enterDetector.CheckHeld()

					if shouldSendRawEnter {
						// Held Enter: cancel any pending and send actual enter
						enterDetector.CancelPending()
						processedInput = append(processedInput, 13)
					} else if enterDetector.consecutiveCount == 1 {
						// First Enter in potential sequence: defer sending backslash+enter
						enterDetector.SetPendingAction(func() {
							// Send backslash+enter after delay
							deferredOutput := []byte{'\\', 13}
							select {
							case deferredOutputChannel <- deferredOutput:
							default:
								// Channel full, send directly (shouldn't happen with large buffer)
								wrapper.stdin.Write(deferredOutput)
							}
						})
						// Don't add anything to processedInput yet
					} else {
						// Subsequent Enter in sequence but not yet held: send actual enter
						processedInput = append(processedInput, 13)
					}
				} else {
					// Simple mode in INSERT: send backslash+enter for regular Enter
					processedInput = append(processedInput, '\\')
					processedInput = append(processedInput, 13)
				}
			} else {
				// Not in INSERT mode: send normal Enter
				processedInput = append(processedInput, 13)
			}
		} else {
			// All other characters: pass through unchanged
			processedInput = append(processedInput, input[i])
			// Reset enter detector on any non-enter input (this flushes pending)
			if wrapper.config.EnableHeldEnterDetection {
				enterDetector.Reset()
			}
		}
	}
	return processedInput
}

func handleUserInput(wrapper *CLIWrapper) {
	// Start goroutine to handle deferred output
	go func() {
		for deferredOutput := range deferredOutputChannel {
			wrapper.stdin.Write(deferredOutput)
		}
	}()

	if wrapper.config.EnableInputThrottling {
		go func() {
			// Create a buffer to intercept input and track typing activity
			buffer := make([]byte, 1024)
			for {
				n, err := os.Stdin.Read(buffer)
				if err != nil {
					return
				}
				if n > 0 {
					// Mark that user is typing
					wrapper.markUserInput()

					// Process the input to handle special keys and replace enter with backslash+enter
					processedInput := processUserInput(buffer, n, wrapper)

					// Forward the processed input to the wrapped program (if any)
					if len(processedInput) > 0 {
						wrapper.stdin.Write(processedInput)
					}
				}
			}
		}()
	} else {
		go func() {
			// Simple input copy with enter key replacement
			buffer := make([]byte, 1024)
			for {
				n, err := os.Stdin.Read(buffer)
				if err != nil {
					return
				}
				if n > 0 {
					// Process the input to handle special keys and replace enter with backslash+enter
					processedInput := processUserInput(buffer, n, wrapper)

					// Forward the processed input to the wrapped program (if any)
					if len(processedInput) > 0 {
						wrapper.stdin.Write(processedInput)
					}
				}
			}
		}()
	}
}

func main() {
	// Load configuration from environment variables
	config := LoadConfig()

	// Initialize logging based on configuration
	var logFile *os.File
	var err error
	logger, logFile, err = initLogging(config)
	if err != nil {
		fmt.Printf("Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	// Always look for "claude" program on PATH
	command, err := exec.LookPath("claude")
	if err != nil {
		fmt.Printf("Error: 'claude' program not found on PATH: %v\n", err)
		os.Exit(1)
	}

	// Pass all arguments straight through to claude
	args := os.Args[1:]

	// Create the CLI wrapper first (program starts in canonical mode like normal shell)
	wrapper, err := NewCLIWrapper(config, command, args...)
	if err != nil {
		logger.Error("Failed to create CLI wrapper", "error", err)
		os.Exit(1)
	}
	defer wrapper.Close()

	// Now set up raw mode for our input handling
	var oldState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		var err error
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			logger.Warn("Failed to set terminal to raw mode", "error", err)
		} else {
			defer term.Restore(int(os.Stdin.Fd()), oldState)
		}
	} else {
		logger.Info("Input is not a terminal, skipping raw mode setup")
	}

	// Function to restore terminal and exit
	exitWithRestore := func(code int) {
		if oldState != nil {
			term.Restore(int(os.Stdin.Fd()), oldState)
		}
		os.Exit(code)
	}

	// Start copying output from wrapped program to stdout
	wrapper.CopyOutput()

	// Set up file watching for current directory
	watchDir := "."
	if len(os.Args) > 2 && strings.HasPrefix(os.Args[len(os.Args)-1], "--watch=") {
		watchDir = strings.TrimPrefix(os.Args[len(os.Args)-1], "--watch=")
	}

	fileWatcher, err := setupFileWatcher(watchDir, wrapper)
	if err != nil {
		logger.Error("Failed to setup file watcher", "error", err)
		exitWithRestore(1)
	}
	defer fileWatcher.Close()

	// Handle user input
	handleUserInput(wrapper)

	// Start periodic INSERT mode status logging for debugging
	// go func() {
	// 	ticker := time.NewTicker(5 * time.Second)
	// 	defer ticker.Stop()
	// 	for range ticker.C {
	// 		insertMode := wrapper.isInInsertMode()
	// 		wrapper.outputBuffer.insertMutex.RLock()
	// 		lastNonEmptyLine := string(wrapper.outputBuffer.lastNonEmptyLine)
	// 		currentLine := string(wrapper.outputBuffer.currentLine)
	// 		wrapper.outputBuffer.insertMutex.RUnlock()
	// 		log.Printf("Periodic status check - INSERT mode: %t, last non-empty line: %q, current line: %q", insertMode, lastNonEmptyLine, currentLine)
	// 	}
	// }()

	// Handle external termination (SIGTERM, SIGHUP) gracefully
	// Let PTY handle SIGINT (Ctrl+C) naturally to ensure proper forwarding
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGHUP)

	// Also monitor parent process death (in case go run is killed)
	parentPid := os.Getppid()
	go func() {
		for {
			if os.Getppid() != parentPid {
				logger.Info("Parent process died, shutting down", "old_pid", parentPid, "new_pid", os.Getppid())
				wrapper.Close()
				exitWithRestore(0)
				return
			}
			time.Sleep(1 * time.Second)
		}
	}()

	go func() {
		sig := <-c
		logger.Info("Received signal, forwarding to wrapped process", "signal", sig)

		// Forward signal to wrapped process for graceful shutdown
		if wrapper.cmd.Process != nil {
			wrapper.cmd.Process.Signal(sig)
			logger.Info("Forwarded signal to wrapped process", "signal", sig, "pid", wrapper.cmd.Process.Pid)

			// Give wrapped process time to clean up gracefully
			time.Sleep(10 * time.Second)
			logger.Info("Grace period expired, forcing shutdown")
		}

		wrapper.Close()
		exitWithRestore(0)
	}()

	// Wait for the wrapped process to finish
	err = wrapper.cmd.Wait()
	exitCode := 0
	if err != nil {
		// Extract exit code from error
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}

	// Exit with the same code as the wrapped process
	exitWithRestore(exitCode)
}
