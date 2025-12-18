package main

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type TmuxInsertDetector struct {
	isInsertMode bool
	mutex        sync.RWMutex
	stopChan     chan struct{}
	pollInterval time.Duration
}

func NewTmuxInsertDetector(pollInterval time.Duration) *TmuxInsertDetector {
	return &TmuxInsertDetector{
		pollInterval: pollInterval,
		stopChan:     make(chan struct{}),
	}
}

// IsRunningInTmux checks if we're inside a tmux session
func IsRunningInTmux() bool {
	return os.Getenv("TMUX") != ""
}

// Start begins polling tmux for pane contents
func (t *TmuxInsertDetector) Start() {
	go func() {
		ticker := time.NewTicker(t.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-t.stopChan:
				return
			case <-ticker.C:
				t.checkInsertMode()
			}
		}
	}()
}

func (t *TmuxInsertDetector) Stop() {
	close(t.stopChan)
}

func (t *TmuxInsertDetector) checkInsertMode() {
	// Capture entire visible pane
	cmd := exec.Command("tmux", "capture-pane", "-p")
	output, err := cmd.Output()
	if err != nil {
		return // Silently fail, keep previous state
	}

	newInsertMode := strings.Contains(string(output), "-- INSERT")

	t.mutex.Lock()
	if newInsertMode != t.isInsertMode {
		logger.Debug("INSERT mode changed", "insert_mode", newInsertMode)
	}
	t.isInsertMode = newInsertMode
	t.mutex.Unlock()
}

func (t *TmuxInsertDetector) IsInsertMode() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.isInsertMode
}
