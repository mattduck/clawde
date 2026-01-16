package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/mattduck/clawde/internal/diffparser"
	"github.com/mattduck/clawde/internal/tmux"
)

func main() {
	// Flags
	listFlag := flag.Bool("list", false, "List claude/clawde panes and exit")
	paneFlag := flag.String("pane", "", "Specific pane ID to capture from (default: first claude/clawde in current window)")
	rawFlag := flag.Bool("raw", false, "Output raw captured content instead of parsed diff")
	noPagerFlag := flag.Bool("no-pager", false, "Output to stdout instead of pager")
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

	// Capture pane content
	content, err := tmux.CapturePane(targetPane, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *rawFlag {
		fmt.Print(content)
		return
	}

	// Parse diffs - only take the last one
	diffs := diffparser.Parse(content)
	if len(diffs) == 0 {
		fmt.Fprintln(os.Stderr, "no diffs found in pane output")
		os.Exit(0)
	}

	// Convert to unified diff format (last diff only)
	lastDiff := diffs[len(diffs)-1]
	unified := lastDiff.ToUnified()

	// Output
	if *noPagerFlag {
		fmt.Print(unified)
		return
	}

	// Clear screen before showing diff
	fmt.Print("\033[2J\033[H")

	// Try to use a pager
	pager := detectPager()
	if pager == "" {
		fmt.Print(unified)
		return
	}

	if err := runPager(pager, unified); err != nil {
		// Fall back to stdout
		fmt.Print(unified)
	}
}

// detectPager tries to find the user's preferred diff pager
func detectPager() string {
	// Check environment variables
	if pager := os.Getenv("CLAWDE_DIFF_PAGER"); pager != "" {
		return pager
	}
	if pager := os.Getenv("GIT_PAGER"); pager != "" {
		return pager
	}

	// Try git config
	if pager := gitConfig("pager.diff"); pager != "" {
		return pager
	}
	if pager := gitConfig("core.pager"); pager != "" {
		return pager
	}

	// Check for common diff tools
	if _, err := exec.LookPath("delta"); err == nil {
		return "delta --paging=always"
	}
	if _, err := exec.LookPath("diff-so-fancy"); err == nil {
		return "diff-so-fancy | less -R"
	}

	// Fall back to less (without -F so it doesn't quit on short content)
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
	// Handle piped commands like "diff-so-fancy | less -R"
	cmd := exec.Command("sh", "-c", pager)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Override LESS to remove -F (quit-if-one-screen) if present
	cmd.Env = os.Environ()
	lessOpts := os.Getenv("LESS")
	if strings.Contains(lessOpts, "F") {
		// Remove F flag from LESS
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
