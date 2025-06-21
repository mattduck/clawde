package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// NO_CLAWDE - This test file contains AI marker examples and should be excluded from comment detection

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
			name:     "Go comment with AI:",
			content:  "package main\n\n// AI: there's the placeholder\nfunc main() {}",
			expected: 1,
			wantType: ":",
			wantContent: "AI: there's the placeholder",
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
		{
			name:     "JS comment with AI:",
			content:  "// AI: check this logic\nfunction test() {}",
			expected: 1,
			wantType: ":",
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

func TestPythonSingleLineComments(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
		wantType string
	}{
		{
			name:     "Python comment with AI?",
			content:  "print('hello')\n# This needs improvement AI?\ndef test():\n    pass",
			expected: 1,
			wantType: "?",
		},
		{
			name:     "Python comment with AI!",
			content:  "# Refactor this function AI!\ndef test():\n    pass",
			expected: 1,
			wantType: "!",
		},
		{
			name:     "Python comment with AI:",
			content:  "# AI: there's the placeholder\ndef hello_world():\n    print(\"Hello, World!\")",
			expected: 1,
			wantType: ":",
		},
		{
			name:     "Python comment without AI marker",
			content:  "# This is a regular comment\ndef test():\n    pass",
			expected: 0,
		},
		{
			name:     "Python comment ending with word containing ai",
			content:  "# Visiting hawaii?",
			expected: 0, // Should not match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comments, err := extractAICommentsFromString(tt.content, "test.py")
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
		{
			name: "Multiline comment with AI:",
			content: `/*
 * AI: this function needs review
 * for performance optimizations
 */
function test() {}`,
			expected: 1,
			wantType: ":",
			wantContent: "AI: this function needs review for performance optimizations",
		},
		{
			name: "Single-line multiline comment with content",
			content: "/* This is a single-line multiline comment AI? */",
			expected: 1,
			wantType: "?",
			wantContent: "This is a single-line multiline comment AI?",
		},
		{
			name: "Python single-line triple quote with content",
			content: `"""This is a single-line docstring AI!"""`,
			expected: 1,
			wantType: "!",
			wantContent: "This is a single-line docstring AI!",
		},
		{
			name: "Empty multiline markers should not match",
			content: `/**/`,
			expected: 0,
		},
		{
			name: "Python empty triple quotes should not match",
			content: `""""""`,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := ".go"
			if strings.Contains(tt.name, "JS") {
				ext = ".js"
			}
			if strings.Contains(tt.name, "Python") {
				ext = ".py"
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
			expected: 0, // Should not match - "CommentAI?" is not an AI marker
		},
		{
			name:     "Case insensitivity",
			content:  "// Comment ai?",
			expected: 1, // Should match case insensitively
		},
		{
			name:     "AI marker in string literal",
			content:  `fmt.Println("This AI? is in a string")`,
			expected: 0, // Should not match strings
		},
		{
			name:     "Comment ending with word containing ai?",
			content:  "// Traveling to hawaii?",
			expected: 0, // Should not match - "hawaii?" is not an AI marker
		},
		{
			name:     "Comment ending with word containing ai!",
			content:  "// The brave samurai!",
			expected: 0, // Should not match - "samurai!" is not an AI marker
		},
		{
			name:     "AI: marker at start of comment",
			content:  "// AI: What should this function do?",
			expected: 1, // Should match
		},
		{
			name:     "Comment ending with word containing ai:",
			content:  "// Welcome to Hawaii:",
			expected: 0, // Should not match - "hawaii:" is not an AI marker
		},
		{
			name:     "Comment ending with AI:",
			content:  "// This comment ends with AI:",
			expected: 0, // Should not match - AI: only supported at start
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
			expected: "See test.go at line 5 and surrounding context. Answer the question(s), but DO NOT MAKE CHANGES. Replace the AI? marker with [ai] when done.",
		},
		{
			name: "single line command",
			comment: AIComment{
				FilePath:   "test.go",
				LineNumber: 10,
				ActionType: "!",
			},
			expected: "See test.go at line 10 and surrounding context. Make the appropriate changes. YOU MUST replace the AI! marker with [ai] when done.",
		},
		{
			name: "multiline question - should show range",
			comment: AIComment{
				FilePath:   "test.go",
				LineNumber: 15,
				EndLine:    17, // This field doesn't exist yet
				ActionType: "?",
			},
			expected: "See test.go at lines 15-17 and surrounding context. Answer the question(s), but DO NOT MAKE CHANGES. Replace the AI? marker with [ai] when done.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderCommentPrompt(tt.comment, nil)
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
	prompt := renderCommentPrompt(comment, nil)
	expected := "See test.go at lines 3-7 and surrounding context. Answer the question(s), but DO NOT MAKE CHANGES. Replace the AI? marker with [ai] when done."
	if prompt != expected {
		t.Errorf("renderCommentPrompt() = %q, want %q", prompt, expected)
	}
}

func TestConsecutiveSingleLineComments(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
		wantStartLine int
		wantEndLine   int
		wantContent   string
		wantType      string
	}{
		{
			name: "consecutive whole-line comments with AI?",
			content: `package main

// This is a long comment
// that spans multiple lines
// and should be grouped AI?

func main() {}`,
			expected: 1,
			wantStartLine: 3,
			wantEndLine: 5,
			wantContent: "This is a long comment that spans multiple lines and should be grouped AI?",
			wantType: "?",
		},
		{
			name: "consecutive whole-line comments with AI!",
			content: `package main

// Fix this function
// it has performance issues
// please optimize AI!

func main() {}`,
			expected: 1,
			wantStartLine: 3,
			wantEndLine: 5,
			wantContent: "Fix this function it has performance issues please optimize AI!",
			wantType: "!",
		},
		{
			name: "consecutive comments with AI marker on first line",
			content: `package main

// blah AI?
// continues here
// and here too

func main() {}`,
			expected: 1,
			wantStartLine: 3,
			wantEndLine: 5,
			wantContent: "blah AI? continues here and here too",
			wantType: "?",
		},
		{
			name: "mixed inline and whole-line comments - should not group",
			content: `package main

func test() { // inline comment AI?
// whole line comment
// another whole line comment AI!
}`,
			expected: 2, // Should find 2 separate comments
			wantStartLine: 3, // First comment (inline)
			wantEndLine: 0,   // Inline comment has EndLine = 0
			wantContent: "inline comment AI?",
			wantType: "?",
		},
		{
			name: "single whole-line comment - should not have EndLine",
			content: `package main

// Single comment AI?

func main() {}`,
			expected: 1,
			wantStartLine: 3,
			wantEndLine: 0, // Single line should have EndLine = 0
			wantContent: "Single comment AI?",
			wantType: "?",
		},
		{
			name: "consecutive comments with gap - should not group",
			content: `package main

// First comment AI?

// Second comment after gap AI!

func main() {}`,
			expected: 2, // Should find 2 separate comments
			wantStartLine: 3, // First comment
			wantEndLine: 0,   // Single line
			wantContent: "First comment AI?",
			wantType: "?",
		},
		{
			name: "consecutive comments without AI marker - should not match",
			content: `package main

// This is a comment
// without any AI markers
// just regular comments

func main() {}`,
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
				if comment.LineNumber != tt.wantStartLine {
					t.Errorf("Expected LineNumber = %d, got %d", tt.wantStartLine, comment.LineNumber)
				}
				if comment.EndLine != tt.wantEndLine {
					t.Errorf("Expected EndLine = %d, got %d", tt.wantEndLine, comment.EndLine)
				}
				if comment.Content != tt.wantContent {
					t.Errorf("Expected Content = %q, got %q", tt.wantContent, comment.Content)
				}
				if comment.ActionType != tt.wantType {
					t.Errorf("Expected ActionType = %q, got %q", tt.wantType, comment.ActionType)
				}

				// Test the rendered prompt for multi-line blocks
				if comment.EndLine > 0 {
					prompt := renderCommentPrompt(comment, nil)
					expectedPrompt := fmt.Sprintf("See test.go at lines %d-%d and surrounding context. Answer the question(s), but DO NOT MAKE CHANGES. Replace the AI? marker with [ai] when done.", comment.LineNumber, comment.EndLine)
					if tt.wantType == "!" {
						expectedPrompt = fmt.Sprintf("See test.go at lines %d-%d and surrounding context. Make the appropriate changes. YOU MUST replace the AI! marker with [ai] when done.", comment.LineNumber, comment.EndLine)
					}
					if prompt != expectedPrompt {
						t.Errorf("renderCommentPrompt() = %q, want %q", prompt, expectedPrompt)
					}
				}
			}
		})
	}
}

func TestCaseInsensitiveAIMarkers(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
		wantType string
	}{
		{
			name:     "lowercase ai?",
			content:  "// This is a test comment ai?",
			expected: 1,
			wantType: "?",
		},
		{
			name:     "lowercase ai!",
			content:  "// Fix this function ai!",
			expected: 1,
			wantType: "!",
		},
		{
			name:     "mixed case Ai?",
			content:  "// What should this do Ai?",
			expected: 1,
			wantType: "?",
		},
		{
			name:     "mixed case aI!",
			content:  "// Refactor this aI!",
			expected: 1,
			wantType: "!",
		},
		{
			name:     "uppercase AI?",
			content:  "// This needs clarification AI?",
			expected: 1,
			wantType: "?",
		},
		{
			name:     "uppercase AI!",
			content:  "// Optimize this AI!",
			expected: 1,
			wantType: "!",
		},
		{
			name:     "lowercase ai:",
			content:  "// ai: needs attention",
			expected: 1,
			wantType: ":",
		},
		{
			name:     "uppercase AI:",
			content:  "// AI: check implementation",
			expected: 1,
			wantType: ":",
		},
		{
			name:     "multiline with mixed case",
			content: `/*
 * This is a multiline comment
 * that needs review ai?
 */`,
			expected: 1,
			wantType: "?",
		},
		{
			name: "consecutive comments with different cases",
			content: `// First comment ai?
// Second comment AI!
// Third comment Ai?`,
			expected: 1, // Should be grouped into one comment
			wantType: "?", // First marker wins
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
					t.Errorf("Expected ActionType = %q, got %q", tt.wantType, comment.ActionType)
				}
			}
		})
	}
}

