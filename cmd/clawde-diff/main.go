package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mattduck/clawde/internal/diffparser"
	"github.com/mattduck/clawde/internal/tmux"
)

var (
	// Current pager process (for watch mode)
	currentPagerCmd *exec.Cmd
	pagerKilledByUs bool          // true if we killed the pager, false if user quit
	pagerExited     chan struct{} // closed when current pager exits
)

func main() {
	// Flags
	listFlag := flag.Bool("list", false, "List claude/clawde panes and exit")
	paneFlag := flag.String("pane", "", "Specific pane ID to capture from (default: first claude/clawde in current window)")
	rawFlag := flag.Bool("raw", false, "Output raw captured content instead of parsed diff")
	noPagerFlag := flag.Bool("no-pager", false, "Output to stdout instead of pager")
	watchFlag := flag.Bool("watch", false, "Watch mode: continuously poll for new diffs")
	intervalFlag := flag.Duration("interval", 3*time.Second, "Poll interval for watch mode")
	debugFlag := flag.Bool("debug", false, "Show debug output")
	flag.Parse()

	if !tmux.IsRunningInTmux() {
		fmt.Fprintln(os.Stderr, "error: not running inside tmux")
		os.Exit(1)
	}

	// Get current window
	currentWindow, err := tmux.GetCurrentWindow()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Find claude panes
	claudePanes, err := tmux.FindClaudePanes(currentWindow)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *listFlag {
		if len(claudePanes) == 0 {
			fmt.Println("No claude/clawde panes found in current window")
		} else {
			for _, p := range claudePanes {
				fmt.Printf("%s %s\n", p.ID, p.Command)
			}
		}
		return
	}

	// Select pane
	var targetPane string
	if *paneFlag != "" {
		targetPane = *paneFlag
	} else if len(claudePanes) > 0 {
		targetPane = claudePanes[0].ID
	} else {
		fmt.Fprintln(os.Stderr, "error: no claude/clawde panes found in current window")
		os.Exit(1)
	}

	if *watchFlag {
		runWatchMode(targetPane, *intervalFlag, *noPagerFlag, *debugFlag)
		return
	}

	// Single-shot mode
	runOnce(targetPane, *rawFlag, *noPagerFlag)
}

