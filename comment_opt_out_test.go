package main

// NO_CLAWDE - This test file contains AI marker examples and should be excluded from comment detection

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestOptOutFunctionality(t *testing.T) {
	// Initialize logger for tests
	if logger == nil {
		handler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
		logger = slog.New(handler)
	}
	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{
			name: "Single-line comment with NO_CLAWDE should opt out",
			content: `package main

// NO_CLAWDE - This file should be excluded

// This comment has markers AI!
func main() {}`,
			expected: 0, // Should return no comments due to opt-out
		},
		{
			name: "Multiline comment with NO_CLAWDE should opt out",
			content: `package main

/*
 * NO_CLAWDE - This file should be excluded
 * from comment processing
 */

// This comment has AI! markers
func main() {}`,
			expected: 0, // Should return no comments due to opt-out
		},
		{
			name: "Case insensitive NO_CLAWDE should opt out",
			content: `package main

// no_clawde - lowercase should work too

// This comment has AI! markers
func main() {}`,
			expected: 0, // Should return no comments due to opt-out
		},
		{
			name: "Mixed case NO_CLAWDE should opt out",
			content: `package main

// No_Clawde - mixed case should work

// This comment has AI! markers
func main() {}`,
			expected: 0, // Should return no comments due to opt-out
		},
		{
			name: "NO_CLAWDE in Python comment should opt out",
			content: `#!/usr/bin/env python3

# NO_CLAWDE - Python file opt-out

# This comment has AI! markers
def main():
    pass`,
			expected: 0, // Should return no comments due to opt-out
		},
		{
			name: "NO_CLAWDE in Python docstring should opt out",
			content: `#!/usr/bin/env python3

"""
NO_CLAWDE - Python docstring opt-out
This file should be excluded
"""

# This comment has AI! markers
def main():
    pass`,
			expected: 0, // Should return no comments due to opt-out
		},
		{
			name: "NO_CLAWDE in JavaScript comment should opt out",
			content: `// NO_CLAWDE - JavaScript file opt-out

// This comment has AI! markers
function main() {}`,
			expected: 0, // Should return no comments due to opt-out
		},
		{
			name: "NO_CLAWDE in JavaScript multiline comment should opt out",
			content: `/*
 * NO_CLAWDE - JavaScript multiline opt-out
 */

// This comment has AI! markers
function main() {}`,
			expected: 0, // Should return no comments due to opt-out
		},
		{
			name: "File without NO_CLAWDE should process normally",
			content: `package main

// This comment has markers AI!
func main() {}`,
			expected: 1, // Should find the AI comment normally
		},
		{
			name: "NO_CLAWDE later in file should still opt out entire file",
			content: `package main

// This comment has AI! markers
func main() {}

// NO_CLAWDE - Even later in file should opt out entire file`,
			expected: 0, // Should return no comments due to opt-out
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := ".go"
			if strings.Contains(tt.content, "#!/usr/bin/env python3") || strings.Contains(tt.content, "def main()") {
				ext = ".py"
			}
			if strings.Contains(tt.content, "function main()") {
				ext = ".js"
			}

			// Create a temporary file to test actual file-based processing
			tmpFile, err := os.CreateTemp("", "test_optout_*"+ext)
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())
			defer tmpFile.Close()

			// Write test content to file
			if _, err := tmpFile.WriteString(tt.content); err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			tmpFile.Close() // Close before reading

			// Use actual ExtractAIComments function that includes opt-out check
			comments, err := ExtractAIComments(tmpFile.Name())
			if err != nil {
				t.Fatalf("ExtractAIComments() error = %v", err)
			}

			if len(comments) != tt.expected {
				t.Errorf("Expected %d comments, got %d", tt.expected, len(comments))
				for i, c := range comments {
					t.Errorf("  Comment %d: %q (type: %s)", i, c.Content, c.ActionType)
				}
			}
		})
	}
}
