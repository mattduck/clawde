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
	"syscall"

	"github.com/creack/pty"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/term"
)

type CLIWrapper struct {
	cmd    *exec.Cmd
	ptmx   *os.File
	stdin  io.Writer
	stdout io.Reader
}

func NewCLIWrapper(command string, args ...string) (*CLIWrapper, error) {
	cmd := exec.Command(command, args...)

	// Start the command with a pty
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start command with pty: %w", err)
	}

	return &CLIWrapper{
		cmd:    cmd,
		ptmx:   ptmx,
		stdin:  ptmx,
		stdout: ptmx,
	}, nil
}

func (w *CLIWrapper) SendCommand(command string) error {
	_, err := w.stdin.Write([]byte(command + "\n"))
	return err
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
	// Copy output from the wrapped program to stdout
	go func() {
		io.Copy(os.Stdout, w.stdout)
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
	go func() {
		// Copy stdin directly to the PTY to preserve all control characters
		io.Copy(wrapper.stdin, os.Stdin)
	}()
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

	// Put terminal in raw mode to pass through control characters
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

	// Create the CLI wrapper
	wrapper, err := NewCLIWrapper(command, args...)
	if err != nil {
		log.Printf("Failed to create CLI wrapper: %v", err)
		exitWithRestore(1)
	}
	defer wrapper.Close()

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

	// Handle Ctrl+C gracefully
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nShutting down...")
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
