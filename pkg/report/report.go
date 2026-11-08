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

// String is the status's name in the JSON report and the HTML template.
func (s ImageStatus) String() string {
	switch s {
	case StatusFloating:
		return "floating"
	case StatusUpgrade:
		return "upgrade"
	case StatusDowngrade:
		return "downgrade"
	case StatusUnverified:
		return "unverified"
	default:
		return "good"
	}
}

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
	if r.ResolvedRunningVersion != "" && sameTag(r.RecommendedVersion, r.ResolvedRunningVersion) {
		return StatusGood
	}
	if r.RecommendedVersion != "" && !sameTag(r.RecommendedVersion, r.Image.Tag) {
		return StatusUpgrade
	}
	return StatusGood
}

// withResolvedRunning returns results with the real running version in place of
// a tag like 16-alpine, for reports people read. JSON keeps both fields.
func withResolvedRunning(results []models.ImageInfo) []models.ImageInfo {
	shown := make([]models.ImageInfo, len(results))
	copy(shown, results)
	for i := range shown {
		if shown[i].ResolvedRunningVersion != "" {
			shown[i].RunningVersion = shown[i].ResolvedRunningVersion
		}
	}
	return shown
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

// runningLabel says what state the container is in, for reports to show beside
// the running version. A healthy running container needs no label.
func runningLabel(r models.ImageInfo) string {
	switch r.ContainerState {
	case "exited":
		if r.ExitCode != nil {
			return fmt.Sprintf("exited (%d)", *r.ExitCode)
		}
		return "exited"
	case "running":
		if r.RunningVersion == "" {
			return "running"
		}
		return ""
	case "":
		switch {
		case r.RunningVersion != "":
			return ""
		case r.Dockerfile != "" && r.Image.Service == "":
			return "not a service"
		}
		return "not running"
	}
	return r.ContainerState
}

// containerProblem reports whether the container needs attention now. A clean
// exit, or a service that was never started, may be fine.
func containerProblem(r models.ImageInfo) bool {
	switch r.ContainerState {
	case "unhealthy", "restarting", "dead":
		return true
	case "exited":
		return r.ExitCode != nil && *r.ExitCode != 0
	}
	return false
}

func countContainerProblems(results []models.ImageInfo) int {
	count := 0
	for _, r := range results {
		if containerProblem(r) {
			count++
		}
	}
	return count
}

// runningText is the running version and container state as plain text.
func runningText(r models.ImageInfo, version string) string {
	label := runningLabel(r)
	if containerProblem(r) {
		label = "🚨 " + label
	}
	switch {
	case version == "":
		return label
	case label == "":
		return version
	}
	return version + " " + label
}

// newerBuildText describes a newer build of the compose tag, or "" if there isn't one.
func newerBuildText(r models.ImageInfo) string {
	if !r.NewerBuildAvailable {
		return ""
	}
	var dates []string
	if r.NewerBuildDate != "" {
		dates = append(dates, "pushed "+dateOnly(r.NewerBuildDate))
	}
	if r.RunningBuildDate != "" {
		dates = append(dates, "running image built "+dateOnly(r.RunningBuildDate))
	}
	text := "newer build of " + r.Image.Tag
	if len(dates) > 0 {
		text += " (" + strings.Join(dates, "; ") + ")"
	}
	return text
}

// dateOnly trims an RFC 3339 time to its date.
func dateOnly(timestamp string) string {
	date, _, _ := strings.Cut(timestamp, "T")
	return date
}

func countNewerBuilds(results []models.ImageInfo) int {
	count := 0
	for _, r := range results {
		if r.NewerBuildAvailable {
			count++
		}
	}
	return count
}

// tagMismatchText warns that the next up -d will change the running version.
func tagMismatchText(r models.ImageInfo) string {
	if !r.TagMismatch {
		return ""
	}
	running := r.RunningVersion
	if r.ResolvedRunningVersion != "" {
		running = r.ResolvedRunningVersion
	}
	effect := "change"
	if r.TagMismatchDowngrade {
		effect = "downgrade"
	}
	return fmt.Sprintf("compose %s, running %s: next up -d will %s this", r.Image.Tag, running, effect)
}

func countTagMismatches(results []models.ImageInfo) int {
	count := 0
	for _, r := range results {
		if r.TagMismatch {
			count++
		}
	}
	return count
}

// baseImageText explains that a row is a Dockerfile's base image, and what upgrading
// takes: pulling won't help a locally built image.
func baseImageText(r models.ImageInfo) string {
	if r.Dockerfile == "" {
		return ""
	}
	text := "base image, from " + r.Dockerfile
	if r.LocalImage != "" {
		text = "base image of " + r.LocalImage + ", from " + r.Dockerfile
	}
	if r.NewerBuildAvailable || ClassifyImage(r) == StatusUpgrade {
		text += "; rebuild from the newer base to upgrade"
	}
	return text
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
	results = withResolvedRunning(results)
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
	if problems := countContainerProblems(results); problems > 0 {
		sb.WriteString(fmt.Sprintf("  Container problems:  %d  🚨\n", problems))
	}
	if builds := countNewerBuilds(results); builds > 0 {
		sb.WriteString(fmt.Sprintf("  Newer builds:        %d  🔄\n", builds))
	}
	if mismatches := countTagMismatches(results); mismatches > 0 {
		sb.WriteString(fmt.Sprintf("  Tag mismatches:      %d  🔀\n", mismatches))
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
		version := r.RunningVersion
		if version == "unknown" {
			version = "latest (unknown)"
		}
		sb.WriteString(fmt.Sprintf("      Running:   %s\n", runningText(r, version)))
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

		if r.DataUpgradeWarning != "" {
			sb.WriteString(fmt.Sprintf("      🗄️  %s\n", r.DataUpgradeWarning))
		}

		if base := baseImageText(r); base != "" {
			sb.WriteString(fmt.Sprintf("      🏗️  %s\n", base))
		}

		if build := newerBuildText(r); build != "" {
			sb.WriteString(fmt.Sprintf("      🔄 %s\n", build))
		}

		if mismatch := tagMismatchText(r); mismatch != "" {
			sb.WriteString(fmt.Sprintf("      🔀 %s\n", mismatch))
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