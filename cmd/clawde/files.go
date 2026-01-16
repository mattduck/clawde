package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// GitIgnoreCache stores git-ignored files for fast lookup
type GitIgnoreCache struct {
	ignoredFiles map[string]bool
	isGitRepo    bool
	mu           sync.RWMutex
}

// NewGitIgnoreCache creates a new GitIgnoreCache and populates it
func NewGitIgnoreCache(watchDir string) *GitIgnoreCache {
	cache := &GitIgnoreCache{
		ignoredFiles: make(map[string]bool),
		isGitRepo:    false,
	}

	// Check if we're in a git repository
	gitDir := filepath.Join(watchDir, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		cache.isGitRepo = true
		logger.Info("Git repository detected", "dir", watchDir)
		cache.loadGitIgnoredFiles(watchDir)
	} else {
		logger.Info("Not a git repository", "dir", watchDir)
	}

	return cache
}

// loadGitIgnoredFiles runs git ls-files to get ignored files
func (g *GitIgnoreCache) loadGitIgnoredFiles(watchDir string) {
	// First, find the git repository root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = watchDir

	var rootOut bytes.Buffer
	cmd.Stdout = &rootOut

	err := cmd.Run()
	if err != nil {
		logger.Warn("Failed to find git root", "error", err)
		return
	}

	gitRoot := strings.TrimSpace(rootOut.String())
	logger.Info("Git repository root", "root", gitRoot)

	// Now get the list of ignored files from the git root
	cmd = exec.Command("git", "ls-files", "--ignored", "--exclude-standard", "--others")
	cmd.Dir = gitRoot

	var out bytes.Buffer
	cmd.Stdout = &out

	err = cmd.Run()
	if err != nil {
		logger.Warn("Failed to run git ls-files", "error", err)
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	// Parse the output and store in map
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	for _, line := range lines {
		if line != "" {
			// Git ls-files returns paths relative to git root
			// Convert to absolute path by joining with git root
			absPath := filepath.Join(gitRoot, line)
			g.ignoredFiles[absPath] = true
		}
	}

	logger.Info("Loaded git-ignored files into cache", "count", len(g.ignoredFiles))
}

// IsIgnored checks if a path is git-ignored
func (g *GitIgnoreCache) IsIgnored(path string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Check exact path match
	if g.ignoredFiles[path] {
		return true
	}

	// Check if any parent directory is ignored
	dir := path
	for {
		dir = filepath.Dir(dir)
		if dir == "." || dir == "/" {
			break
		}
		if g.ignoredFiles[dir] {
			return true
		}
	}

	return false
}

// FileWatcher manages file system monitoring
type FileWatcher struct {
	watcher      *fsnotify.Watcher
	watchDir     string
	onFileChange func(string) // Callback for file changes
	gitIgnore    *GitIgnoreCache
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(watchDir string, onFileChange func(string)) (*FileWatcher, error) {
	// Check if the watch directory exists
	if _, err := os.Stat(watchDir); os.IsNotExist(err) {
		logger.Error("Watch directory does not exist", "dir", watchDir)
		return nil, fmt.Errorf("watch directory does not exist: %s", watchDir)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("Failed to create file watcher", "error", err)
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Initialize git ignore cache
	gitIgnore := NewGitIgnoreCache(watchDir)

	fw := &FileWatcher{
		watcher:      watcher,
		watchDir:     watchDir,
		onFileChange: onFileChange,
		gitIgnore:    gitIgnore,
	}

	logger.Info("File watcher created successfully", "dir", watchDir)
	return fw, nil
}

// Start begins watching for file changes
func (fw *FileWatcher) Start() error {
	// Add the directory and all subdirectories to the watcher recursively
	err := fw.addDirectoriesRecursively(fw.watchDir)
	if err != nil {
		logger.Error("Failed to add directories to watcher", "error", err)
		return fmt.Errorf("failed to add directories to watcher: %w", err)
	}

	// Start the event processing goroutine
	go fw.processEvents()

	// List files in the directory for debugging
	files, err := filepath.Glob(filepath.Join(fw.watchDir, "*"))
	if err != nil {
		logger.Warn("Could not list files in watch directory", "error", err)
	} else {
		logger.Info("Files in watch directory", "count", len(files))
		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				logger.Debug("File stat error", "file", file, "error", err)
			} else {
				logger.Debug("File info", "file", file, "size", info.Size(), "mode", info.Mode())
			}
		}
	}

	return nil
}

// Close stops the file watcher
func (fw *FileWatcher) Close() error {
	if fw.watcher != nil {
		return fw.watcher.Close()
	}
	return nil
}

// processEvents handles file system events
func (fw *FileWatcher) processEvents() {
	defer fw.watcher.Close()
	logger.Info("File watcher goroutine started, listening for events")

	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				logger.Info("File watcher events channel closed")
				return
			}

			logger.Debug("Raw file event received", "file", event.Name, "op", event.Op.String())

			// Log all event types for debugging
			if event.Op&fsnotify.Create == fsnotify.Create {
				logger.Debug("CREATE event", "file", event.Name)
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				logger.Debug("WRITE event", "file", event.Name)
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				logger.Debug("REMOVE event", "file", event.Name)
			}
			if event.Op&fsnotify.Rename == fsnotify.Rename {
				logger.Debug("RENAME event", "file", event.Name)
			}
			if event.Op&fsnotify.Chmod == fsnotify.Chmod {
				logger.Debug("CHMOD event", "file", event.Name)
			}

			// Handle directory creation events - add new directories to watcher
			if event.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if fw.shouldIgnoreDirectory(event.Name) {
						logger.Debug("Ignoring creation of ignored directory", "name", event.Name)
					} else {
						logger.Info("New directory created", "name", event.Name)
						if err := fw.addDirectoriesRecursively(event.Name); err != nil {
							logger.Warn("Failed to add new directory to watcher", "dir", event.Name, "error", err)
						} else {
							logger.Info("Successfully added new directory and subdirectories to watcher", "name", event.Name)
						}
					}
				}
			}

			// React to write and create events on specific file types
			// Many editors use atomic replacement (create temp file, rename) instead of direct writes
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				// Skip files in ignored directories
				if fw.shouldIgnoreDirectory(filepath.Dir(event.Name)) {
					logger.Debug("Ignoring file in ignored directory", "name", event.Name)
				} else if fw.gitIgnore != nil && fw.gitIgnore.isGitRepo && fw.gitIgnore.IsIgnored(event.Name) {
					logger.Debug("Ignoring git-ignored file", "name", event.Name)
				} else {
					ext := filepath.Ext(event.Name)
					logger.Debug("File extension detected", "ext", ext, "file", event.Name)

					// Skip temporary files (ending with ~, .tmp, .swp, etc.)
					if strings.HasSuffix(event.Name, "~") ||
						strings.HasSuffix(event.Name, ".tmp") ||
						strings.HasSuffix(event.Name, ".swp") ||
						strings.Contains(event.Name, ".#") {
						logger.Debug("Ignoring temporary file", "name", event.Name)
					} else if ext == ".py" || ext == ".js" || ext == ".go" {
						// Skip test files (contain false positives)
						if filepath.Base(event.Name) == "test_comments.go" || filepath.Base(event.Name) == "comment_test.go" {
							logger.Debug("Ignoring test file", "name", event.Name)
						} else {
							logger.Info("File change detected for monitored extension", "name", event.Name)

							// Call the callback function with the file path
							if fw.onFileChange != nil {
								fw.onFileChange(event.Name)
							}
						}
					} else {
						logger.Debug("Ignoring file change for unmonitored extension", "ext", ext, "file", event.Name)
					}
				}
			} else {
				logger.Debug("Ignoring event type", "op", event.Op.String(), "file", event.Name)
			}

		case err, ok := <-fw.watcher.Errors:
			logger.Error("File watcher error", "error", err)
			if !ok {
				logger.Info("File watcher errors channel closed")
				return
			}
		}
	}
}