func TestInlineVsWholeLineComments(t *testing.T) {
	content := `package main

func test() {
    x := 1 // inline comment AI?
    // whole line comment starts here
    // and continues here
    // ending with marker AI!
    y := 2 // another inline AI?
}`

	comments, err := extractAICommentsFromString(content, "test.go")
	if err != nil {
		t.Fatalf("extractAICommentsFromString() error = %v", err)
	}

	if len(comments) != 3 {
		t.Fatalf("Expected 3 comments, got %d", len(comments))
	}

	// First comment should be inline
	comment1 := comments[0]
	if comment1.LineNumber != 4 || comment1.EndLine != 0 {
		t.Errorf("First comment should be inline at line 4, got line %d with EndLine %d", comment1.LineNumber, comment1.EndLine)
	}
	if comment1.ActionType != "?" {
		t.Errorf("First comment should be type '?', got %q", comment1.ActionType)
	}

	// Second comment should be a multi-line block
	comment2 := comments[1]
	if comment2.LineNumber != 5 || comment2.EndLine != 7 {
		t.Errorf("Second comment should be multi-line from 5-7, got %d-%d", comment2.LineNumber, comment2.EndLine)
	}
	if comment2.ActionType != "!" {
		t.Errorf("Second comment should be type '!', got %q", comment2.ActionType)
	}

	// Third comment should be inline
	comment3 := comments[2]
	if comment3.LineNumber != 8 || comment3.EndLine != 0 {
		t.Errorf("Third comment should be inline at line 8, got line %d with EndLine %d", comment3.LineNumber, comment3.EndLine)
	}
	if comment3.ActionType != "?" {
		t.Errorf("Third comment should be type '?', got %q", comment3.ActionType)
	}
}