func runOnce(targetPane string, rawMode, noPagerMode bool) {
	content, err := tmux.CapturePane(targetPane, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if rawMode {
		fmt.Print(content)
		return
	}

	diffs := diffparser.Parse(content)
	if len(diffs) == 0 {
		fmt.Fprintln(os.Stderr, "no diffs found in pane output")
		os.Exit(0)
	}

	lastDiff := diffs[len(diffs)-1]
	unified := lastDiff.ToUnified()

	if noPagerMode {
		fmt.Print(unified)
		return
	}

	// Clear screen before showing diff
	fmt.Print("\033[2J\033[H")

	pager := detectPager()
	if pager == "" {
		fmt.Print(unified)
		return
	}

	if err := runPager(pager, unified); err != nil {
		fmt.Print(unified)
	}
}

func runWatchMode(targetPane string, interval time.Duration, noPagerMode, debug bool) {
	// Set up signal handling for clean exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var lastHash string
	pager := detectPager()
	if noPagerMode {
		pager = ""
	}

	// Piped pagers (like "diff-so-fancy | less") don't work in watch mode
	// because we can't reliably kill them. Fall back to less.
	if pager != "" && strings.Contains(pager, "|") {
		fmt.Fprintf(os.Stderr, "warning: piped pager %q not supported in watch mode, using less\n", pager)
		if _, err := exec.LookPath("less"); err == nil {
			pager = "less -R"
		} else {
			pager = ""
		}
	}

	if debug {
		fmt.Fprintf(os.Stderr, "[debug] watch mode: pane=%s interval=%v pager=%q\n", targetPane, interval, pager)
	}

	// Channel to know when pager exits
	pagerDone := make(chan struct{}, 1)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Do initial check immediately
	lastHash = checkAndUpdate(targetPane, lastHash, pager, pagerDone, debug)

	for {
		select {
		case <-sigChan:
			// Clean up and exit
			if debug {
				fmt.Fprintf(os.Stderr, "[debug] received signal, exiting\n")
			}
			killCurrentPager()
			fmt.Print("\033[2J\033[H") // Clear screen
			return

		case <-pagerDone:
			// Check if we killed it or user quit
			if pagerKilledByUs {
				if debug {
					fmt.Fprintf(os.Stderr, "[debug] pager killed by us, continuing\n")
				}
				pagerKilledByUs = false // Reset for next time
				continue
			}
			// User quit the pager, exit watch mode
			if debug {
				fmt.Fprintf(os.Stderr, "[debug] pager exited by user, exiting watch mode\n")
			}
			return

		case <-ticker.C:
			lastHash = checkAndUpdate(targetPane, lastHash, pager, pagerDone, debug)
		}
	}
}

func checkAndUpdate(targetPane, lastHash, pager string, pagerDone chan struct{}, debug bool) string {
	content, err := tmux.CapturePane(targetPane, true)
	if err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[debug] capture error: %v\n", err)
		}
		return lastHash
	}

	diffs := diffparser.Parse(content)
	if len(diffs) == 0 {
		if debug {
			fmt.Fprintf(os.Stderr, "[debug] no diffs found in pane output\n")
		}
		return lastHash
	}

	if debug {
		fmt.Fprintf(os.Stderr, "[debug] found %d diffs\n", len(diffs))
	}

	lastDiff := diffs[len(diffs)-1]
	unified := lastDiff.ToUnified()

	if debug {
		fmt.Fprintf(os.Stderr, "[debug] unified diff length: %d bytes\n", len(unified))
	}

	// Hash the diff content
	hash := hashContent(unified)

	// Only update if diff has changed
	if hash == lastHash {
		if debug {
			fmt.Fprintf(os.Stderr, "[debug] hash unchanged, skipping\n")
		}
		return lastHash
	}

	if debug {
		fmt.Fprintf(os.Stderr, "[debug] hash changed, updating display\n")
	}

	// Kill existing pager if running (also resets terminal and clears screen)
	killCurrentPager()

	if pager == "" {
		fmt.Print(unified)
		return hash
	}

	if debug {
		fmt.Fprintf(os.Stderr, "[debug] starting pager: %s\n", pager)
	}

	// Start new pager in background
	startPagerAsync(pager, unified, pagerDone, debug)

	return hash
}

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// killProcessTree kills a process and all its children using pgrep
func killProcessTree(pid int) {
	// First find all children using pgrep
	cmd := exec.Command("pgrep", "-P", strconv.Itoa(pid))
	output, _ := cmd.Output()

	// Kill children first (recursively)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if childPid, err := strconv.Atoi(line); err == nil && childPid > 0 {
			killProcessTree(childPid)
		}
	}

	// Then kill the process
	syscall.Kill(pid, syscall.SIGTERM)
	time.Sleep(50 * time.Millisecond)
	syscall.Kill(pid, syscall.SIGKILL)
}

func killCurrentPager() {
	cmd := currentPagerCmd  // Save reference locally
	exitChan := pagerExited // Save channel reference

	if cmd != nil && cmd.Process != nil {
		pagerKilledByUs = true // Mark that we're killing it, not the user
		currentPagerCmd = nil  // Clear immediately to prevent double-kill

		// Kill entire process tree (pager and any children like less)
		killProcessTree(cmd.Process.Pid)

		// Wait for process to exit (goroutine in startPagerAsync will close exitChan)
		if exitChan != nil {
			select {
			case <-exitChan:
				// Process exited cleanly
			case <-time.After(200 * time.Millisecond):
				// Timeout - already force killed above
			}
		}
	}

	// Reset terminal state in case pager didn't clean up
	resetTerminal()
}

