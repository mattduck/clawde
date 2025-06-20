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
	ActionType   string   // "?" for questions, "!" for commands, ":" for context
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
			regexp.MustCompile(`(?i)^\s*//\s*(.*AI[?!:].*)`),
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
			regexp.MustCompile(`(?i)^\s*//\s*(.*AI[?!:].*)`),
		},
		Multiline: []MultilineCommentPair{
			{
				Start: regexp.MustCompile(`/\*`),
				End:   regexp.MustCompile(`\*/`),
			},
		},
	},
	".py": {
		SingleLine: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^\s*#\s*(.*AI[?!:].*)`),
		},
		Multiline: []MultilineCommentPair{
			{
				Start: regexp.MustCompile(`"""`),
				End:   regexp.MustCompile(`"""`),
			},
			{
				Start: regexp.MustCompile(`'''`),
				End:   regexp.MustCompile(`'''`),
			},
		},
	},
}

// Cache for processed comments to avoid reprocessing
var processedComments = make(map[string]bool)

// Maximum comment content length before truncation
const maxCommentLength = 1000

// truncateComment truncates comment content if it exceeds maxCommentLength
func truncateComment(content string) string {
	if len(content) <= maxCommentLength {
		return content
	}
	return content[:maxCommentLength] + "...(truncated)"
}

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
	processedLines := make(map[int]bool) // Track which lines we've already processed

	// Determine comment prefix from file extension
	ext := filepath.Ext(filePath)
	var commentPrefix string
	switch ext {
	case ".py":
		commentPrefix = "#"
	case ".go", ".js":
		commentPrefix = "//"
	default:
		commentPrefix = "//" // Default fallback
	}

	for i, line := range lines {
		if processedLines[i] {
			continue // Skip lines already processed as part of a comment block
		}

		// Check if this line contains a comment at all
		if !strings.Contains(line, commentPrefix) {
			continue
		}

		// Check if this is a whole-line comment vs inline comment
		beforeComment := strings.Split(line, commentPrefix)[0]
		isWholeLine := strings.TrimSpace(beforeComment) == ""

		if isWholeLine {
			// Look for consecutive whole-line comments to group them
			commentLines := []string{line}
			endLine := i

			// Check subsequent lines for consecutive whole-line comments
			for j := i + 1; j < len(lines); j++ {
				// Check if the line contains a comment (even without AI marker)
				if strings.Contains(lines[j], commentPrefix) {
					beforeNextComment := strings.Split(lines[j], commentPrefix)[0]
					if strings.TrimSpace(beforeNextComment) == "" {
						// This is also a whole-line comment
						commentLines = append(commentLines, lines[j])
						processedLines[j] = true
						endLine = j
					} else {
						break // Next comment is inline, don't group it
					}
				} else {
					break // Next line is not a comment
				}
			}

			// Extract all comment content and combine
			var allContent []string
			for _, commentLine := range commentLines {
				// Extract comment content after comment prefix
				if parts := strings.Split(commentLine, commentPrefix); len(parts) >= 2 {
					content := strings.TrimSpace(strings.Join(parts[1:], commentPrefix))
					if content != "" {
						allContent = append(allContent, content)
					}
				}
			}
			combinedContent := truncateComment(strings.Join(allContent, " "))

			// Check if the combined content has AI markers
			// Priority: AI! and AI? take precedence over AI:
			// AI: is only supported at the start, not at the end
			var actionType string
			lowerContent := strings.ToLower(combinedContent)
			
			// Check for AI! first (highest priority) - can be at start or end
			if strings.HasSuffix(lowerContent, " ai!") || lowerContent == "ai!" || strings.HasPrefix(lowerContent, "ai!") {
				actionType = "!"
			} else if strings.HasSuffix(lowerContent, " ai?") || lowerContent == "ai?" || strings.HasPrefix(lowerContent, "ai?") {
				actionType = "?"
			} else if strings.HasPrefix(lowerContent, "ai:") {
				// AI: only supported at start
				actionType = ":"
			} else {
				// AI marker is in the middle or not present - skip this comment
				continue
			}

			comment := AIComment{
				FilePath:   filePath,
				LineNumber: i + 1,       // 1-indexed
				EndLine:    endLine + 1, // End line (1-indexed), same as start for single line
				Content:    combinedContent,
				FullLine:   strings.Join(commentLines, "\n"),
				ActionType: actionType,
			}

			// For single-line blocks, set EndLine to 0 to indicate single-line
			if len(commentLines) == 1 {
				comment.EndLine = 0
			}

			// Generate hash for caching
			comment.Hash = generateCommentHash(comment)

			// Add context lines (5 lines before and after)
			comment.ContextLines = extractContextLines(lines, i, 5)

			comments = append(comments, comment)
			if len(commentLines) == 1 {
				log.Printf("Found single-line AI comment at %s:%d - %s", filePath, i+1, combinedContent)
			} else {
				log.Printf("Found multi-line single-line AI comment block at %s:%d-%d - %s", filePath, i+1, endLine+1, combinedContent)
			}
		} else {
			// Handle inline comments individually (don't group them)
			// Extract comment content after comment prefix
			if parts := strings.Split(line, commentPrefix); len(parts) >= 2 {
				commentContent := truncateComment(strings.TrimSpace(strings.Join(parts[1:], commentPrefix)))

				// Check if it contains AI markers
				// Priority: AI! and AI? take precedence over AI:
				// AI: is only supported at the start, not at the end
				var actionType string
				lowerContent := strings.ToLower(commentContent)
				
				// Check for AI! first (highest priority) - can be at start or end
				if strings.HasSuffix(lowerContent, " ai!") || lowerContent == "ai!" || strings.HasPrefix(lowerContent, "ai!") {
					actionType = "!"
				} else if strings.HasSuffix(lowerContent, " ai?") || lowerContent == "ai?" || strings.HasPrefix(lowerContent, "ai?") {
					actionType = "?"
				} else if strings.HasPrefix(lowerContent, "ai:") {
					// AI: only supported at start
					actionType = ":"
				} else {
					// AI marker is in the middle or not present - skip this comment
					continue
				}

				comment := AIComment{
					FilePath:   filePath,
					LineNumber: i + 1, // 1-indexed
					EndLine:    0,     // 0 indicates single-line comment
					Content:    commentContent,
					FullLine:   line,
					ActionType: actionType,
				}

				// Generate hash for caching
				comment.Hash = generateCommentHash(comment)

				// Add context lines (5 lines before and after)
				comment.ContextLines = extractContextLines(lines, i, 5)

				comments = append(comments, comment)
				log.Printf("Found inline AI comment at %s:%d - %s", filePath, i+1, commentContent)
			}
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
				lowerComment := strings.ToLower(fullComment)
				if strings.Contains(lowerComment, "ai?") || strings.Contains(lowerComment, "ai!") || strings.Contains(lowerComment, "ai:") {
					// Priority: AI! and AI? take precedence over AI:
					actionType := "?"
					if strings.Contains(lowerComment, "ai!") {
						actionType = "!"
					} else if strings.Contains(lowerComment, "ai?") {
						actionType = "?"
					} else {
						actionType = ":"
					}

					// Extract content by removing comment markers
					content := truncateComment(extractMultilineContent(fullComment))

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
				lowerComment := strings.ToLower(fullComment)
				if strings.Contains(lowerComment, "ai?") || strings.Contains(lowerComment, "ai!") || strings.Contains(lowerComment, "ai:") {
					// Priority: AI! and AI? take precedence over AI:
					actionType := "?"
					if strings.Contains(lowerComment, "ai!") {
						actionType = "!"
					} else if strings.Contains(lowerComment, "ai?") {
						actionType = "?"
					} else {
						actionType = ":"
					}

					// Extract content by removing comment markers.
					content := truncateComment(extractMultilineContent(fullComment))

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
	// Remove various multiline comment markers
	content := fullComment
	
	// Remove C-style /* */ markers
	content = strings.ReplaceAll(content, "/*", "")
	content = strings.ReplaceAll(content, "*/", "")
	
	// Remove Python triple quote markers
	content = strings.ReplaceAll(content, `"""`, "")
	content = strings.ReplaceAll(content, `'''`, "")

	// Split into lines and clean each line
	lines := strings.Split(content, "\n")
	var cleanLines []string
	for _, line := range lines {
		// Remove leading * and whitespace (for C-style comments)
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
