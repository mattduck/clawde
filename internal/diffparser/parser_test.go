package diffparser

import (
	"strings"
	"testing"
)

func TestParseBasicDiff(t *testing.T) {
	input := `⏺ Update(/path/to/file.go)
  ⎿  Added 1 line, removed 1 line
      10      func hello() {
      11 -        return "hello"
      11 +        return "goodbye"
      12      }
`

	diffs := Parse(input)

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	d := diffs[0]
	if d.Path != "/path/to/file.go" {
		t.Errorf("expected path /path/to/file.go, got %s", d.Path)
	}

	if len(d.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(d.Hunks))
	}

	h := d.Hunks[0]
	if h.StartLine != 10 {
		t.Errorf("expected start line 10, got %d", h.StartLine)
	}

	if len(h.Lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(h.Lines))
	}

	// Check line types
	expected := []struct {
		lineType LineType
		content  string
	}{
		{LineContext, "    func hello() {"},
		{LineDelete, "        return \"hello\""},
		{LineAdd, "        return \"goodbye\""},
		{LineContext, "    }"},
	}

	for i, exp := range expected {
		if h.Lines[i].Type != exp.lineType {
			t.Errorf("line %d: expected type %v, got %v", i, exp.lineType, h.Lines[i].Type)
		}
		if h.Lines[i].Content != exp.content {
			t.Errorf("line %d: expected content %q, got %q", i, exp.content, h.Lines[i].Content)
		}
	}
}

func TestParseContentStartingWithDash(t *testing.T) {
	// Content that starts with "- " (like markdown lists) should not be treated as deletions
	input := `⏺ Update(/path/to/todo.md)
  ⎿  Added 1 line, removed 1 line
      5
      6  - [x] Completed task
      7
      8 -- [ ] Incomplete task
      8 +- [x] Incomplete task
      9    - Subtask one
      10    - Subtask two
`

	diffs := Parse(input)

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	h := diffs[0].Hunks[0]

	// Find the actual change lines
	var deletions, additions int
	for _, line := range h.Lines {
		switch line.Type {
		case LineDelete:
			deletions++
			if line.Content != "- [ ] Incomplete task" {
				t.Errorf("unexpected deletion content: %q", line.Content)
			}
		case LineAdd:
			additions++
			if line.Content != "- [x] Incomplete task" {
				t.Errorf("unexpected addition content: %q", line.Content)
			}
		case LineContext:
			// Context lines starting with "- " should be preserved
			if strings.HasPrefix(line.Content, "- [x]") || strings.HasPrefix(line.Content, "- Subtask") {
				// These are correct context lines
			}
		}
	}

	if deletions != 1 {
		t.Errorf("expected 1 deletion, got %d", deletions)
	}
	if additions != 1 {
		t.Errorf("expected 1 addition, got %d", additions)
	}
}

func TestParseEmptyLines(t *testing.T) {
	input := `⏺ Update(/path/to/file.py)
  ⎿  Added 1 line
      10      def foo():
      11
      12 +        # new comment
      13          return True
`

	diffs := Parse(input)

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	h := diffs[0].Hunks[0]

	// Check that empty line is captured as context
	foundEmpty := false
	for _, line := range h.Lines {
		if line.Type == LineContext && line.Content == "" {
			foundEmpty = true
			break
		}
	}

	if !foundEmpty {
		t.Error("expected to find empty context line")
	}
}

func TestParsePreservesIndentation(t *testing.T) {
	input := `⏺ Update(/path/to/file.py)
  ⎿  Changed 1 line
      50          if condition:
      51 -            old_value = compute()
      51 +            new_value = compute()
      52              return result
`

	diffs := Parse(input)

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	h := diffs[0].Hunks[0]

	// Check that indentation is preserved
	for _, line := range h.Lines {
		if line.Type == LineDelete && !strings.HasPrefix(line.Content, "            ") {
			t.Errorf("deletion should preserve indentation, got: %q", line.Content)
		}
		if line.Type == LineAdd && !strings.HasPrefix(line.Content, "            ") {
			t.Errorf("addition should preserve indentation, got: %q", line.Content)
		}
	}
}

