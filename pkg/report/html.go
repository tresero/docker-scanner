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

// htmlData is the data structure passed to the HTML template
type htmlData struct {
	Generated         string
	ProjectCount      int
	ImageCount        int
	LatestCount       int
	SecurityCount     int
	HasLatest         bool
	HasSecurityIssues bool
	Images            []models.ImageInfo
	HighIssues        []models.SecurityIssue
	MediumIssues      []models.SecurityIssue
	LowIssues         []models.SecurityIssue
}

// GenerateHTML produces an email-friendly HTML report using html/template
func GenerateHTML(results []models.ImageInfo) string {
	data := buildHTMLData(results)

	funcMap := template.FuncMap{
		"odd": func(i int) bool { return i%2 == 1 },
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
	latestCount := 0
	var allIssues []models.SecurityIssue

	for _, r := range results {
		projects[r.Image.Project] = true
		if r.UsesLatest {
			latestCount++
		}
		allIssues = append(allIssues, r.SecurityIssues...)
	}

	return htmlData{
		Generated:         time.Now().Format("2006-01-02 15:04:05"),
		ProjectCount:      len(projects),
		ImageCount:        len(results),
		LatestCount:       latestCount,
		SecurityCount:     len(allIssues),
		HasLatest:         latestCount > 0,
		HasSecurityIssues: len(allIssues) > 0,
		Images:            results,
		HighIssues:        filterBySeverity(allIssues, "high"),
		MediumIssues:      filterBySeverity(allIssues, "medium"),
		LowIssues:         filterBySeverity(allIssues, "low"),
	}
}