func TestAIMarkersWithinMultilineComments(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		expected     int
		wantTypes    []string
		wantContents []string
	}{
		{
			name: "AI? at end of line within multiline comment",
			content: `/*
 * This is a multiline comment
 * What should this function do AI?
 * More content here
 */`,
			expected:     1,
			wantTypes:    []string{"?"},
			wantContents: []string{"This is a multiline comment What should this function do AI? More content here"},
		},
		{
			name: "AI! at end of line within multiline comment",
			content: `/*
 * TODO: Fix this implementation AI!
 * It has performance issues
 * and needs refactoring
 */`,
			expected:     1,
			wantTypes:    []string{"!"},
			wantContents: []string{"TODO: Fix this implementation AI! It has performance issues and needs refactoring"},
		},
		{
			name: "AI: at start of line within multiline comment",
			content: `/*
 * This function needs review
 * AI: Check for thread safety
 * and error handling
 */`,
			expected:     1,
			wantTypes:    []string{":"},
			wantContents: []string{"This function needs review AI: Check for thread safety and error handling"},
		},
		{
			name: "Multiple AI markers on different lines",
			content: `/*
 * AI: This needs attention
 * What about error handling AI?
 * Fix the performance issues AI!
 */`,
			expected:     1,
			wantTypes:    []string{"!"},  // AI! takes precedence
			wantContents: []string{"AI: This needs attention What about error handling AI? Fix the performance issues AI!"},
		},
		{
			name: "AI marker in middle of line (should not match)",
			content: `/*
 * This comment has AI? in the middle
 * and should not be detected
 */`,
			expected: 0,
		},
		{
			name: "Python multiline with AI markers",
			content: `"""
This is a docstring
What does this function do AI?
More documentation here
"""`,
			expected:     1,
			wantTypes:    []string{"?"},
			wantContents: []string{"This is a docstring What does this function do AI? More documentation here"},
		},
		{
			name: "Consecutive single-line comments with AI marker on separate line",
			content: `// This is a long comment that
// spans multiple lines and asks
// a question about the code
// AI?`,
			expected:     1,
			wantTypes:    []string{"?"},
			wantContents: []string{"This is a long comment that spans multiple lines and asks a question about the code AI?"},
		},
		{
			name: "AI marker at start of its own line in multiline",
			content: `/*
 * This function does something
 * AI!
 * Make it better
 */`,
			expected:     1,
			wantTypes:    []string{"!"},
			wantContents: []string{"This function does something AI! Make it better"},
		},
		{
			name: "Mixed single and multiline with line markers",
			content: `// First part of comment
// What should happen here AI?
/*
 * Another comment block
 * Fix this implementation AI!
 */`,
			expected: 2,
			wantTypes: []string{"?", "!"},
			wantContents: []string{
				"First part of comment What should happen here AI?",
				"Another comment block Fix this implementation AI!",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := ".go"
			if strings.Contains(tt.content, `"""`) {
				ext = ".py"
			}
			
			comments, err := extractAICommentsFromString(tt.content, "test"+ext)
			if err != nil {
				t.Fatalf("extractAICommentsFromString() error = %v", err)
			}

			if len(comments) != tt.expected {
				t.Errorf("Expected %d comments, got %d", tt.expected, len(comments))
				for i, c := range comments {
					t.Errorf("  Comment %d: %q (type: %s)", i, c.Content, c.ActionType)
				}
				return
			}

			for i, comment := range comments {
				if i < len(tt.wantTypes) && comment.ActionType != tt.wantTypes[i] {
					t.Errorf("Comment %d: Expected ActionType %q, got %q", i, tt.wantTypes[i], comment.ActionType)
				}
				if i < len(tt.wantContents) && comment.Content != tt.wantContents[i] {
					t.Errorf("Comment %d: Expected Content %q, got %q", i, tt.wantContents[i], comment.Content)
				}
			}
		})
	}
}

