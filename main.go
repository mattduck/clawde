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
	"time"

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
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	go func() {
		defer watcher.Close()
		
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				
				// Only react to write events on specific file types
				if event.Op&fsnotify.Write == fsnotify.Write {
					ext := filepath.Ext(event.Name)
					if ext == ".py" || ext == ".js" || ext == ".go" {
						
						// Send a command to the wrapped CLI
						// This is just an example - customize based on your CLI
						filename := filepath.Base(event.Name)
						command := fmt.Sprintf(`print("File %s was modified at %s")`, 
							filename, time.Now().Format("15:04:05"))
						
						if err := wrapper.SendCommand(command); err != nil {
							log.Printf("Error sending command: %v", err)
						}
					}
				}
				
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("File watcher error: %v", err)
			}
		}
	}()

	// Add the directory to the watcher
	err = watcher.Add(watchDir)
	if err != nil {
		return fmt.Errorf("failed to add directory to watcher: %w", err)
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
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <command> [args...]")
		fmt.Println("Example: go run main.go python3")
		fmt.Println("Example: go run main.go node")
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	// Put terminal in raw mode to pass through control characters
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("Failed to set terminal to raw mode: %v", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Function to restore terminal and exit
	exitWithRestore := func(code int) {
		term.Restore(int(os.Stdin.Fd()), oldState)
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