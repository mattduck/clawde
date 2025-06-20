package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// extractAICommentsFromString parses AI comments from string content instead of file
func extractAICommentsFromString(content, filePath string) ([]AIComment, error) {
	// Get file extension to determine comment patterns
	ext := filepath.Ext(filePath)
	patterns, exists := commentPatterns[ext]
	if !exists {
		return nil, nil
	}

	lines := strings.Split(content, "\n")
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

	return comments, nil
}

func TestGoSingleLineComments(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
		wantType string
		wantContent string
		wantLine int
	}{
		{
			name:     "Go comment with AI?",
			content:  "package main\n\n// This is a test comment AI?\nfunc main() {}",
			expected: 1,
			wantType: "?",
			wantContent: "This is a test comment AI?",
			wantLine: 3,
		},
		{
			name:     "Go comment with AI!",
			content:  "package main\n\n// Fix this function AI!\nfunc main() {}",
			expected: 1,
			wantType: "!",
			wantContent: "Fix this function AI!",
			wantLine: 3,
		},
		{
			name:     "Go comment without AI marker",
			content:  "package main\n\n// This is a regular comment\nfunc main() {}",
			expected: 0,
		},
		{
			name:     "Multiple AI comments",
			content:  "// First comment AI?\n\n// Second comment AI!\n\nfunc main() {}",
			expected: 2,
			wantType: "?",
			wantContent: "First comment AI?",
			wantLine: 1,
		},
		{
			name:     "AI marker in middle of comment",
			content:  "// This AI? comment has marker in middle\nfunc main() {}",
			expected: 0,
		},
		{
			name:     "Indented comment",
			content:  "package main\n\nfunc main() {\n    // Indented comment AI?\n}",
			expected: 1,
			wantType: "?",
			wantContent: "Indented comment AI?",
			wantLine: 4,
		},
		{
			name:     "Empty file",
			content:  "",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comments, err := extractAICommentsFromString(tt.content, "test.go")
			if err != nil {
				t.Fatalf("extractAICommentsFromString() error = %v", err)
			}

			if len(comments) != tt.expected {
				t.Errorf("Expected %d comments, got %d", tt.expected, len(comments))
				return
			}

			if tt.expected > 0 {
				comment := comments[0]
				if comment.ActionType != tt.wantType {
					t.Errorf("Expected ActionType %s, got %s", tt.wantType, comment.ActionType)
				}
				if comment.Content != tt.wantContent {
					t.Errorf("Expected Content %s, got %s", tt.wantContent, comment.Content)
				}
				if comment.LineNumber != tt.wantLine {
					t.Errorf("Expected LineNumber %d, got %d", tt.wantLine, comment.LineNumber)
				}
			}
		})
	}
}

func TestJavaScriptSingleLineComments(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
		wantType string
	}{
		{
			name:     "JS comment with AI?",
			content:  "console.log('hello');\n// This needs improvement AI?\nfunction test() {}",
			expected: 1,
			wantType: "?",
		},
		{
			name:     "JS comment with AI!",
			content:  "// Refactor this function AI!\nfunction test() {}",
			expected: 1,
			wantType: "!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comments, err := extractAICommentsFromString(tt.content, "test.js")
			if err != nil {
				t.Fatalf("extractAICommentsFromString() error = %v", err)
			}

			if len(comments) != tt.expected {
				t.Errorf("Expected %d comments, got %d", tt.expected, len(comments))
				return
			}

			if tt.expected > 0 {
				comment := comments[0]
				if comment.ActionType != tt.wantType {
					t.Errorf("Expected ActionType %s, got %s", tt.wantType, comment.ActionType)
				}
			}
		})
	}
}

