package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Pane represents a tmux pane
type Pane struct {
	Session string
	Window  string
	Index   string
	Command string
	ID      string // full ID like "0:1.2"
}

// IsRunningInTmux checks if we're inside a tmux session
func IsRunningInTmux() bool {
	return os.Getenv("TMUX") != ""
}

// GetCurrentWindow returns the current tmux window identifier
func GetCurrentWindow() (string, error) {
	cmd := exec.Command("tmux", "display-message", "-p", "#{session_name}:#{window_index}")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current window: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// ListPanes lists all panes, optionally filtered to a specific window
func ListPanes(window string) ([]Pane, error) {
	args := []string{"list-panes", "-F", "#{session_name}:#{window_index}.#{pane_index} #{pane_current_command}"}
	if window != "" {
		args = append(args, "-t", window)
	} else {
		args = append(args, "-a")
	}

	cmd := exec.Command("tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list panes: %w", err)
	}

	var panes []Pane
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}

		id := parts[0]
		command := parts[1]

		// Parse session:window.pane
		var session, window, index string
		if colonIdx := strings.Index(id, ":"); colonIdx != -1 {
			session = id[:colonIdx]
			rest := id[colonIdx+1:]
			if dotIdx := strings.Index(rest, "."); dotIdx != -1 {
				window = rest[:dotIdx]
				index = rest[dotIdx+1:]
			}
		}

		panes = append(panes, Pane{
			Session: session,
			Window:  window,
			Index:   index,
			Command: command,
			ID:      id,
		})
	}

	return panes, nil
}

// FindClaudePanes returns panes running claude or clawde
func FindClaudePanes(window string) ([]Pane, error) {
	panes, err := ListPanes(window)
	if err != nil {
		return nil, err
	}

	var claudePanes []Pane
	for _, p := range panes {
		cmd := strings.ToLower(p.Command)
		if cmd == "claude" || cmd == "clawde" {
			claudePanes = append(claudePanes, p)
		}
	}
	return claudePanes, nil
}

// CapturePane captures the content of a pane
// If withScrollback is true, captures the entire scrollback history
func CapturePane(paneID string, withScrollback bool) (string, error) {
	args := []string{"capture-pane", "-t", paneID, "-p"}
	if withScrollback {
		args = append(args, "-S", "-")
	}

	cmd := exec.Command("tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane %s: %w", paneID, err)
	}
	return string(output), nil
}

// SendKeys sends keys to a pane
func SendKeys(paneID string, keys ...string) error {
	args := append([]string{"send-keys", "-t", paneID}, keys...)
	cmd := exec.Command("tmux", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send keys to pane %s: %w", paneID, err)
	}
	return nil
}

// CapturePaneLines captures the last N lines from a pane's scrollback
func CapturePaneLines(paneID string, lines int) (string, error) {
	args := []string{"capture-pane", "-t", paneID, "-p", "-S", fmt.Sprintf("-%d", lines)}
	cmd := exec.Command("tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to capture pane %s: %w", paneID, err)
	}
	return string(output), nil
}

// StartPipePane starts streaming pane output to a shell command
func StartPipePane(paneID, command string) error {
	cmd := exec.Command("tmux", "pipe-pane", "-t", paneID, command)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start pipe-pane for %s: %w", paneID, err)
	}
	return nil
}

// StopPipePane stops streaming pane output (by calling pipe-pane with no command)
func StopPipePane(paneID string) error {
	cmd := exec.Command("tmux", "pipe-pane", "-t", paneID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop pipe-pane for %s: %w", paneID, err)
	}
	return nil
}
