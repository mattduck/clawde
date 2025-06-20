package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/term"
)

// TODO: these were to try to reduce flicker, not sure if it actually
// helps. might want to ignore.

// Global flag to control output throttling
var enableOutputThrottling = true

// Global flag to control input tracking for adaptive delays
var enableInputTracking = true

type CLIWrapper struct {
	cmd          *exec.Cmd
	ptmx         *os.File
	stdin        io.Writer
	stdout       io.Reader
	outputBuffer *outputBuffer
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
}

func NewCLIWrapper(command string, args ...string) (*CLIWrapper, error) {
	cmd := exec.Command(command, args...)

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
func renderCommentPrompt(comment AIComment) string {
	var locationStr string
	if comment.EndLine == 0 || comment.EndLine == comment.LineNumber {
		// Single-line comment
		locationStr = fmt.Sprintf("at line %d", comment.LineNumber)
	} else {
		// Multiline comment
		locationStr = fmt.Sprintf("at lines %d-%d", comment.LineNumber, comment.EndLine)
	}

	if comment.ActionType == "!" {
		return fmt.Sprintf("See %s %s and surrounding context. Summarise the ask, and make the appropriate changes",
			comment.FilePath, locationStr)
	} else {
		return fmt.Sprintf("See %s %s and surrounding context. Summarise the question and answer it. DO NOT MAKE CHANGES.",
			comment.FilePath, locationStr)
	}
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
	if enableOutputThrottling {
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
				log.Printf("Terminal resized to %dx%d", size.Cols, size.Rows)
			} else {
				log.Printf("Failed to get terminal size on resize: %v", err)
			}
		}
	}()
}

func setupFileWatcher(watchDir string, wrapper *CLIWrapper) error {
	log.Printf("Starting file watcher setup for directory: %s", watchDir)

	// Check if the watch directory exists
	if _, err := os.Stat(watchDir); os.IsNotExist(err) {
		log.Printf("ERROR: Watch directory does not exist: %s", watchDir)
		return fmt.Errorf("watch directory does not exist: %s", watchDir)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("ERROR: Failed to create file watcher: %v", err)
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	log.Printf("File watcher created successfully")

	go func() {
		defer watcher.Close()
		log.Printf("File watcher goroutine started, listening for events...")

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					log.Printf("File watcher events channel closed")
					return
				}

				log.Printf("Raw file event received: %s | Op: %s", event.Name, event.Op.String())

				// Log all event types for debugging
				if event.Op&fsnotify.Create == fsnotify.Create {
					log.Printf("CREATE event: %s", event.Name)
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Printf("WRITE event: %s", event.Name)
				}
				if event.Op&fsnotify.Remove == fsnotify.Remove {
					log.Printf("REMOVE event: %s", event.Name)
				}
				if event.Op&fsnotify.Rename == fsnotify.Rename {
					log.Printf("RENAME event: %s", event.Name)
				}
				if event.Op&fsnotify.Chmod == fsnotify.Chmod {
					log.Printf("CHMOD event: %s", event.Name)
				}

				// React to write and create events on specific file types
				// Many editors use atomic replacement (create temp file, rename) instead of direct writes
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					ext := filepath.Ext(event.Name)
					log.Printf("File extension detected: %s for file %s", ext, event.Name)

					// Skip temporary files (ending with ~, .tmp, .swp, etc.)
					if strings.HasSuffix(event.Name, "~") ||
						strings.HasSuffix(event.Name, ".tmp") ||
						strings.HasSuffix(event.Name, ".swp") ||
						strings.Contains(event.Name, ".#") {
						log.Printf("Ignoring temporary file: %s", event.Name)
					} else if ext == ".py" || ext == ".js" || ext == ".go" {
						log.Printf("File change detected for monitored extension: %s", event.Name)

						// Extract AI comments from the changed file
						comments, err := ExtractAIComments(event.Name)
						if err != nil {
							log.Printf("ERROR: Failed to extract AI comments from %s: %v", event.Name, err)
						} else if len(comments) > 0 {
							log.Printf("=== AI COMMENTS FOUND IN %s ===", event.Name)
							for i, comment := range comments {
								log.Printf("Comment #%d:", i+1)
								log.Printf("  FilePath: %s", comment.FilePath)
								log.Printf("  LineNumber: %d", comment.LineNumber)
								log.Printf("  Content: %s", comment.Content)
								log.Printf("  ActionType: %s", comment.ActionType)
								log.Printf("  Hash: %s", comment.Hash)
								log.Printf("  FullLine: %s", comment.FullLine)
								log.Printf("  Context (%d lines):", len(comment.ContextLines))
								for _, contextLine := range comment.ContextLines {
									log.Printf("    %s", contextLine)
								}
								log.Printf("  ---")

								// Process AI comments (both "?" and "!" action types)
								if comment.ActionType == "?" || comment.ActionType == "!" {
									// Check if we've already processed this comment
									if !isCommentProcessed(comment) {
										log.Printf("Processing new AI comment (%s): %s", comment.ActionType, comment.Hash)

										// Render the prompt template
										prompt := renderCommentPrompt(comment)
										log.Printf("Sending prompt to underlying program: %s", prompt)

										// Send the prompt to the wrapped program
										if err := wrapper.SendCommand(prompt); err != nil {
											log.Printf("ERROR: Failed to send prompt to wrapped program: %v", err)
										} else {
											// Mark this comment as processed to avoid reprocessing
											markCommentProcessed(comment)
											log.Printf("Successfully sent prompt and marked comment as processed")
										}
									} else {
										log.Printf("Skipping already processed AI comment: %s", comment.Hash)
									}
								} else {
									log.Printf("Skipping AI comment with unsupported action type: %s", comment.ActionType)
								}
							}
							log.Printf("=== END AI COMMENTS ===")
						} else {
							log.Printf("No AI comments found in %s", event.Name)
						}
					} else {
						log.Printf("Ignoring file change for unmonitored extension: %s (file: %s)", ext, event.Name)
					}
				} else {
					log.Printf("Ignoring event type %s for file %s", event.Op.String(), event.Name)
				}

			case err, ok := <-watcher.Errors:
				log.Printf("File watcher error: %v", err)
				if !ok {
					log.Printf("File watcher errors channel closed")
					return
				}
			}
		}
	}()

	// Add the directory to the watcher
	log.Printf("Adding directory to watcher: %s", watchDir)
	err = watcher.Add(watchDir)
	if err != nil {
		log.Printf("ERROR: Failed to add directory to watcher: %v", err)
		return fmt.Errorf("failed to add directory to watcher: %w", err)
	}

	log.Printf("Directory successfully added to watcher: %s", watchDir)

	// List files in the directory for debugging
	files, err := filepath.Glob(filepath.Join(watchDir, "*"))
	if err != nil {
		log.Printf("WARNING: Could not list files in watch directory: %v", err)
	} else {
		log.Printf("Files in watch directory (%d total):", len(files))
		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				log.Printf("  - %s (stat error: %v)", file, err)
			} else {
				log.Printf("  - %s (size: %d, mode: %s)", file, info.Size(), info.Mode())
			}
		}
	}

	return nil
}

