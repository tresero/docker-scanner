package report

import (
	"bytes"
	"docker-scanner/pkg/models"
	"embed"
	"html/template"
	"time"
)

//go:embed templates/report.html
var templateFS embed.FS

// Styles holds all inline CSS styles used in the HTML template
type Styles struct {
	Body          template.CSS
	H1            template.CSS
	H2            template.CSS
	H3            template.CSS
	Table         template.CSS
	Th            template.CSS
	Td            template.CSS
	TdAlt         template.CSS
	Code          template.CSS
	Warn          template.CSS
	Ok            template.CSS
	Meta          template.CSS
	BadgeHigh     template.CSS
	BadgeMed      template.CSS
	BadgeLow      template.CSS
	Boom          template.CSS
	Downgrade     template.CSS
	Unknown       template.CSS
	Muted         template.CSS
	RowFloating   template.CSS
	RowUpgrade    template.CSS
	RowGood       template.CSS
	RowUnknown    template.CSS
	RowDowngrade  template.CSS
}

// LegendEntry describes one symbol or phrase used in the report so readers
// can decode the table at a glance. Defined alongside the styles so adding
// a new status symbol means updating one place, not three.
type LegendEntry struct {
	Symbol      template.HTML
	SymbolStyle template.CSS
	Meaning     string
}

var styles = Styles{
	Body:      "font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#1a1a1a;max-width:900px;margin:0 auto;padding:20px;",
	H1:        "font-size:24px;font-weight:600;border-bottom:2px solid #e1e4e8;padding-bottom:8px;",
	H2:        "font-size:20px;font-weight:600;margin-top:24px;color:#24292f;",
	H3:        "font-size:16px;font-weight:600;margin-top:20px;color:#24292f;",
	Table:     "border-collapse:collapse;width:100%;margin-bottom:16px;font-size:14px;",
	Th:        "background:#f6f8fa;border:1px solid #d0d7de;padding:8px 12px;text-align:left;font-weight:600;",
	Td:        "border:1px solid #d0d7de;padding:8px 12px;vertical-align:top;",
	TdAlt:     "border:1px solid #d0d7de;padding:8px 12px;vertical-align:top;background:#f6f8fa;",
	Code:      "background:#eff1f3;padding:2px 6px;border-radius:3px;font-family:monospace;font-size:13px;",
	Warn:      "color:#d1242f;font-weight:600;",
	Ok:        "color:#1a7f37;font-weight:600;",
	Meta:      "color:#656d76;font-size:13px;",
	BadgeHigh: "display:inline-block;padding:2px 8px;border-radius:12px;font-size:12px;font-weight:600;color:#fff;background:#cf222e;",
	BadgeMed:  "display:inline-block;padding:2px 8px;border-radius:12px;font-size:12px;font-weight:600;color:#fff;background:#bf8700;",
	BadgeLow:  "display:inline-block;padding:2px 8px;border-radius:12px;font-size:12px;font-weight:600;color:#fff;background:#0969da;",
	Boom:      "background:#fff8c5;border:1px solid #d4a72c;border-radius:4px;padding:4px 8px;font-size:13px;",
	Downgrade:    "background:#ffebe9;border:1px solid #d1242f;border-radius:4px;padding:4px 8px;font-size:13px;",
	Unknown:      "color:#bf8700;",
	Muted:        "color:#656d76;",
	RowFloating:  "background:#ffebe9;",
	RowUpgrade:   "background:#fff8c5;",
	RowGood:      "background:#dafbe1;",
	RowUnknown:   "background:#eaeef2;",
	RowDowngrade: "background:#f5e6d3;",
}