func TestParseMultipleFiles(t *testing.T) {
	input := `⏺ Update(/path/to/first.go)
  ⎿  Changed 1 line
      10      func a() {
      11 -        old
      11 +        new
      12      }

⏺ Update(/path/to/second.go)
  ⎿  Changed 1 line
      20      func b() {
      21 -        foo
      21 +        bar
      22      }
`

	diffs := Parse(input)

	if len(diffs) != 2 {
		t.Fatalf("expected 2 diffs, got %d", len(diffs))
	}

	if diffs[0].Path != "/path/to/first.go" {
		t.Errorf("expected first path /path/to/first.go, got %s", diffs[0].Path)
	}
	if diffs[1].Path != "/path/to/second.go" {
		t.Errorf("expected second path /path/to/second.go, got %s", diffs[1].Path)
	}
}

func TestParseAdditionsOnly(t *testing.T) {
	input := `⏺ Update(/path/to/file.go)
  ⎿  Added 3 lines
      10      func process() {
      11          validate()
      12 +        log.Info("starting")
      13 +        metrics.Inc()
      14 +        trace.Start()
      15          execute()
      16      }
`

	diffs := Parse(input)

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	h := diffs[0].Hunks[0]

	var additions int
	for _, line := range h.Lines {
		if line.Type == LineAdd {
			additions++
		}
	}

	if additions != 3 {
		t.Errorf("expected 3 additions, got %d", additions)
	}
}

func TestParseDeletionsOnly(t *testing.T) {
	input := `⏺ Update(/path/to/file.go)
  ⎿  Removed 2 lines
      10      func cleanup() {
      11 -        debug.Log("removing")
      12 -        debug.Trace()
      13          doCleanup()
      14      }
`

	diffs := Parse(input)

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	h := diffs[0].Hunks[0]

	var deletions int
	for _, line := range h.Lines {
		if line.Type == LineDelete {
			deletions++
		}
	}

	if deletions != 2 {
		t.Errorf("expected 2 deletions, got %d", deletions)
	}
}

func TestParseStopsAtThinkingMarker(t *testing.T) {
	input := `⏺ Update(/path/to/file.go)
  ⎿  Changed 1 line
      10 -    old
      10 +    new

∴ Thinking…

Some other content that should be ignored
`

	diffs := Parse(input)

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	// Should only have lines from before the thinking marker
	h := diffs[0].Hunks[0]
	if len(h.Lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(h.Lines))
	}
}

func TestToUnifiedFormat(t *testing.T) {
	input := `⏺ Update(/path/to/file.go)
  ⎿  Changed 1 line
      10      context
      11 -    old
      11 +    new
      12      more context
`

	diffs := Parse(input)
	unified := diffs[0].ToUnified()

	// Check header
	if !strings.Contains(unified, "--- a/path/to/file.go") {
		t.Error("unified diff should contain --- a/ header")
	}
	if !strings.Contains(unified, "+++ b/path/to/file.go") {
		t.Error("unified diff should contain +++ b/ header")
	}

	// Check hunk header
	if !strings.Contains(unified, "@@ -10 +10 @@") {
		t.Error("unified diff should contain hunk header")
	}

	// Check line prefixes
	lines := strings.Split(unified, "\n")
	var hasContext, hasAdd, hasDelete bool
	for _, line := range lines {
		if strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "---") && !strings.HasPrefix(line, "+++") {
			hasContext = true
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			hasAdd = true
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			hasDelete = true
		}
	}

	if !hasContext {
		t.Error("unified diff should have context lines (space prefix)")
	}
	if !hasAdd {
		t.Error("unified diff should have addition lines (+ prefix)")
	}
	if !hasDelete {
		t.Error("unified diff should have deletion lines (- prefix)")
	}
}

func TestToUnifiedAbsolutePath(t *testing.T) {
	input := `⏺ Update(/absolute/path/to/file.go)
  ⎿  Changed 1 line
      10 -    old
      10 +    new
`

	diffs := Parse(input)
	unified := diffs[0].ToUnified()

	// Absolute paths should get a/ and b/ prefix correctly
	if !strings.Contains(unified, "--- a/absolute/path/to/file.go") {
		t.Errorf("expected --- a/absolute/path/to/file.go in:\n%s", unified)
	}
}

func TestParseNoDiffs(t *testing.T) {
	input := `Some random output
that doesn't contain any diffs
just regular text
`

	diffs := Parse(input)

	if len(diffs) != 0 {
		t.Errorf("expected 0 diffs, got %d", len(diffs))
	}
}

