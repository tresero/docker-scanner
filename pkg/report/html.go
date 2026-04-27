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
	Body       template.CSS
	H1         template.CSS
	H2         template.CSS
	H3         template.CSS
	Table      template.CSS
	Th         template.CSS
	Td         template.CSS
	TdAlt      template.CSS
	Code       template.CSS
	Warn       template.CSS
	Ok         template.CSS
	Meta       template.CSS
	BadgeHigh  template.CSS
	BadgeMed   template.CSS
	BadgeLow   template.CSS
	Boom       template.CSS
	Downgrade  template.CSS
	Unknown    template.CSS
	Muted      template.CSS
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
	Downgrade: "background:#ffebe9;border:1px solid #d1242f;border-radius:4px;padding:4px 8px;font-size:13px;",
	Unknown:   "color:#bf8700;",
	Muted:     "color:#656d76;",
}

// htmlData is the data structure passed to the HTML template
type htmlData struct {
	Styles              Styles
	Generated           string
	ProjectCount        int
	ImageCount          int
	LatestCount         int
	SecurityCount       int
	HasLatest           bool
	HasSecurityIssues   bool
	HasUnknownVersions  bool
	Images              []models.ImageInfo
	HighIssues          []models.SecurityIssue
	MediumIssues        []models.SecurityIssue
	LowIssues           []models.SecurityIssue
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
	hasUnknown := false
	var allIssues []models.SecurityIssue

	for _, r := range results {
		projects[r.Image.Project] = true
		if r.UsesLatest {
			latestCount++
		}
		if r.RunningVersion == "unknown" {
			hasUnknown = true
		}
		allIssues = append(allIssues, r.SecurityIssues...)
	}

	return htmlData{
		Styles:             styles,
		Generated:          time.Now().Format("2006-01-02 15:04:05"),
		ProjectCount:       len(projects),
		ImageCount:         len(results),
		LatestCount:        latestCount,
		SecurityCount:      len(allIssues),
		HasLatest:          latestCount > 0,
		HasSecurityIssues:  len(allIssues) > 0,
		HasUnknownVersions: hasUnknown,
		Images:             results,
		HighIssues:         filterBySeverity(allIssues, "high"),
		MediumIssues:       filterBySeverity(allIssues, "medium"),
		LowIssues:          filterBySeverity(allIssues, "low"),
	}
}