func resetTerminal() {
	// Exit alternate screen mode (pagers like less use this)
	fmt.Print("\033[?1049l")

	// stty sane resets terminal modes
	stty := exec.Command("stty", "sane")
	stty.Stdin = os.Stdin
	stty.Run()

	// Clear screen and scrollback
	fmt.Print("\033[2J\033[3J\033[H")
}

func startPagerAsync(pager string, content string, done chan struct{}, debug bool) {
	// Write content to temp file so pager can read it while having full terminal access
	tmpFile, err := os.CreateTemp("", "clawde-diff-*.diff")
	if err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[debug] temp file error: %v\n", err)
		}
		fmt.Print(content)
		return
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(content); err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[debug] temp file write error: %v\n", err)
		}
		tmpFile.Close()
		os.Remove(tmpPath)
		fmt.Print(content)
		return
	}
	tmpFile.Close()

	if debug {
		fmt.Fprintf(os.Stderr, "[debug] wrote %d bytes to %s\n", len(content), tmpPath)
	}

	// Open temp file for stdin
	inputFile, err := os.Open(tmpPath)
	if err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[debug] temp file open error: %v\n", err)
		}
		os.Remove(tmpPath)
		fmt.Print(content)
		return
	}

	// Parse pager command and run directly (no shell wrapper)
	parts := strings.Fields(pager)
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = inputFile
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Override LESS to remove -F
	cmd.Env = os.Environ()
	lessOpts := os.Getenv("LESS")
	if strings.Contains(lessOpts, "F") {
		newLess := strings.ReplaceAll(lessOpts, "F", "")
		cmd.Env = replaceEnv(cmd.Env, "LESS", newLess)
	}

	if err := cmd.Start(); err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[debug] pager start error: %v\n", err)
		}
		inputFile.Close()
		os.Remove(tmpPath)
		fmt.Print(content)
		return
	}

	if debug {
		fmt.Fprintf(os.Stderr, "[debug] pager started with pid %d\n", cmd.Process.Pid)
	}

	currentPagerCmd = cmd
	pagerExited = make(chan struct{}) // Create new exit channel
	exitChan := pagerExited           // Capture for goroutine

	// Wait for pager to exit in background
	go func() {
		err := cmd.Wait()
		if debug {
			fmt.Fprintf(os.Stderr, "[debug] pager exited, err=%v\n", err)
		}
		inputFile.Close()
		os.Remove(tmpPath) // Clean up temp file

		// Signal that pager exited (close the channel)
		close(exitChan)

		// Also notify the main loop
		select {
		case done <- struct{}{}:
		default:
		}
	}()
}

// detectPager tries to find the user's preferred diff pager
func detectPager() string {
	if pager := os.Getenv("CLAWDE_DIFF_PAGER"); pager != "" {
		return pager
	}
	if pager := os.Getenv("GIT_PAGER"); pager != "" {
		return pager
	}

	if pager := gitConfig("pager.diff"); pager != "" {
		return pager
	}
	if pager := gitConfig("core.pager"); pager != "" {
		return pager
	}

	if _, err := exec.LookPath("delta"); err == nil {
		return "delta --paging=always"
	}
	if _, err := exec.LookPath("diff-so-fancy"); err == nil {
		return "diff-so-fancy | less -R"
	}

	if _, err := exec.LookPath("less"); err == nil {
		return "less -R"
	}

	return ""
}

func gitConfig(key string) string {
	cmd := exec.Command("git", "config", "--get", key)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func replaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func runPager(pager string, content string) error {
	cmd := exec.Command("sh", "-c", pager)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Env = os.Environ()
	lessOpts := os.Getenv("LESS")
	if strings.Contains(lessOpts, "F") {
		newLess := strings.ReplaceAll(lessOpts, "F", "")
		cmd.Env = replaceEnv(cmd.Env, "LESS", newLess)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	io.WriteString(stdin, content)
	stdin.Close()

	return cmd.Wait()
}
