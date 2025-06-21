package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestLongCommentsWithAIMarkers(t *testing.T) {
	// Create a comment longer than maxCommentLength (1000 characters)
	longPrefix := strings.Repeat("This is a very long comment that exceeds the maximum comment length. ", 15) // ~1050 characters
	
	tests := []struct {
		name        string
		content     string
		expected    int
		wantType    string
		shouldTruncate bool
	}{
		{
			name:        "Long single-line comment ending with AI?",
			content:     fmt.Sprintf("// %s AI?", longPrefix),
			expected:    1,
			wantType:    "?",
			shouldTruncate: true,
		},
		{
			name:        "Long single-line comment ending with AI!",
			content:     fmt.Sprintf("// %s AI!", longPrefix),
			expected:    1,
			wantType:    "!",
			shouldTruncate: true,
		},
		{
			name: "Long multiline comment ending with AI?",
			content: fmt.Sprintf(`/*
 * %s
 * AI?
 */`, longPrefix),
			expected:    1,
			wantType:    "?",
			shouldTruncate: true,
		},
		{
			name: "Long multiline comment ending with AI!",
			content: fmt.Sprintf(`/*
 * %s
 * AI!
 */`, longPrefix),
			expected:    1,
			wantType:    "!",
			shouldTruncate: true,
		},
		{
			name: "Long consecutive single-line comments ending with AI?",
			content: fmt.Sprintf(`// %s
// More content here
// Even more content
// AI?`, longPrefix),
			expected:    1,
			wantType:    "?",
			shouldTruncate: true,
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
				
				// Verify ActionType is detected correctly despite long content
				if comment.ActionType != tt.wantType {
					t.Errorf("Expected ActionType %s, got %s", tt.wantType, comment.ActionType)
				}

				// Verify content is truncated if expected
				if tt.shouldTruncate {
					if !strings.Contains(comment.Content, "...(truncated)") {
						t.Errorf("Expected content to be truncated, but got: %s", comment.Content)
					}
					if len(comment.Content) > 1020 { // maxCommentLength + some buffer for truncation text
						t.Errorf("Content should be truncated but length is %d", len(comment.Content))
					}
				}

				// Verify FullLine contains the complete original comment (not truncated)
				if strings.Contains(comment.FullLine, "...(truncated)") {
					t.Errorf("FullLine should not be truncated, but got: %s", comment.FullLine)
				}

				// Verify FullLine contains the AI marker
				lowerFullLine := strings.ToLower(comment.FullLine)
				expectedMarker := strings.ToLower(fmt.Sprintf("ai%s", tt.wantType))
				if !strings.Contains(lowerFullLine, expectedMarker) {
					t.Errorf("FullLine should contain AI marker %s, but got: %s", expectedMarker, comment.FullLine)
				}
			}
		})
	}
}