// legendEntries is the single source of truth for what symbols mean in the
// report. Add a new symbol to the template? Add it here too. The
// TestGenerateHTML_LegendCoversAllSymbols test will catch omissions.
var legendEntries = []LegendEntry{
	{Symbol: "✅", SymbolStyle: styles.Ok, Meaning: "Image is pinned and up to date (green row)"},
	{Symbol: "🟡", SymbolStyle: "", Meaning: "Upgrade available — a newer version exists (yellow row)"},
	{Symbol: "🔴", SymbolStyle: styles.Warn, Meaning: "Image uses a floating tag like latest or main (red row)"},
	{Symbol: "❓", SymbolStyle: "", Meaning: "Not verified — the scan could not confirm this image's status; the note says why (grey row)"},
	{Symbol: "⬇️", SymbolStyle: "", Meaning: "Recommended version is older than running version — downgrade risk (beige row)"},
	{Symbol: "💥", SymbolStyle: "", Meaning: "Recommended version is a major version jump — review breaking changes"},
	{Symbol: "latest (unknown)", SymbolStyle: styles.Unknown, Meaning: "Container is running but its exact version could not be determined"},
	{Symbol: "🚨", SymbolStyle: styles.Warn, Meaning: "Container needs attention: unhealthy, restarting, dead, or exited with an error"},
	{Symbol: "🔄", SymbolStyle: "", Meaning: "Newer build — the compose tag now points to a newer image than the one running; pull it to get base image fixes"},
	{Symbol: "🔀", SymbolStyle: "", Meaning: "Compose pins a different version than the container is running, so the next up -d will change it"},
	{Symbol: "🗄️", SymbolStyle: styles.Warn, Meaning: "Database: upgrading it takes more than a new image, and the note says what to do"},
	{Symbol: "Before upgrading", SymbolStyle: "", Meaning: "Docker only replaces the program, and the data stays as it was. Each app handles its own data differently. Back up first, and read the release notes for every version you are moving past. No 💥 does not mean an upgrade is safe for the data."},
	{Symbol: "🏗️", SymbolStyle: "", Meaning: "Base image of a locally built image — rebuild to upgrade, because pulling won't apply"},
	{Symbol: "exited (0)", SymbolStyle: styles.Muted, Meaning: "Container stopped cleanly, with exit code 0"},
	{Symbol: "not created", SymbolStyle: styles.Muted, Meaning: "Image is defined in compose but no container exists"},
	{Symbol: "not running", SymbolStyle: styles.Muted, Meaning: "No container was found, and Docker could not be asked"},
	{Symbol: "(age unknown)", SymbolStyle: styles.Muted, Meaning: "Recommended version exists but its release date could not be fetched"},
}

// htmlData is the data structure passed to the HTML template
type htmlData struct {
	Styles              Styles
	Generated           string
	ProjectCount        int
	ImageCount          int
	LatestCount         int
	UpgradeCount        int
	DowngradeCount      int
	GoodCount           int
	UnverifiedCount     int
	ContainerProblems   int
	NewerBuilds         int
	TagMismatches       int
	SecurityCount       int
	HasLatest           bool
	HasSecurityIssues   bool
	HasUnknownVersions  bool
	Images              []models.ImageInfo
	Legend              []LegendEntry
	HighIssues          []models.SecurityIssue
	MediumIssues        []models.SecurityIssue
	LowIssues           []models.SecurityIssue
}

// GenerateHTML produces an email-friendly HTML report using html/template
func GenerateHTML(results []models.ImageInfo) string {
	data := buildHTMLData(withResolvedRunning(results))

	funcMap := template.FuncMap{
		"odd":              func(i int) bool { return i%2 == 1 },
		"runningLabel":     runningLabel,
		"containerProblem": containerProblem,
		"newerBuildText":   newerBuildText,
		"tagMismatchText":  tagMismatchText,
		"baseImageText":    baseImageText,
		"imageStatus": func(r models.ImageInfo) string {
			return ClassifyImage(r).String()
		},
	}

	tmpl, err := template.New("report.html").Funcs(funcMap).ParseFS(templateFS, "templates/report.html")
	if err != nil {
		return "Error parsing template: " + err.Error()
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "Error executing template: " + err.Error()
	}

	return buf.String()
}

func buildHTMLData(results []models.ImageInfo) htmlData {
	projects := make(map[string]bool)
	hasUnknown := false
	var allIssues []models.SecurityIssue

	for _, r := range results {
		projects[r.Image.Project] = true
		if r.RunningVersion == "unknown" {
			hasUnknown = true
		}
		allIssues = append(allIssues, r.SecurityIssues...)
	}

	floating, upgrades, downgrades, good := countByStatus(results)

	return htmlData{
		Styles:             styles,
		Generated:          time.Now().Format("2006-01-02 15:04:05"),
		ProjectCount:       len(projects),
		ImageCount:         len(results),
		LatestCount:        floating,
		UpgradeCount:       upgrades,
		DowngradeCount:     downgrades,
		GoodCount:          good,
		UnverifiedCount:    countUnverified(results),
		ContainerProblems:  countContainerProblems(results),
		NewerBuilds:        countNewerBuilds(results),
		TagMismatches:      countTagMismatches(results),
		SecurityCount:      len(allIssues),
		HasLatest:          floating > 0,
		HasSecurityIssues:  len(allIssues) > 0,
		HasUnknownVersions: hasUnknown,
		Images:             results,
		Legend:             legendEntries,
		HighIssues:         filterBySeverity(allIssues, "high"),
		MediumIssues:       filterBySeverity(allIssues, "medium"),
		LowIssues:          filterBySeverity(allIssues, "low"),
	}
}