func TestMixedAIMarkers(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
		wantTypes []string
		wantContents []string
	}{
		{
			name:     "Comment with AI: and AI!",
			content:  "// AI: This function needs optimization AI!",
			expected: 1,
			wantTypes: []string{"!"},
			wantContents: []string{"AI: This function needs optimization AI!"},
		},
		{
			name:     "Comment with AI: and AI?",
			content:  "// AI: What about error handling here AI?",
			expected: 1,
			wantTypes: []string{"?"},
			wantContents: []string{"AI: What about error handling here AI?"},
		},
		{
			name: "Multi-line comment with mixed markers",
			content: `package main

// AI: This block needs review
// Consider performance optimization
// and error handling AI!

func test() {}`,
			expected: 1,
			wantTypes: []string{"!"},
			wantContents: []string{"AI: This block needs review Consider performance optimization and error handling AI!"},
		},
		{
			name: "Consecutive comments: AI: first line, AI! on middle line",
			content: `// AI: some context
// Fix this please AI!
// More details here`,
			expected: 1,
			wantTypes: []string{"!"},
			wantContents: []string{"AI: some context Fix this please AI! More details here"},
		},
		{
			name: "Multiline block comment with mixed markers",
			content: `/*
 * AI: Check this implementation
 * for thread safety issues AI?
 */`,
			expected: 1,
			wantTypes: []string{"?"},
			wantContents: []string{"AI: Check this implementation for thread safety issues AI?"},
		},
		{
			name:     "AI: in middle with AI! at end",
			content:  "// This comment AI: has markers in various places AI!",
			expected: 1,
			wantTypes: []string{"!"},
			wantContents: []string{"This comment AI: has markers in various places AI!"},
		},
		{
			name:     "Comment ending with AI: (should not match)",
			content:  "// This comment ends with AI:",
			expected: 0,
		},
		{
			name:     "Multiple AI: markers with AI?",
			content:  "// AI: First marker AI: Second marker AI?",
			expected: 1,
			wantTypes: []string{"?"},
			wantContents: []string{"AI: First marker AI: Second marker AI?"},
		},
		{
			name: "Separate comments with different markers",
			content: `// AI: This is the first comment
// This is a separate comment AI?`,
			expected: 1, // Should be grouped together
			wantTypes: []string{"?"},
			wantContents: []string{"AI: This is the first comment This is a separate comment AI?"},
		},
		{
			name:     "Only AI: marker (no ? or !)",
			content:  "// AI: This only has colon marker",
			expected: 1,
			wantTypes: []string{":"},
			wantContents: []string{"AI: This only has colon marker"},
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

			for i, comment := range comments {
				if i < len(tt.wantTypes) && comment.ActionType != tt.wantTypes[i] {
					t.Errorf("Comment %d: Expected ActionType %q, got %q", i, tt.wantTypes[i], comment.ActionType)
				}
				if i < len(tt.wantContents) && comment.Content != tt.wantContents[i] {
					t.Errorf("Comment %d: Expected Content %q, got %q", i, tt.wantContents[i], comment.Content)
				}
			}
		})
	}
}