package main

// NO_CLAWDE - This file contains AI marker examples and should be excluded from comment detection

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

// MultilineTokenPair represents the actual tokens for multiline comments
type MultilineTokenPair struct {
	Start string // The start token (e.g., "/*")
	End   string // The end token (e.g., "*/")
}

// CommentPattern defines how to detect comments in different file types
type CommentPattern struct {
	SingleLine       []*regexp.Regexp       // Multiple single-line comment patterns
	Multiline        []MultilineCommentPair // Paired start/end patterns for multiline comments
	SingleLineTokens []string               // The actual single-line tokens (e.g., "//", "#")
	MultilineTokens  []MultilineTokenPair   // The actual multiline tokens
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
		SingleLineTokens: []string{"//"},
		MultilineTokens: []MultilineTokenPair{
			{Start: "/*", End: "*/"},
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
		SingleLineTokens: []string{"//"},
		MultilineTokens: []MultilineTokenPair{
			{Start: "/*", End: "*/"},
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
		SingleLineTokens: []string{"#"},
		MultilineTokens: []MultilineTokenPair{
			{Start: `"""`, End: `"""`},
			{Start: `'''`, End: `'''`},
		},
	},
}

// Cache for processed comments to avoid reprocessing
var processedComments = make(map[string]bool)

// Size limits to prevent performance issues with large files/lines
const (
	maxFileSize       = 10 * 1024 * 1024 // 10MB - skip files larger than this
	maxLineLength     = 10 * 1024        // 10KB - skip lines longer than this
	maxTotalLines     = 50000            // Skip files with more lines than this
	maxFilesToSearch  = 10000            // Stop searching after this many files
	maxCommentLength  = 1000             // Maximum comment content length before truncation
)

// truncateComment truncates comment content if it exceeds maxCommentLength
func truncateComment(content string) string {
	if len(content) <= maxCommentLength {
		return content
	}
	return content[:maxCommentLength] + "...(truncated)"
}

// checkForOptOut scans file content for NO_CLAWDE marker in any comment type
func checkForOptOut(content string, ext string) bool {
	lines := strings.Split(content, "\n")
	
	// Get comment patterns for this file extension
	patterns, exists := commentPatterns[ext]
	if !exists {
		return false
	}

	// Check single-line comments for NO_CLAWDE
	for _, line := range lines {
		for _, token := range patterns.SingleLineTokens {
			if strings.Contains(line, token) {
				// Extract comment content after the token
				parts := strings.Split(line, token)
				if len(parts) >= 2 {
					commentContent := strings.Join(parts[1:], token)
					if strings.Contains(strings.ToLower(commentContent), "no_clawde") {
						log.Printf("Found NO_CLAWDE opt-out marker in file, skipping comment processing")
						return true
					}
				}
			}
		}
	}

	// Check multiline comments for NO_CLAWDE
	for _, pair := range patterns.Multiline {
		inComment := false
		var commentLines []string

		for _, line := range lines {
			if !inComment && pair.Start.MatchString(line) {
				inComment = true
				commentLines = []string{line}
				
				// Check if end pattern is also on the same line (single-line multiline comment)
				if pair.End.MatchString(line) {
					// Process the comment immediately
					fullComment := strings.Join(commentLines, "\n")
					if strings.Contains(strings.ToLower(fullComment), "no_clawde") {
						log.Printf("Found NO_CLAWDE opt-out marker in file, skipping comment processing")
						return true
					}
					inComment = false
					commentLines = nil
				}
				continue
			}

			if inComment {
				commentLines = append(commentLines, line)
				if pair.End.MatchString(line) {
					// Check the complete multiline comment
					fullComment := strings.Join(commentLines, "\n")
					if strings.Contains(strings.ToLower(fullComment), "no_clawde") {
						log.Printf("Found NO_CLAWDE opt-out marker in file, skipping comment processing")
						return true
					}
					inComment = false
					commentLines = nil
				}
			}
		}
	}

	return false
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

	// Check file size before reading
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}
	if fileInfo.Size() > maxFileSize {
		log.Printf("Skipping file %s: size %d bytes exceeds limit %d bytes", filePath, fileInfo.Size(), maxFileSize)
		return nil, nil
	}

	// Read file contents
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Check if file has opted out of comment detection
	if checkForOptOut(string(content), ext) {
		return nil, nil
	}

	lines := strings.Split(string(content), "\n")
	
	// Check total line count
	if len(lines) > maxTotalLines {
		log.Printf("Skipping file %s: %d lines exceeds limit %d lines", filePath, len(lines), maxTotalLines)
		return nil, nil
	}
	
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

		// Check line length
		if len(line) > maxLineLength {
			log.Printf("Skipping line %d in %s: length %d exceeds limit %d", i+1, filePath, len(line), maxLineLength)
			continue
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

			// Check if any line in the comment block has AI markers
			// Priority: AI! and AI? take precedence over AI:
			// AI: is only supported at the start, not at the end
			actionType := checkAIMarkerInLines(allContent)
			if actionType == "" {
				// No AI marker found in any line - skip this comment
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
				actionType := checkAIMarkerInLines([]string{commentContent})
				if actionType == "" {
					// No AI marker found - skip this comment
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
		// Check line length
		if len(line) > maxLineLength {
			log.Printf("Skipping line %d in %s: length %d exceeds limit %d", i+1, filePath, len(line), maxLineLength)
			continue
		}

		if !inComment && pair.Start.MatchString(line) {
			inComment = true
			startLine = i
			commentLines = []string{line}

			// Check if end pattern is also on the same line (single-line multiline comment)
			// Only treat as single-line if there's actual content between start and end markers
			if pair.End.MatchString(line) && hasContentBetweenMarkers(line, pair) {
				// Process the comment immediately
				fullComment := strings.Join(commentLines, "\n")
				if hasValidAIMarker(fullComment, filepath.Ext(filePath)) {
					actionType := determineActionType(fullComment, filepath.Ext(filePath))

					// Extract content by removing comment markers
					content := truncateComment(extractMultilineContentForExt(fullComment, filepath.Ext(filePath)))

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
				// Check if the comment block contains valid AI markers
				fullComment := strings.Join(commentLines, "\n")
				if hasValidAIMarker(fullComment, filepath.Ext(filePath)) {
					actionType := determineActionType(fullComment, filepath.Ext(filePath))

					// Extract content by removing comment markers.
					content := truncateComment(extractMultilineContentForExt(fullComment, filepath.Ext(filePath)))

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

// hasContentBetweenMarkers checks if there's actual content between start and end markers on the same line
func hasContentBetweenMarkers(line string, pair MultilineCommentPair) bool {
	// For symmetric markers (like """ or '''), check if there's content between them
	if pair.Start.String() == pair.End.String() {
		// Find all matches of the marker
		matches := pair.Start.FindAllStringIndex(line, -1)
		if len(matches) >= 2 {
			// Check if there's non-whitespace content between first and last match
			start := matches[0][1]            // End of first marker
			end := matches[len(matches)-1][0] // Start of last marker
			between := strings.TrimSpace(line[start:end])
			return between != ""
		}
		return false
	}

	// For asymmetric markers (like /* */), check if there's content between them
	startLoc := pair.Start.FindStringIndex(line)
	endLoc := pair.End.FindStringIndex(line)
	if startLoc != nil && endLoc != nil && startLoc[1] <= endLoc[0] {
		between := strings.TrimSpace(line[startLoc[1]:endLoc[0]])
		return between != ""
	}

	return false
}

// hasValidAIMarker checks if a multiline comment has AI markers at valid positions
func hasValidAIMarker(fullComment string, ext string) bool {
	// Get the cleaned lines using the language-specific token removal
	lines := extractMultilineContentLines(fullComment, ext)

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Trim trailing space for consistent marker detection
		lowerLine := strings.ToLower(strings.TrimSpace(line))

		// Check if AI? or AI! is at the end of the line
		if strings.HasSuffix(lowerLine, " ai?") || lowerLine == "ai?" ||
			strings.HasSuffix(lowerLine, " ai!") || lowerLine == "ai!" {
			return true
		}

		// Check if AI: is at the start of the line
		if strings.HasPrefix(lowerLine, "ai:") {
			return true
		}

		// Also check for AI? or AI! at the start (for consistency with single-line)
		if strings.HasPrefix(lowerLine, "ai?") || strings.HasPrefix(lowerLine, "ai!") {
			return true
		}
	}

	return false
}

// determineActionType determines the action type based on AI markers in the comment
func determineActionType(fullComment string, ext string) string {
	// Get the cleaned lines using the language-specific token removal
	lines := extractMultilineContentLines(fullComment, ext)

	hasQuestion := false
	hasCommand := false
	hasContext := false

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Trim trailing space for consistent marker detection
		lowerLine := strings.ToLower(strings.TrimSpace(line))

		// Check for ! (highest priority)
		if strings.HasSuffix(lowerLine, " ai!") || lowerLine == "ai!" || strings.HasPrefix(lowerLine, "ai!") {
			hasCommand = true
		}

		// Check for ?
		if strings.HasSuffix(lowerLine, " ai?") || lowerLine == "ai?" || strings.HasPrefix(lowerLine, "ai?") {
			hasQuestion = true
		}

		// Check for :
		if strings.HasPrefix(lowerLine, "ai:") {
			hasContext = true
		}
	}

	// Priority: AI! > AI? > AI:
	if hasCommand {
		return "!"
	} else if hasQuestion {
		return "?"
	} else if hasContext {
		return ":"
	}

	// This should never happen if hasValidAIMarker returned true
	log.Fatalf("Internal error: determineActionType called but no valid AI marker found in comment: %s", fullComment)
	return ""
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

// extractMultilineContentLines removes comment markers and returns individual lines
func extractMultilineContentLines(fullComment string, ext string) []string {
	content := fullComment

	// Get tokens for this file extension
	if ext != "" {
		if patterns, exists := commentPatterns[ext]; exists {
			// Remove multiline comment markers using language-specific tokens
			for _, tokenPair := range patterns.MultilineTokens {
				content = strings.ReplaceAll(content, tokenPair.Start, "")
				content = strings.ReplaceAll(content, tokenPair.End, "")
			}
		}
	} else {
		// Fallback: remove all known multiline comment markers for backward compatibility
		for _, patterns := range commentPatterns {
			for _, tokenPair := range patterns.MultilineTokens {
				content = strings.ReplaceAll(content, tokenPair.Start, "")
				content = strings.ReplaceAll(content, tokenPair.End, "")
			}
		}
	}

	// Split into lines and clean each line
	lines := strings.Split(content, "\n")
	var cleanLines []string
	for _, line := range lines {
		// Remove leading * and whitespace (for C-style comments)
		cleaned := strings.TrimSpace(line)
		cleaned = strings.TrimPrefix(cleaned, "*")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned != "" {
			// Ensure proper spacing between lines by adding a space at the end
			// if the line doesn't already end with whitespace
			if !strings.HasSuffix(cleaned, " ") {
				cleaned += " "
			}
			cleanLines = append(cleanLines, cleaned)
		}
	}

	return cleanLines
}

// extractMultilineContentForExt removes comment markers using language-specific tokens
func extractMultilineContentForExt(fullComment string, ext string) string {
	cleanLines := extractMultilineContentLines(fullComment, ext)
	// Join without additional spaces since each line already has proper spacing
	joined := strings.Join(cleanLines, "")
	// Trim any trailing space that might have been added to the last line
	return strings.TrimSpace(joined)
}

// extractMultilineContent removes comment markers from multiline comment content
func extractMultilineContent(fullComment string) string {
	return extractMultilineContentForExt(fullComment, "")
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

// checkAIMarkerInLines checks if any line in a slice of lines contains AI markers
// Returns the action type ("!", "?", ":") or empty string if no marker found
// Returns the first non-colon marker found, or ":" if only colon markers exist
func checkAIMarkerInLines(lines []string) string {
	hasContext := false
	
	for _, line := range lines {
		if line == "" {
			continue
		}

		lowerLine := strings.ToLower(line)

		// Check for ! or ? - return immediately if found (first non-colon marker wins)
		if strings.HasSuffix(lowerLine, " ai!") || lowerLine == "ai!" || strings.HasPrefix(lowerLine, "ai!") {
			return "!"
		}
		if strings.HasSuffix(lowerLine, " ai?") || lowerLine == "ai?" || strings.HasPrefix(lowerLine, "ai?") {
			return "?"
		}
		// Remember if we saw a colon marker, but don't return it yet
		if strings.HasPrefix(lowerLine, "ai:") {
			hasContext = true
		}
	}

	// Only return ":" if we found colon markers but no ! or ? markers
	if hasContext {
		return ":"
	}

	return ""
}