func TestParseRelativePath(t *testing.T) {
	input := `⏺ Update(src/main.go)
  ⎿  Changed 1 line
      10 -    old
      10 +    new
`

	diffs := Parse(input)

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	if diffs[0].Path != "src/main.go" {
		t.Errorf("expected path src/main.go, got %s", diffs[0].Path)
	}

	unified := diffs[0].ToUnified()
	if !strings.Contains(unified, "--- a/src/main.go") {
		t.Errorf("expected --- a/src/main.go in:\n%s", unified)
	}
}

func TestParseWriteFormat(t *testing.T) {
	// Write format uses ⏺ Write(...) instead of ⏺ Update(...)
	// and has decorative header/footer lines
	input := `⏺ Write(/path/to/main.go)

────────────────────────────────────────────────────────────────────────────────
 Overwrite file main.go
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌
   1  package main
   2
   3  import (
   4 +  "crypto/sha256"
   5 +  "encoding/hex"
   6    "flag"
   7    "fmt"
 ...
  64 -  // Capture pane content
  64 +  if *watchFlag {
  65 +    runWatchMode(targetPane, *intervalFlag, *noPagerFlag)
  66 +    return
  67 +  }
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌
 Do you want to overwrite main.go?
`

	diffs := Parse(input)

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	d := diffs[0]
	if d.Path != "/path/to/main.go" {
		t.Errorf("expected path /path/to/main.go, got %s", d.Path)
	}

	// Has a "..." break so should be 2 hunks
	if len(d.Hunks) != 2 {
		t.Fatalf("expected 2 hunks (separated by ...), got %d", len(d.Hunks))
	}

	// Count additions and deletions across all hunks
	var additions, deletions int
	for _, h := range d.Hunks {
		for _, line := range h.Lines {
			switch line.Type {
			case LineAdd:
				additions++
			case LineDelete:
				deletions++
			}
		}
	}

	// Should have 6 additions and 1 deletion total
	if additions != 6 {
		t.Errorf("expected 6 additions, got %d", additions)
	}
	if deletions != 1 {
		t.Errorf("expected 1 deletion, got %d", deletions)
	}
}

func TestParseUpdateWithBreaks(t *testing.T) {
	// Update format with " ..." breaks should create multiple hunks
	input := `⏺ Update(/path/to/file.go)
  ⎿  Changed multiple lines
      10      func hello() {
      11 -        return "hello"
      11 +        return "goodbye"
      12      }
 ...
      50      func world() {
      51 -        return "world"
      51 +        return "planet"
      52      }
`

	diffs := Parse(input)

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	d := diffs[0]
	if len(d.Hunks) != 2 {
		t.Fatalf("expected 2 hunks (separated by ...), got %d", len(d.Hunks))
	}

	// First hunk should start at line 10
	if d.Hunks[0].StartLine != 10 {
		t.Errorf("expected first hunk to start at line 10, got %d", d.Hunks[0].StartLine)
	}

	// Second hunk should start at line 50
	if d.Hunks[1].StartLine != 50 {
		t.Errorf("expected second hunk to start at line 50, got %d", d.Hunks[1].StartLine)
	}
}

func TestParseWriteWithBreaks(t *testing.T) {
	// Write format with " ..." breaks should create multiple hunks
	input := `⏺ Write(/path/to/file.go)

────────────────────────────────────────────────────────────────────────────────
 Overwrite file file.go
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌
   1  package main
   2
   3  import (
   4 +  "crypto/sha256"
   5    "flag"
   6  )
 ...
  61      os.Exit(1)
  62    }
  63
  64 -  // Capture pane content
  64 +  if *watchFlag {
  65 +    return
  66 +  }
╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌
`

	diffs := Parse(input)

	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}

	d := diffs[0]
	if len(d.Hunks) != 2 {
		t.Fatalf("expected 2 hunks (separated by ...), got %d", len(d.Hunks))
	}

	// First hunk should start at line 1
	if d.Hunks[0].StartLine != 1 {
		t.Errorf("expected first hunk to start at line 1, got %d", d.Hunks[0].StartLine)
	}

	// Second hunk should start at line 61
	if d.Hunks[1].StartLine != 61 {
		t.Errorf("expected second hunk to start at line 61, got %d", d.Hunks[1].StartLine)
	}
}
