package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// AIComment represents an AI-related comment found in source code
type AIComment struct {
	FilePath     string   // Path to the file containing the comment
	LineNumber   int      // Line number where the comment appears (1-indexed)
	EndLine      int      // End line number for multiline comments (0 for single-line)
	Content      string   // The comment content (stripped of comment markers)
	FullLine     string   // The complete line containing the comment
	ContextLines []string // Surrounding lines for context
	ActionType   string   // "?" for questions, "!" for commands
	Hash         string   // Fingerprint for caching/deduplication
}

// MultilineCommentPair represents a paired start/end pattern for multiline comments
type MultilineCommentPair struct {
	Start *regexp.Regexp // Pattern to match comment start (e.g., /*)
	End   *regexp.Regexp // Pattern to match comment end (e.g., */)
}

// CommentPattern defines how to detect comments in different file types
type CommentPattern struct {
	SingleLine []*regexp.Regexp       // Multiple single-line comment patterns
	Multiline  []MultilineCommentPair // Paired start/end patterns for multiline comments
}

// Comment patterns for different file extensions
var commentPatterns = map[string]CommentPattern{
	".go": {
		SingleLine: []*regexp.Regexp{
			regexp.MustCompile(`^\s*//\s*(.*AI[?!].*)`),
		},
		Multiline: []MultilineCommentPair{
			{
				Start: regexp.MustCompile(`/\*`),
				End:   regexp.MustCompile(`\*/`),
			},
		},
	},
	".js": {
		SingleLine: []*regexp.Regexp{
			regexp.MustCompile(`^\s*//\s*(.*AI[?!].*)`),
		},
		Multiline: []MultilineCommentPair{
			{
				Start: regexp.MustCompile(`/\*`),
				End:   regexp.MustCompile(`\*/`),
			},
		},
	},
}

// Cache for processed comments to avoid reprocessing
var processedComments = make(map[string]bool)

// ExtractAIComments scans a file for AI-related comments
func ExtractAIComments(filePath string) ([]AIComment, error) {
	// Get file extension to determine comment patterns
	ext := filepath.Ext(filePath)
	patterns, exists := commentPatterns[ext]
	if !exists {
		log.Printf("No comment patterns defined for file extension: %s", ext)
		return nil, nil
	}

	// Read file contents
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	lines := strings.Split(string(content), "\n")
	var comments []AIComment

	// Check single-line comments
	for _, pattern := range patterns.SingleLine {
		foundComments := extractSingleLineComments(filePath, lines, pattern)
		comments = append(comments, foundComments...)
	}

	// Check multiline comments
	for _, pair := range patterns.Multiline {
		foundComments := extractMultilineComments(filePath, lines, pair)
		comments = append(comments, foundComments...)
	}

	log.Printf("Found %d AI comments in %s", len(comments), filePath)
	return comments, nil
}

// extractSingleLineComments finds AI comments in single-line comment patterns
func extractSingleLineComments(filePath string, lines []string, pattern *regexp.Regexp) []AIComment {
	var comments []AIComment

	for i, line := range lines {
		matches := pattern.FindStringSubmatch(line)
		if len(matches) >= 2 {
			commentContent := strings.TrimSpace(matches[1])

			// Check if AI marker is at start or end of comment content
			// Keep the full content including markers, just determine action type
			var actionType string

			if strings.HasPrefix(commentContent, "AI?") || strings.HasPrefix(commentContent, "AI!") {
				// Starts with AI marker
				if strings.HasPrefix(commentContent, "AI?") {
					actionType = "?"
				} else {
					actionType = "!"
				}
			} else if strings.HasSuffix(commentContent, "AI?") || strings.HasSuffix(commentContent, "AI!") {
				// Ends with AI marker
				if strings.HasSuffix(commentContent, "AI?") {
					actionType = "?"
				} else {
					actionType = "!"
				}
			} else {
				// AI marker is in the middle - skip this comment
				continue
			}

			comment := AIComment{
				FilePath:   filePath,
				LineNumber: i + 1, // 1-indexed
				EndLine:    0,      // 0 indicates single-line comment
				Content:    commentContent,
				FullLine:   line,
				ActionType: actionType,
			}

			// Generate hash for caching
			comment.Hash = generateCommentHash(comment)

			// Add context lines (5 lines before and after)
			comment.ContextLines = extractContextLines(lines, i, 5)

			comments = append(comments, comment)
			log.Printf("Found single-line AI comment at %s:%d - %s", filePath, i+1, commentContent)
		}
	}

	return comments
}