// shouldIgnoreDirectory checks if a directory should be ignored
func (fw *FileWatcher) shouldIgnoreDirectory(dirPath string) bool {
	// First check git ignore cache if available
	if fw.gitIgnore != nil && fw.gitIgnore.isGitRepo {
		if fw.gitIgnore.IsIgnored(dirPath) {
			return true
		}
	}

	dirName := filepath.Base(dirPath)

	// Always ignore .git directory
	if dirName == ".git" {
		return true
	}

	// Common directories to ignore
	ignoredDirs := []string{
		".svn",
		".hg",
		"node_modules",
		".vscode",
		".idea",
		"__pycache__",
		".pytest_cache",
		"target", // Rust/Java build dirs
		"build",
		"dist",
		".next",  // Next.js
		".nuxt",  // Nuxt.js
		"vendor", // Go/PHP vendor dirs
	}

	for _, ignored := range ignoredDirs {
		if dirName == ignored {
			return true
		}
	}

	// Ignore hidden directories that start with .
	if strings.HasPrefix(dirName, ".") && dirName != "." {
		return true
	}

	return false
}

// addDirectoriesRecursively walks the directory tree and adds all directories to the watcher
func (fw *FileWatcher) addDirectoriesRecursively(rootDir string) error {
	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Warn("Error accessing path", "path", path, "error", err)
			return nil // Continue walking even if one directory fails
		}

		// Only add directories to the watcher
		if info.IsDir() {
			// Skip ignored directories
			if fw.shouldIgnoreDirectory(path) {
				logger.Debug("Skipping ignored directory", "path", path)
				return filepath.SkipDir // Don't recurse into this directory
			}

			logger.Debug("Adding directory to watcher", "path", path)
			if err := fw.watcher.Add(path); err != nil {
				logger.Warn("Failed to add directory to watcher", "path", path, "error", err)
				return nil // Continue walking even if one directory fails
			}
			logger.Debug("Successfully added directory to watcher", "path", path)
		}

		return nil
	})
}

