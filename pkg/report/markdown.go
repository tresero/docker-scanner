package report

import (
	"docker-scanner/pkg/models"
	"fmt"
	"strings"
	"time"
)

// GenerateMarkdown produces a markdown-formatted report
func GenerateMarkdown(results []models.ImageInfo) string {
	w := newMdWriter()

	buildHeader(w)
	buildSummary(w, results)
	buildImageTable(w, results)
	buildLatestDetails(w, results)
	buildSecuritySection(w, results)

	return w.String()
}

func buildHeader(w *mdWriter) {
	w.H1("Docker Compose Security & Version Report")
	w.P(fmt.Sprintf("**Generated:** %s", time.Now().Format("2006-01-02 15:04:05")))
	w.HR()
}

func buildSummary(w *mdWriter, results []models.ImageInfo) {
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

	latestStatus := fmt.Sprintf("%d", latestCount)
	if latestCount > 0 {
		latestStatus = fmt.Sprintf("**%d** :warning:", latestCount)
	}

	securityStatus := fmt.Sprintf("%d", securityCount)
	if securityCount > 0 {
		securityStatus = fmt.Sprintf("**%d** :warning:", securityCount)
	}

	w.H2("Summary")
	w.Table(
		[]string{"Metric", "Count"},
		[][]string{
			{"Projects scanned", fmt.Sprintf("%d", len(projects))},
			{"Images found", fmt.Sprintf("%d", len(results))},
			{"Using `latest` tag", latestStatus},
			{"Security issues", securityStatus},
		},
	)
}

func buildImageTable(w *mdWriter, results []models.ImageInfo) {
	rows := make([][]string, 0, len(results))

	for _, r := range results {
		status := ":white_check_mark:"
		if r.UsesLatest {
			status = ":warning:"
		}

		fullImage := fmt.Sprintf("`%s/%s`", r.Image.Registry, r.Image.Name)
		tag := fmt.Sprintf("`%s`", r.Image.Tag)

		recommended := "-"
		if r.RecommendedVersion != "" {
			recommended = fmt.Sprintf("`%s`", r.RecommendedVersion)
			if r.RecommendedAge != "" && r.RecommendedAge != "age unknown" {
				recommended += fmt.Sprintf(" (%s)", r.RecommendedAge)
			} else if r.RecommendedAge == "age unknown" {
				recommended += " (age unknown)"
			}
			if r.MajorVersionJump {
				recommended += " :boom:"
			}
		}

		running := "not running"
		if r.RunningVersion == "unknown" {
			running = "`latest` (unknown)"
		} else if r.RunningVersion != "" {
			running = fmt.Sprintf("`%s`", r.RunningVersion)
		}

		rows = append(rows, []string{
			status, r.Image.Project, r.Image.Service, fullImage, tag, running, recommended,
		})
	}

	w.H2("Images")
	w.Table(
		[]string{"Status", "Project", "Service", "Image", "Tag", "Running", "Recommended"},
		rows,
	)
}

func buildLatestDetails(w *mdWriter, results []models.ImageInfo) {
	hasLatest := false
	for _, r := range results {
		if r.UsesLatest {
			hasLatest = true
			break
		}
	}

	if !hasLatest {
		return
	}

	w.H2(":warning: Images Using `latest`")
	w.P("Using `latest` is a security risk because:")
	w.BulletList([]string{
		"Builds are not reproducible",
		"Breaking changes can be pulled in without warning",
		"No audit trail of which version is running",
	})

	for _, r := range results {
		if !r.UsesLatest {
			continue
		}

		fullImage := r.Image.Registry + "/" + r.Image.Name
		// Include project name to avoid duplicate heading warnings
		w.H3(fmt.Sprintf("`%s` (%s)", fullImage, r.Image.Project))

		var details []string
		details = append(details, fmt.Sprintf("**Service:** %s", r.Image.Service))
		details = append(details, fmt.Sprintf("**File:** `%s`", r.File))

		if r.RunningVersion != "" {
			details = append(details, fmt.Sprintf("**Running:** `%s`", r.RunningVersion))
		}

		if r.RecommendedVersion != "" {
			recLine := fmt.Sprintf("**Recommended:** `%s`", r.RecommendedVersion)
			if r.RecommendedAge != "" && r.RecommendedAge != "age unknown" {
				recLine += fmt.Sprintf(" (released %s ago)", r.RecommendedAge)
			} else if r.RecommendedAge == "age unknown" {
				recLine += " (age unknown)"
			}
			details = append(details, recLine)
		}

		if r.MajorVersionJump {
			details = append(details, ":boom: **Major version jump — review breaking changes before upgrading**")
		}

		if r.IsDowngrade {
			details = append(details, ":arrow_down: **DOWNGRADE — recommended is older than running version, verify database compatibility**")
		}

		if len(r.AvailableVersions) > 0 {
			details = append(details, fmt.Sprintf("**Available versions:** %s",
				formatVersionList(r.AvailableVersions)))
		}

		w.BulletList(details)
	}
}

func buildSecuritySection(w *mdWriter, results []models.ImageInfo) {
	// Add version help section for unknowns
	buildVersionHelp(w, results)

	var allIssues []models.SecurityIssue
	for _, r := range results {
		allIssues = append(allIssues, r.SecurityIssues...)
	}

	w.H2("Security")

	if len(allIssues) == 0 {
		w.P(":white_check_mark: No security issues found.")
		return
	}

	high := filterBySeverity(allIssues, "high")
	medium := filterBySeverity(allIssues, "medium")
	low := filterBySeverity(allIssues, "low")

	if len(high) > 0 {
		w.H3(":red_circle: High Severity")
		buildIssueTable(w, high)
	}

	if len(medium) > 0 {
		w.H3(":yellow_circle: Medium Severity")
		buildIssueTable(w, medium)
	}

	if len(low) > 0 {
		w.H3(":blue_circle: Low Severity")
		buildIssueTable(w, low)
	}
}

func buildIssueTable(w *mdWriter, issues []models.SecurityIssue) {
	rows := make([][]string, 0, len(issues))
	for _, issue := range issues {
		rows = append(rows, []string{
			issue.Description,
			fmt.Sprintf("`%s`", issue.Location),
			issue.Suggestion,
		})
	}

	w.Table([]string{"Issue", "Location", "Suggestion"}, rows)
}

func formatVersionList(versions []string) string {
	var formatted []string
	for _, v := range versions {
		formatted = append(formatted, fmt.Sprintf("`%s`", v))
	}
	return strings.Join(formatted, ", ")
}

func buildVersionHelp(w *mdWriter, results []models.ImageInfo) {
	var unknowns []models.ImageInfo
	for _, r := range results {
		if r.RunningVersion == "unknown" {
			unknowns = append(unknowns, r)
		}
	}

	if len(unknowns) == 0 {
		return
	}

	w.H2("Unknown Running Versions")
	w.P("The following containers are running `latest` but the exact version could not be determined. This is a common problem with `latest` tags — another reason to pin to a specific version.")

	var items []string
	for _, r := range unknowns {
		name := r.Image.ContainerName
		if name == "" {
			name = r.Image.Service
		}
		items = append(items, fmt.Sprintf("**%s** — container: `%s` — image: `%s/%s`",
			r.Image.Service, name, r.Image.Registry, r.Image.Name))
	}
	w.BulletList(items)
}