func TestMultilineComments(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
		wantType string
		wantContent string
	}{
		{
			name: "Go multiline comment with AI?",
			content: `package main

/*
 * This is a multiline comment
 * that needs clarification AI?
 */
func main() {}`,
			expected: 1,
			wantType: "?",
			wantContent: "This is a multiline comment that needs clarification AI?",
		},
		{
			name: "JS multiline comment with AI!",
			content: `console.log('test');

/*
 * TODO: Fix this implementation AI!
 * It has performance issues
 */
function test() {}`,
			expected: 1,
			wantType: "!",
			wantContent: "TODO: Fix this implementation AI! It has performance issues",
		},
		{
			name: "Multiline without AI marker",
			content: `/*
 * Regular multiline comment
 * No AI marker here
 */`,
			expected: 0,
		},
		{
			name: "Single line multiline comment",
			content: "/* Quick comment AI? */",
			expected: 1,
			wantType: "?",
			wantContent: "Quick comment AI?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := ".go"
			if strings.Contains(tt.name, "JS") {
				ext = ".js"
			}
			
			comments, err := extractAICommentsFromString(tt.content, "test"+ext)
			if err != nil {
				t.Fatalf("extractAICommentsFromString() error = %v", err)
			}

			if len(comments) != tt.expected {
				t.Errorf("Expected %d comments, got %d", tt.expected, len(comments))
				return
			}

			if tt.expected > 0 {
				comment := comments[0]
				if comment.ActionType != tt.wantType {
					t.Errorf("Expected ActionType %s, got %s", tt.wantType, comment.ActionType)
				}
				if comment.Content != tt.wantContent {
					t.Errorf("Expected Content %q, got %q", tt.wantContent, comment.Content)
				}
			}
		})
	}
}

func TestContextExtraction(t *testing.T) {
	content := `line 1
line 2
line 3
// This comment needs attention AI?
line 5
line 6
line 7`

	comments, err := extractAICommentsFromString(content, "test.go")
	if err != nil {
		t.Fatalf("extractAICommentsFromString() error = %v", err)
	}

	if len(comments) != 1 {
		t.Fatalf("Expected 1 comment, got %d", len(comments))
	}

	comment := comments[0]
	
	// Should have context lines
	if len(comment.ContextLines) == 0 {
		t.Errorf("Expected context lines, got none")
	}

	// Check that target line is marked with >
	found := false
	for _, line := range comment.ContextLines {
		if strings.Contains(line, "> 4:") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Target line not marked in context")
	}

	// Check that context includes surrounding lines
	contextText := strings.Join(comment.ContextLines, "\n")
	if !strings.Contains(contextText, "line 1") || !strings.Contains(contextText, "line 7") {
		t.Errorf("Context doesn't include expected surrounding lines")
	}
}

func TestHashGeneration(t *testing.T) {
	comment1 := AIComment{
		FilePath:   "test.go",
		LineNumber: 5,
		Content:    "Test comment",
		ActionType: "?",
	}
	comment1.Hash = generateCommentHash(comment1)

	comment2 := AIComment{
		FilePath:   "test.go",
		LineNumber: 5,
		Content:    "Test comment",
		ActionType: "?",
	}
	comment2.Hash = generateCommentHash(comment2)

	// Same comments should have same hash
	if comment1.Hash != comment2.Hash {
		t.Errorf("Same comments should have same hash: %s != %s", comment1.Hash, comment2.Hash)
	}

	// Different comments should have different hashes
	comment3 := comment1
	comment3.Content = "Different content"
	comment3.Hash = generateCommentHash(comment3)

	if comment1.Hash == comment3.Hash {
		t.Errorf("Different comments should have different hashes")
	}
}

func TestCaching(t *testing.T) {
	// Clear cache before test
	clearProcessedCache()

	comment := AIComment{
		FilePath:   "test.go",
		LineNumber: 5,
		Content:    "Test comment",
		ActionType: "?",
	}
	comment.Hash = generateCommentHash(comment)

	// Should not be processed initially
	if isCommentProcessed(comment) {
		t.Errorf("Comment should not be processed initially")
	}

	// Mark as processed
	markCommentProcessed(comment)

	// Should now be processed
	if !isCommentProcessed(comment) {
		t.Errorf("Comment should be processed after marking")
	}

	// Clear cache
	clearProcessedCache()

	// Should not be processed after clearing
	if isCommentProcessed(comment) {
		t.Errorf("Comment should not be processed after clearing cache")
	}
}