// extractMultilineComments finds AI comments in multiline comment blocks
func extractMultilineComments(filePath string, lines []string, pair MultilineCommentPair) []AIComment {
	var comments []AIComment
	inComment := false
	var commentLines []string
	var startLine int

	for i, line := range lines {
		if !inComment && pair.Start.MatchString(line) {
			inComment = true
			startLine = i
			commentLines = []string{line}

			// Check if end pattern is also on the same line (single-line multiline comment)
			if pair.End.MatchString(line) {
				// Process the comment immediately
				fullComment := strings.Join(commentLines, "\n")
				if strings.Contains(fullComment, "AI?") || strings.Contains(fullComment, "AI!") {
					actionType := "?"
					if strings.Contains(fullComment, "AI!") {
						actionType = "!"
					}

					// Extract content by removing comment markers
					content := extractMultilineContent(fullComment)

					comment := AIComment{
						FilePath:   filePath,
						LineNumber: startLine + 1, // 1-indexed
						EndLine:    i + 1,         // End line (1-indexed) - same as start for single-line multiline
						Content:    content,
						FullLine:   fullComment,
						ActionType: actionType,
					}

					// Generate hash for caching
					comment.Hash = generateCommentHash(comment)

					// Add context lines
					comment.ContextLines = extractContextLines(lines, startLine, 5)

					comments = append(comments, comment)
					log.Printf("Found multiline AI comment at %s:%d - %s", filePath, startLine+1, content)
				}

				inComment = false
				commentLines = nil
			}
			continue
		}

		if inComment {
			commentLines = append(commentLines, line)
			if pair.End.MatchString(line) {
				// Check if the comment block contains the keywords
				fullComment := strings.Join(commentLines, "\n")
				if strings.Contains(fullComment, "AI?") || strings.Contains(fullComment, "AI!") {
					actionType := "?"
					if strings.Contains(fullComment, "AI!") {
						actionType = "!"
					}

					// Extract content by removing comment markers. AI?
					content := extractMultilineContent(fullComment)

					comment := AIComment{
						FilePath:   filePath,
						LineNumber: startLine + 1, // 1-indexed
						EndLine:    i + 1,         // End line (1-indexed)
						Content:    content,
						FullLine:   fullComment,
						ActionType: actionType,
					}

					// Generate hash for caching
					comment.Hash = generateCommentHash(comment)

					// Add context lines
					comment.ContextLines = extractContextLines(lines, startLine, 5)

					comments = append(comments, comment)
					log.Printf("Found multiline AI comment at %s:%d - %s", filePath, startLine+1, content)
				}

				inComment = false
				commentLines = nil
			}
		}
	}

	return comments
}

// extractContextLines gets N lines before and after the target line
func extractContextLines(lines []string, targetLine, contextSize int) []string {
	start := targetLine - contextSize
	if start < 0 {
		start = 0
	}
	end := targetLine + contextSize + 1
	if end > len(lines) {
		end = len(lines)
	}

	context := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		prefix := "  "
		if i == targetLine {
			prefix = "> " // Mark the target line
		}
		context = append(context, prefix+strconv.Itoa(i+1)+": "+lines[i])
	}

	return context
}

// extractMultilineContent removes comment markers from multiline comment content
func extractMultilineContent(fullComment string) string {
	// Remove /* and */ markers and clean up
	content := strings.ReplaceAll(fullComment, "/*", "")
	content = strings.ReplaceAll(content, "*/", "")

	// Split into lines and clean each line
	lines := strings.Split(content, "\n")
	var cleanLines []string
	for _, line := range lines {
		// Remove leading * and whitespace
		cleaned := strings.TrimSpace(line)
		cleaned = strings.TrimPrefix(cleaned, "*")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned != "" {
			cleanLines = append(cleanLines, cleaned)
		}
	}

	return strings.Join(cleanLines, " ")
}

// generateCommentHash creates a fingerprint for comment caching
func generateCommentHash(comment AIComment) string {
	data := fmt.Sprintf("%s:%d:%s:%s", comment.FilePath, comment.LineNumber, comment.Content, comment.ActionType)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes for shorter hash
}

// isCommentProcessed checks if a comment has already been processed.
func isCommentProcessed(comment AIComment) bool {
	return processedComments[comment.Hash]
}

// markCommentProcessed marks a comment as processed in the cache
func markCommentProcessed(comment AIComment) {
	processedComments[comment.Hash] = true
}

// clearProcessedCache clears the processed comments cache
func clearProcessedCache() {
	processedComments = make(map[string]bool)
}
