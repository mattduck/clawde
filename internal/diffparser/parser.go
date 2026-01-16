package diffparser

import (
	"regexp"
	"strings"
)

// Hunk represents a single diff hunk
type Hunk struct {
	StartLine int
	Lines     []DiffLine
}

// DiffLine represents a single line in a diff
type DiffLine struct {
	Type    LineType
	Content string
}

type LineType int

const (
	LineContext LineType = iota
	LineAdd
	LineDelete
)

// FileDiff represents a diff for a single file
type FileDiff struct {
	Path  string
	Hunks []Hunk
}

var (
	// Matches: ⏺ Update(/path/to/file) or ⏺ Write(/path/to/file)
	updatePattern = regexp.MustCompile(`^⏺ (Update|Write)\((.+)\)`)
	// Matches: summary line like "⎿  Added 2 lines, removed 3 lines"
	summaryPattern = regexp.MustCompile(`^\s*⎿`)
	// Matches: change line - LINENUM + single space + -/+ + content
	changePattern = regexp.MustCompile(`^\s+(\d+) ([-+])(.*)$`)
	// Matches: context line - LINENUM + two spaces + content
	contextPattern = regexp.MustCompile(`^\s+(\d+)  (.*)$`)
	// Matches: empty context line - just LINENUM
	emptyLinePattern = regexp.MustCompile(`^\s+(\d+)\s*$`)
	// Matches: hunk break marker (skipped lines)
	breakPattern = regexp.MustCompile(`^\s*\.\.\.`)
	// Matches: end of diff markers (other tool calls, thinking marker)
	endPattern = regexp.MustCompile(`^∴|^⏺ [^UW]`)
)

// Parse extracts file diffs from Claude's terminal output
func Parse(content string) []FileDiff {
	var diffs []FileDiff
	var currentDiff *FileDiff
	var currentHunk *Hunk

	lines := strings.Split(content, "\n")

	for _, line := range lines {
		// Check for new file
		if match := updatePattern.FindStringSubmatch(line); match != nil {
			// Save previous diff if exists
			if currentDiff != nil && currentHunk != nil && len(currentHunk.Lines) > 0 {
				currentDiff.Hunks = append(currentDiff.Hunks, *currentHunk)
			}
			if currentDiff != nil {
				diffs = append(diffs, *currentDiff)
			}

			currentDiff = &FileDiff{Path: match[2]}
			currentHunk = &Hunk{}
			continue
		}

		// Skip summary line
		if summaryPattern.MatchString(line) {
			continue
		}

		// Check for end of diff
		if endPattern.MatchString(line) {
			if currentDiff != nil && currentHunk != nil && len(currentHunk.Lines) > 0 {
				currentDiff.Hunks = append(currentDiff.Hunks, *currentHunk)
			}
			if currentDiff != nil {
				diffs = append(diffs, *currentDiff)
			}
			currentDiff = nil
			currentHunk = nil
			continue
		}

		if currentDiff == nil {
			continue
		}

		// Check for hunk break (... indicating skipped lines)
		if breakPattern.MatchString(line) {
			// Save current hunk and start a new one
			if currentHunk != nil && len(currentHunk.Lines) > 0 {
				currentDiff.Hunks = append(currentDiff.Hunks, *currentHunk)
				currentHunk = &Hunk{}
			}
			continue
		}

		// Try to match change line (single space before -/+)
		if match := changePattern.FindStringSubmatch(line); match != nil {
			lineNum := parseLineNum(match[1])
			if currentHunk.StartLine == 0 {
				currentHunk.StartLine = lineNum
			}

			lineType := LineDelete
			if match[2] == "+" {
				lineType = LineAdd
			}
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    lineType,
				Content: match[3],
			})
			continue
		}

		// Try to match context line (two spaces after line number)
		if match := contextPattern.FindStringSubmatch(line); match != nil {
			lineNum := parseLineNum(match[1])
			if currentHunk.StartLine == 0 {
				currentHunk.StartLine = lineNum
			}
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    LineContext,
				Content: match[2],
			})
			continue
		}

		// Try to match empty line (just line number)
		if match := emptyLinePattern.FindStringSubmatch(line); match != nil {
			lineNum := parseLineNum(match[1])
			if currentHunk.StartLine == 0 {
				currentHunk.StartLine = lineNum
			}
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    LineContext,
				Content: "",
			})
			continue
		}
	}

	// Don't forget the last diff
	if currentDiff != nil && currentHunk != nil && len(currentHunk.Lines) > 0 {
		currentDiff.Hunks = append(currentDiff.Hunks, *currentHunk)
	}
	if currentDiff != nil && len(currentDiff.Hunks) > 0 {
		diffs = append(diffs, *currentDiff)
	}

	return diffs
}

func parseLineNum(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

// ToUnified converts a FileDiff to unified diff format
func (d *FileDiff) ToUnified() string {
	var sb strings.Builder

	// Header
	if strings.HasPrefix(d.Path, "/") {
		sb.WriteString("--- a" + d.Path + "\n")
		sb.WriteString("+++ b" + d.Path + "\n")
	} else {
		sb.WriteString("--- a/" + d.Path + "\n")
		sb.WriteString("+++ b/" + d.Path + "\n")
	}

	for _, hunk := range d.Hunks {
		// Hunk header (simplified - just start line)
		sb.WriteString("@@ -")
		sb.WriteString(intToStr(hunk.StartLine))
		sb.WriteString(" +")
		sb.WriteString(intToStr(hunk.StartLine))
		sb.WriteString(" @@\n")

		for _, line := range hunk.Lines {
			switch line.Type {
			case LineContext:
				sb.WriteString(" ")
			case LineAdd:
				sb.WriteString("+")
			case LineDelete:
				sb.WriteString("-")
			}
			sb.WriteString(line.Content)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// ToUnifiedAll converts multiple FileDiffs to a single unified diff string
func ToUnifiedAll(diffs []FileDiff) string {
	var sb strings.Builder
	for i, d := range diffs {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(d.ToUnified())
	}
	return sb.String()
}