// FindFilesWithAIComments searches for files containing AI-related comments.
// this is a basic search to prune the potential files that we need to search in
// more depth.
func FindFilesWithAIComments(rootDir string, gitIgnore *GitIgnoreCache) ([]string, error) {
	var files []string
	var mutex sync.Mutex
	var wg sync.WaitGroup
	var fileCount int

	logger.Debug("Starting search for files with AI comments", "directory", rootDir)

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Warn("Error accessing path", "path", path, "error", err)
			return nil // Continue walking even if one path fails
		}

		// Only process files (not directories)
		if !info.IsDir() {
			// Check if file has a supported extension first
			ext := filepath.Ext(path)
			if _, exists := commentPatterns[ext]; exists {
				// Check file count limit for supported files only
				fileCount++
				if fileCount > maxFilesToSearch {
					logger.Warn("Stopping file search: reached limit", "limit", maxFilesToSearch)
					return filepath.SkipAll
				}
				// Skip ignored directories and files
				dirPath := filepath.Dir(path)

				// Check git ignore cache if available
				if gitIgnore != nil && gitIgnore.isGitRepo {
					// Check both the file and its directory
					if gitIgnore.IsIgnored(path) || gitIgnore.IsIgnored(dirPath) {
						return nil
					}
				}

				// Check standard ignore patterns
				dirName := filepath.Base(dirPath)
				if dirName == ".git" || strings.HasPrefix(dirName, ".") && dirName != "." {
					return nil
				}

				// Check common ignored directories
				ignoredDirs := []string{"node_modules", "__pycache__", ".pytest_cache", "vendor", "build", "dist"}
				for _, ignored := range ignoredDirs {
					if dirName == ignored {
						return nil
					}
				}

				// Skip test files (contain false positives)
				if filepath.Base(path) == "test_comments.go" || filepath.Base(path) == "comment_test.go" {
					return nil
				}

				wg.Add(1)
				go func(filePath string) {
					defer wg.Done()
					if hasAIComments(filePath) {
						mutex.Lock()
						files = append(files, filePath)
						mutex.Unlock()
					}
				}(path)
			}
		}

		return nil
	})

	wg.Wait()

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", rootDir, err)
	}

	logger.Debug("Found files with AI comments", "count", len(files))
	return files, nil
}

// hasAIComments quickly checks if a file contains AI-related comments
func hasAIComments(filePath string) bool {
	// Check file size before reading
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		logger.Warn("Failed to stat file", "file", filePath, "error", err)
		return false
	}
	if fileInfo.Size() > maxFileSize {
		logger.Debug("Skipping large file", "file", filePath, "size", fileInfo.Size(), "limit", maxFileSize)
		return false
	}

	// Read file contents
	content, err := os.ReadFile(filePath)
	if err != nil {
		logger.Warn("Failed to read file", "file", filePath, "error", err)
		return false
	}

	// Simple case-insensitive search for AI markers
	lowerContent := strings.ToLower(string(content))
	return strings.Contains(lowerContent, "ai?") || strings.Contains(lowerContent, "ai!") || strings.Contains(lowerContent, "ai:")
}
