package report

import (
	"docker-scanner/pkg/models"
	"fmt"
	"strings"
	"time"
)

// Generate produces a human-readable report from scan results
func Generate(results []models.ImageInfo) string {
	var sb strings.Builder

	writeHeader(&sb)
	writeSummary(&sb, results)
	writeImageDetails(&sb, results)
	writeSecurityIssues(&sb, results)

	return sb.String()
}

func writeHeader(sb *strings.Builder) {
	sb.WriteString("\n")
	sb.WriteString("════════════════════════════════════════════════════════════════\n")
	sb.WriteString("  Docker Compose Security & Version Report\n")
	sb.WriteString("  Generated: " + time.Now().Format("2006-01-02 15:04:05") + "\n")
	sb.WriteString("════════════════════════════════════════════════════════════════\n\n")
}

func writeSummary(sb *strings.Builder, results []models.ImageInfo) {
	latestCount := 0
	securityCount := 0
	projects := make(map[string]bool)

	for _, r := range results {
		if r.UsesLatest {
			latestCount++
		}
		securityCount += len(r.SecurityIssues)
		projects[r.Image.Project] = true
	}

	sb.WriteString("  SUMMARY\n")
	sb.WriteString("  ───────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  Projects scanned:    %d\n", len(projects)))
	sb.WriteString(fmt.Sprintf("  Images found:        %d\n", len(results)))
	sb.WriteString(fmt.Sprintf("  Using 'latest' tag:  %d", latestCount))
	if latestCount > 0 {
		sb.WriteString("  ⚠️")
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  Security issues:     %d", securityCount))
	if securityCount > 0 {
		sb.WriteString("  ⚠️")
	}
	sb.WriteString("\n\n")
}

func writeImageDetails(sb *strings.Builder, results []models.ImageInfo) {
	sb.WriteString("  IMAGES\n")
	sb.WriteString("  ───────────────────────────────────────\n")

	for _, r := range results {
		fullImage := r.Image.Registry + "/" + r.Image.Name

		status := "✓"
		if r.UsesLatest {
			status = "⚠️"
		}

		sb.WriteString(fmt.Sprintf("\n  %s  %s\n", status, fullImage))
		sb.WriteString(fmt.Sprintf("      Project:   %s\n", r.Image.Project))
		sb.WriteString(fmt.Sprintf("      Service:   %s\n", r.Image.Service))
		sb.WriteString(fmt.Sprintf("      Tag:       %s\n", r.Image.Tag))
		if r.RunningVersion == "unknown" {
			sb.WriteString("      Running:   latest (unknown)\n")
		} else if r.RunningVersion != "" {
			sb.WriteString(fmt.Sprintf("      Running:   %s\n", r.RunningVersion))
		} else {
			sb.WriteString("      Running:   not running\n")
		}
		sb.WriteString(fmt.Sprintf("      File:      %s\n", r.File))

		if r.UsesLatest {
			sb.WriteString("      ⚠️  Uses 'latest' — pin to a specific version\n")
			if r.RecommendedVersion != "" {
				sb.WriteString(fmt.Sprintf("      → Recommended: %s\n", r.RecommendedVersion))
			}
		}

		if len(r.AvailableVersions) > 0 {
			sb.WriteString(fmt.Sprintf("      Available:  %s\n", strings.Join(r.AvailableVersions, ", ")))
		}
	}

	sb.WriteString("\n")
}

func writeSecurityIssues(sb *strings.Builder, results []models.ImageInfo) {
	var allIssues []models.SecurityIssue
	for _, r := range results {
		allIssues = append(allIssues, r.SecurityIssues...)
	}

	if len(allIssues) == 0 {
		sb.WriteString("  SECURITY\n")
		sb.WriteString("  ───────────────────────────────────────\n")
		sb.WriteString("  ✓  No security issues found\n\n")
		return
	}

	sb.WriteString("  SECURITY ISSUES\n")
	sb.WriteString("  ───────────────────────────────────────\n")

	// Group by severity
	high := filterBySeverity(allIssues, "high")
	medium := filterBySeverity(allIssues, "medium")
	low := filterBySeverity(allIssues, "low")

	if len(high) > 0 {
		sb.WriteString("\n  🔴 HIGH\n")
		for _, issue := range high {
			writeIssue(sb, issue)
		}
	}

	if len(medium) > 0 {
		sb.WriteString("\n  🟡 MEDIUM\n")
		for _, issue := range medium {
			writeIssue(sb, issue)
		}
	}

	if len(low) > 0 {
		sb.WriteString("\n  🔵 LOW\n")
		for _, issue := range low {
			writeIssue(sb, issue)
		}
	}

	sb.WriteString("\n")
}

func writeIssue(sb *strings.Builder, issue models.SecurityIssue) {
	sb.WriteString(fmt.Sprintf("    • %s\n", issue.Description))
	sb.WriteString(fmt.Sprintf("      Location:   %s\n", issue.Location))
	sb.WriteString(fmt.Sprintf("      Suggestion: %s\n", issue.Suggestion))
}

func filterBySeverity(issues []models.SecurityIssue, severity string) []models.SecurityIssue {
	var filtered []models.SecurityIssue
	for _, issue := range issues {
		if issue.Severity == severity {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}