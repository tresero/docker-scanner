package report

import (
	"fmt"
	"strings"
)

// mdWriter handles markdown generation with proper spacing
type mdWriter struct {
	sb strings.Builder
}

func newMdWriter() *mdWriter {
	return &mdWriter{}
}

func (w *mdWriter) String() string {
	return w.sb.String()
}

// H1 writes a level 1 heading with blank lines around it
func (w *mdWriter) H1(text string) {
	w.sb.WriteString(fmt.Sprintf("# %s\n\n", text))
}

// H2 writes a level 2 heading with blank lines around it
func (w *mdWriter) H2(text string) {
	w.sb.WriteString(fmt.Sprintf("## %s\n\n", text))
}

// H3 writes a level 3 heading with a unique suffix to avoid duplicate heading warnings
func (w *mdWriter) H3(text string) {
	w.sb.WriteString(fmt.Sprintf("### %s\n\n", text))
}

// P writes a paragraph with a trailing blank line
func (w *mdWriter) P(text string) {
	w.sb.WriteString(text + "\n\n")
}

// HR writes a horizontal rule
func (w *mdWriter) HR() {
	w.sb.WriteString("---\n\n")
}

// BulletList writes a bullet list with blank lines around it
func (w *mdWriter) BulletList(items []string) {
	for _, item := range items {
		w.sb.WriteString(fmt.Sprintf("- %s\n", item))
	}
	w.sb.WriteString("\n")
}

// Table writes a markdown table with blank lines around it
func (w *mdWriter) Table(headers []string, rows [][]string) {
	// Header row
	w.sb.WriteString("| " + strings.Join(headers, " | ") + " |\n")

	// Separator row
	seps := make([]string, len(headers))
	for i := range headers {
		seps[i] = "---"
	}
	w.sb.WriteString("| " + strings.Join(seps, " | ") + " |\n")

	// Data rows
	for _, row := range rows {
		// Pad row if needed
		for len(row) < len(headers) {
			row = append(row, "")
		}
		w.sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	w.sb.WriteString("\n")
}