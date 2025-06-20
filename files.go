package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher manages file system monitoring
type FileWatcher struct {
	watcher      *fsnotify.Watcher
	watchDir     string
	onFileChange func(string) // Callback for file changes
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(watchDir string, onFileChange func(string)) (*FileWatcher, error) {
	// Check if the watch directory exists
	if _, err := os.Stat(watchDir); os.IsNotExist(err) {
		log.Printf("ERROR: Watch directory does not exist: %s", watchDir)
		return nil, fmt.Errorf("watch directory does not exist: %s", watchDir)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("ERROR: Failed to create file watcher: %v", err)
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	fw := &FileWatcher{
		watcher:      watcher,
		watchDir:     watchDir,
		onFileChange: onFileChange,
	}

	log.Printf("File watcher created successfully for directory: %s", watchDir)
	return fw, nil
}

// Start begins watching for file changes
func (fw *FileWatcher) Start() error {
	// Add the directory and all subdirectories to the watcher recursively
	err := fw.addDirectoriesRecursively(fw.watchDir)
	if err != nil {
		log.Printf("ERROR: Failed to add directories to watcher: %v", err)
		return fmt.Errorf("failed to add directories to watcher: %w", err)
	}

	// Start the event processing goroutine
	go fw.processEvents()

	// List files in the directory for debugging
	files, err := filepath.Glob(filepath.Join(fw.watchDir, "*"))
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
	log.Printf("File watcher goroutine started, listening for events...")

	for {
		select {
		case event, ok := <-fw.watcher.Events:
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

			// Handle directory creation events - add new directories to watcher
			if event.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if shouldIgnoreDirectory(event.Name) {
						log.Printf("Ignoring creation of ignored directory: %s", event.Name)
					} else {
						log.Printf("New directory created: %s", event.Name)
						if err := fw.addDirectoriesRecursively(event.Name); err != nil {
							log.Printf("WARNING: Failed to add new directory %s to watcher: %v", event.Name, err)
						} else {
							log.Printf("Successfully added new directory %s and subdirectories to watcher", event.Name)
						}
					}
				}
			}

			// React to write and create events on specific file types
			// Many editors use atomic replacement (create temp file, rename) instead of direct writes
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				// Skip files in ignored directories
				if shouldIgnoreDirectory(filepath.Dir(event.Name)) {
					log.Printf("Ignoring file in ignored directory: %s", event.Name)
				} else {
					ext := filepath.Ext(event.Name)
					log.Printf("File extension detected: %s for file %s", ext, event.Name)

					// Skip temporary files (ending with ~, .tmp, .swp, etc.)
					if strings.HasSuffix(event.Name, "~") ||
						strings.HasSuffix(event.Name, ".tmp") ||
						strings.HasSuffix(event.Name, ".swp") ||
						strings.Contains(event.Name, ".#") {
						log.Printf("Ignoring temporary file: %s", event.Name)
					} else if ext == ".py" || ext == ".js" || ext == ".go" {
						// Skip test files (contain false positives)
						if filepath.Base(event.Name) == "test_comments.go" || filepath.Base(event.Name) == "comment_test.go" {
							log.Printf("Ignoring test file: %s", event.Name)
						} else {
							log.Printf("File change detected for monitored extension: %s", event.Name)

							// Call the callback function with the file path
							if fw.onFileChange != nil {
								fw.onFileChange(event.Name)
							}
						}
					} else {
						log.Printf("Ignoring file change for unmonitored extension: %s (file: %s)", ext, event.Name)
					}
				}
			} else {
				log.Printf("Ignoring event type %s for file %s", event.Op.String(), event.Name)
			}

		case err, ok := <-fw.watcher.Errors:
			log.Printf("File watcher error: %v", err)
			if !ok {
				log.Printf("File watcher errors channel closed")
				return
			}
		}
	}
}

// shouldIgnoreDirectory checks if a directory should be ignored
func shouldIgnoreDirectory(dirPath string) bool {
	dirName := filepath.Base(dirPath)

	// Common directories to ignore
	ignoredDirs := []string{
		".git",
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
			log.Printf("WARNING: Error accessing path %s: %v", path, err)
			return nil // Continue walking even if one directory fails
		}

		// Only add directories to the watcher
		if info.IsDir() {
			// Skip ignored directories
			if shouldIgnoreDirectory(path) {
				log.Printf("Skipping ignored directory: %s", path)
				return filepath.SkipDir // Don't recurse into this directory
			}

			log.Printf("Adding directory to watcher: %s", path)
			if err := fw.watcher.Add(path); err != nil {
				log.Printf("WARNING: Failed to add directory %s to watcher: %v", path, err)
				return nil // Continue walking even if one directory fails
			}
			log.Printf("Successfully added directory to watcher: %s", path)
		}

		return nil
	})
}

// FindFilesWithAIComments searches for files containing AI-related comments.
// this is a basic search to prune the potential files that we need to search in
// more depth.
func FindFilesWithAIComments(rootDir string) ([]string, error) {
	var files []string
	var mutex sync.Mutex
	var wg sync.WaitGroup

	log.Printf("Starting search for files with AI comments in directory: %s", rootDir)

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("WARNING: Error accessing path %s: %v", path, err)
			return nil // Continue walking even if one path fails
		}

		// Only process files (not directories)
		if !info.IsDir() {
			// Check if file has a supported extension
			ext := filepath.Ext(path)
			if _, exists := commentPatterns[ext]; exists {
				// Skip ignored directories
				if shouldIgnoreDirectory(filepath.Dir(path)) {
					return nil
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

	log.Printf("Found %d files with AI comments", len(files))
	return files, nil
}

// hasAIComments quickly checks if a file contains AI-related comments
func hasAIComments(filePath string) bool {
	// Read file contents
	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("WARNING: Failed to read file %s: %v", filePath, err)
		return false
	}

	// Simple case-insensitive search for AI markers
	lowerContent := strings.ToLower(string(content))
	return strings.Contains(lowerContent, "ai?") || strings.Contains(lowerContent, "ai!") || strings.Contains(lowerContent, "ai:")
}