func TestUnsupportedFileExtensions(t *testing.T) {
	content := "// This is a comment AI?"
	
	comments, err := extractAICommentsFromString(content, "test.txt")
	if err != nil {
		t.Fatalf("extractAICommentsFromString() error = %v", err)
	}

	if len(comments) != 0 {
		t.Errorf("Expected 0 comments for unsupported extension, got %d", len(comments))
	}
}

func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{
			name:     "AI marker at start of comment",
			content:  "// AI? What should this do?",
			expected: 1,
		},
		{
			name:     "Multiple AI markers in same comment",
			content:  "// This AI? comment also has AI! marker",
			expected: 0, // Should skip - markers are in middle
		},
		{
			name:     "AI marker without space",
			content:  "// CommentAI?",
			expected: 1,
		},
		{
			name:     "Case sensitivity",
			content:  "// Comment ai?",
			expected: 0, // Should be case sensitive
		},
		{
			name:     "AI marker in string literal",
			content:  `fmt.Println("This AI? is in a string")`,
			expected: 0, // Should not match strings
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comments, err := extractAICommentsFromString(tt.content, "test.go")
			if err != nil {
				t.Fatalf("extractAICommentsFromString() error = %v", err)
			}

			if len(comments) != tt.expected {
				t.Errorf("Expected %d comments, got %d", tt.expected, len(comments))
			}
		})
	}
}

func TestRenderCommentPrompt(t *testing.T) {
	tests := []struct {
		name     string
		comment  AIComment
		expected string
	}{
		{
			name: "single line question",
			comment: AIComment{
				FilePath:   "test.go",
				LineNumber: 5,
				ActionType: "?",
			},
			expected: "See test.go at line 5. Summarise the question and answer it. DO NOT MAKE CHANGES.",
		},
		{
			name: "single line command",
			comment: AIComment{
				FilePath:   "test.go",
				LineNumber: 10,
				ActionType: "!",
			},
			expected: "See test.go at line 10. Summarise the ask, and make the appropriate changes",
		},
		{
			name: "multiline question - should show range",
			comment: AIComment{
				FilePath:   "test.go",
				LineNumber: 15,
				EndLine:    17, // This field doesn't exist yet
				ActionType: "?",
			},
			expected: "See test.go at lines 15-17. Summarise the question and answer it. DO NOT MAKE CHANGES.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderCommentPrompt(tt.comment)
			if result != tt.expected {
				t.Errorf("renderCommentPrompt() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMultilineCommentLineRanges(t *testing.T) {
	content := `package main

/*
 * This is a multiline comment
 * that spans several lines
 * and has a question AI?
 */

func main() {
	// single line comment
}`

	comments, err := extractAICommentsFromString(content, "test.go")
	if err != nil {
		t.Fatalf("extractAICommentsFromString() error = %v", err)
	}

	if len(comments) != 1 {
		t.Fatalf("Expected 1 comment, got %d", len(comments))
	}

	comment := comments[0]
	if comment.LineNumber != 3 {
		t.Errorf("Expected LineNumber = 3, got %d", comment.LineNumber)
	}
	if comment.EndLine != 7 {
		t.Errorf("Expected EndLine = 7, got %d", comment.EndLine)
	}

	// Test the rendered prompt
	prompt := renderCommentPrompt(comment)
	expected := "See test.go at lines 3-7. Summarise the question and answer it. DO NOT MAKE CHANGES."
	if prompt != expected {
		t.Errorf("renderCommentPrompt() = %q, want %q", prompt, expected)
	}
}