package internal

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/term"
	"github.com/mattn/go-isatty"
)

var (
	// ANSI base colors (0-15) follow terminal theme; 240+ for subtle grays
	colorRed    = lipgloss.AdaptiveColor{Light: "1", Dark: "9"}
	colorYellow = lipgloss.AdaptiveColor{Light: "3", Dark: "11"}
	ColorGreen  = lipgloss.AdaptiveColor{Light: "2", Dark: "10"}
	colorBlue   = lipgloss.AdaptiveColor{Light: "4", Dark: "12"}
	colorCyan   = lipgloss.AdaptiveColor{Light: "6", Dark: "14"}
	colorMagenta = lipgloss.AdaptiveColor{Light: "5", Dark: "13"}
	ColorDim    = lipgloss.AdaptiveColor{Light: "8", Dark: "8"}
	headerFg    = lipgloss.AdaptiveColor{Light: "0", Dark: "15"}
	borderFg    = lipgloss.AdaptiveColor{Light: "240", Dark: "247"}

	statusColor = map[string]lipgloss.TerminalColor{
		"open":        ColorGreen,
		"in_progress": colorYellow,
		"blocked":     colorRed,
		"deferred":    ColorDim,
		"closed":      colorRed,
		"tombstone":   ColorDim,
	}

	typeColor = map[string]lipgloss.TerminalColor{
		"bug":      colorRed,
		"feature":  ColorGreen,
		"epic":     colorMagenta,
		"task":     colorBlue,
		"chore":    ColorDim,
		"docs":     colorCyan,
		"question": colorCyan,
	}
)

func termWidth() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w < 40 {
		return 80
	}
	return w
}

// truncateLines word-wraps text to lineWidth and keeps at most maxLines.
// If the text exceeds maxLines, the last line is trimmed with "...".
func truncateLines(text string, lineWidth, maxLines int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) > lineWidth {
			lines = append(lines, cur)
			cur = w
		} else {
			cur += " " + w
		}
	}
	lines = append(lines, cur)

	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}

	kept := lines[:maxLines]
	last := kept[maxLines-1]
	if len(last)+1 >= lineWidth {
		last = last[:lineWidth-1] + "…"
	} else {
		last += "…"
	}
	kept[maxLines-1] = last
	return strings.Join(kept, "\n")
}

func PrintIssueTable(records []IssueRecord, showStatus bool) {
	if len(records) == 0 {
		return
	}

	isTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())

	if !isTTY {
		for _, rec := range records {
			p := "-"
			if rec.Issue.Priority != nil {
				p = fmt.Sprintf("P%d", *rec.Issue.Priority)
			}
			t := rec.Issue.IssueType
			if t == "" {
				t = "task"
			}
			if showStatus {
				fmt.Printf("%-12s  %s  %-12s  [%s]  %s\n", rec.Issue.ID, p, rec.Issue.Status, t, rec.Issue.Title)
			} else {
				fmt.Printf("%-12s  %s  [%s]  %s\n", rec.Issue.ID, p, t, rec.Issue.Title)
			}
		}
		return
	}

	w := termWidth()

	// Measure widest ID unless the list is large (>50), where it's not worth the scan
	idWidth := 14 // sensible default
	if len(records) <= 50 {
		idWidth = 2
		for _, rec := range records {
			if len(rec.Issue.ID) > idWidth {
				idWidth = len(rec.Issue.ID)
			}
		}
	}
	idWidth += 2 // cell padding

	const (
		priWidth    = 5  // " P0 " + padding
		typeWidth   = 10 // " feature " + padding
		statusWidth = 13 // " in_progress " + padding
	)

	headers := []string{"ID", "P", "Type", "Title"}
	titleCol := 3
	if showStatus {
		headers = []string{"ID", "P", "Status", "Type", "Title"}
		titleCol = 4
	}

	t := table.New().
		Headers(headers...).
		Width(w).
		BorderRow(true).
		BorderStyle(lipgloss.NewStyle().Foreground(borderFg)).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)

			// Fixed-width columns prevent wrapping on everything except Title
			statusCol := -1
			typeCol := titleCol - 1
			if showStatus {
				statusCol = 2
			}
			switch {
			case col == 0:
				s = s.Width(idWidth)
			case col == 1:
				s = s.Width(priWidth)
			case col == statusCol:
				s = s.Width(statusWidth)
			case col == typeCol:
				s = s.Width(typeWidth)
			}

			if row == table.HeaderRow {
				return s.Bold(true).Foreground(headerFg)
			}

			rec := records[row]

			// Priority
			if col == 1 && rec.Issue.Priority != nil {
				switch *rec.Issue.Priority {
				case 0, 1:
					return s.Foreground(colorRed).Bold(true)
				case 2:
					return s.Foreground(colorYellow)
				case 3, 4:
					return s.Foreground(ColorDim)
				}
			}

			// Status
			if col == statusCol {
				if c, ok := statusColor[rec.Issue.Status]; ok {
					return s.Foreground(c)
				}
			}

			// Type
			if col == typeCol {
				typ := rec.Issue.IssueType
				if typ == "" {
					typ = "task"
				}
				if c, ok := typeColor[typ]; ok {
					return s.Foreground(c)
				}
			}

			return s
		})

	// Title column width: total minus fixed columns minus borders (ncols+1 border chars)
	ncols := len(headers)
	fixedWidth := idWidth + priWidth + typeWidth
	if showStatus {
		fixedWidth += statusWidth
	}
	titleWidth := w - fixedWidth - (ncols + 1) - 2 // -2 for cell padding
	if titleWidth < 10 {
		titleWidth = 10
	}

	for _, rec := range records {
		p := "-"
		if rec.Issue.Priority != nil {
			p = fmt.Sprintf("P%d", *rec.Issue.Priority)
		}
		typ := rec.Issue.IssueType
		if typ == "" {
			typ = "task"
		}
		title := truncateLines(rec.Issue.Title, titleWidth, 3)
		if showStatus {
			t.Row(rec.Issue.ID, p, rec.Issue.Status, typ, title)
		} else {
			t.Row(rec.Issue.ID, p, typ, title)
		}
	}

	title := fmt.Sprintf("Issues (%d)", len(records))
	fmt.Println(lipgloss.NewStyle().
		Width(w).
		Align(lipgloss.Center).
		Bold(true).
		Foreground(headerFg).
		Render(title))
	fmt.Println(t)
}