func handleUserInput(wrapper *CLIWrapper) {
	if enableInputTracking {
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

					// Forward the input to the wrapped program
					wrapper.stdin.Write(buffer[:n])
				}
			}
		}()
	} else {
		go func() {
			// Simple direct copy
			io.Copy(wrapper.stdin, os.Stdin)
		}()
	}
}

func main() {
	// Set up logging to file to avoid interfering with wrapped program output
	logFile, err := os.OpenFile("/tmp/matt.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	log.SetOutput(logFile)

	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <command> [args...]")
		fmt.Println("Example: go run main.go python3")
		fmt.Println("Example: go run main.go node")
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	// Create the CLI wrapper first (program starts in canonical mode like normal shell)
	wrapper, err := NewCLIWrapper(command, args...)
	if err != nil {
		log.Printf("Failed to create CLI wrapper: %v", err)
		os.Exit(1)
	}
	defer wrapper.Close()

	// Now set up raw mode for our input handling
	var oldState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		var err error
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			log.Printf("Warning: Failed to set terminal to raw mode: %v", err)
		} else {
			defer term.Restore(int(os.Stdin.Fd()), oldState)
		}
	} else {
		log.Printf("Input is not a terminal, skipping raw mode setup")
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

	if err := setupFileWatcher(watchDir, wrapper); err != nil {
		log.Printf("Failed to setup file watcher: %v", err)
		exitWithRestore(1)
	}

	// Handle user input
	handleUserInput(wrapper)

	// Handle external termination (SIGTERM, SIGHUP) gracefully
	// Let PTY handle SIGINT (Ctrl+C) naturally to ensure proper forwarding
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGHUP)

	// Also monitor parent process death (in case go run is killed)
	parentPid := os.Getppid()
	go func() {
		for {
			if os.Getppid() != parentPid {
				log.Printf("Parent process died (was %d, now %d), shutting down", parentPid, os.Getppid())
				wrapper.Close()
				exitWithRestore(0)
				return
			}
			time.Sleep(1 * time.Second)
		}
	}()

	go func() {
		sig := <-c
		log.Printf("Received %v, forwarding to wrapped process", sig)

		// Forward signal to wrapped process for graceful shutdown
		if wrapper.cmd.Process != nil {
			wrapper.cmd.Process.Signal(sig)
			log.Printf("Forwarded %v to wrapped process (PID: %d)", sig, wrapper.cmd.Process.Pid)

			// Give wrapped process time to clean up gracefully
			time.Sleep(10 * time.Second)
			log.Printf("Grace period expired, forcing shutdown")
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
