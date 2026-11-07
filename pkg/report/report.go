package report

import (
	"docker-scanner/pkg/models"
	"fmt"
	"strings"
	"time"
)

// ImageStatus represents the visual status category for an image.
type ImageStatus int

const (
	StatusGood      ImageStatus = iota // pinned, up to date
	StatusUpgrade                      // pinned, newer version available
	StatusDowngrade                    // recommended is older than running
	StatusFloating                     // uses latest/main/etc.
	StatusUnverified                   // the scan couldn't confirm the status
)

// ClassifyImage returns the status category for color coding.
func ClassifyImage(r models.ImageInfo) ImageStatus {
	if r.UsesLatest {
		return StatusFloating
	}
	if r.IsDowngrade {
		return StatusDowngrade
	}
	if r.Unverified {
		return StatusUnverified
	}
	if r.RecommendedVersion != "" && !sameTag(r.RecommendedVersion, r.Image.Tag) {
		return StatusUpgrade
	}
	return StatusGood
}

// countUnverified returns how many images the scan couldn't confirm. It's
// separate from countByStatus because the JSON report has no count for it.
func countUnverified(results []models.ImageInfo) int {
	count := 0
	for _, r := range results {
		if ClassifyImage(r) == StatusUnverified {
			count++
		}
	}
	return count
}

// sameTag treats 2.8.11 and v2.8.11 as the same version.
func sameTag(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}

// countByStatus returns counts of each status category.
func countByStatus(results []models.ImageInfo) (floating, upgrades, downgrades, good int) {
	for _, r := range results {
		switch ClassifyImage(r) {
		case StatusFloating:
			floating++
		case StatusUpgrade:
			upgrades++
		case StatusDowngrade:
			downgrades++
		case StatusGood:
			good++
		}
	}
	return
}

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
	securityCount := 0
	projects := make(map[string]bool)

	for _, r := range results {
		securityCount += len(r.SecurityIssues)
		projects[r.Image.Project] = true
	}

	floating, upgrades, downgrades, good := countByStatus(results)

	sb.WriteString("  SUMMARY\n")
	sb.WriteString("  ───────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  Projects scanned:    %d\n", len(projects)))
	sb.WriteString(fmt.Sprintf("  Images found:        %d\n", len(results)))
	sb.WriteString(fmt.Sprintf("  Up to date:          %d", good))
	if good == len(results) {
		sb.WriteString("  ✅")
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  Upgrades available:  %d", upgrades))
	if upgrades > 0 {
		sb.WriteString("  🟡")
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  Using floating tag:  %d", floating))
	if floating > 0 {
		sb.WriteString("  🔴")
	}
	sb.WriteString("\n")
	if unverified := countUnverified(results); unverified > 0 {
		sb.WriteString(fmt.Sprintf("  Not verified:        %d  ❓\n", unverified))
	}
	if downgrades > 0 {
		sb.WriteString(fmt.Sprintf("  Downgrades:          %d  ⬇️\n", downgrades))
	}
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

		var status string
		switch ClassifyImage(r) {
		case StatusFloating:
			status = "🔴"
		case StatusUpgrade:
			status = "🟡"
		case StatusDowngrade:
			status = "⬇️"
		case StatusUnverified:
			status = "❓"
		default:
			status = "✅"
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
			sb.WriteString("      ⚠️  Uses floating tag — pin to a specific version\n")
		}

		if r.RecommendedVersion != "" {
			rec := fmt.Sprintf("      → Recommended: %s", r.RecommendedVersion)
			if r.RecommendedAge != "" && r.RecommendedAge != "age unknown" {
				rec += fmt.Sprintf(" (%s)", r.RecommendedAge)
			}
			if r.MajorVersionJump {
				rec += " 💥 major jump"
			}
			if r.IsDowngrade {
				rec += " ⬇️ downgrade"
			}
			sb.WriteString(rec + "\n")
		}

		if r.NewerMajorVersion != "" {
			sb.WriteString(fmt.Sprintf("      💥 Newer major: %s\n", r.NewerMajorVersion))
		}

		if r.RecommendationNote != "" {
			sb.WriteString(fmt.Sprintf("      Note:      %s\n", r.RecommendationNote))
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