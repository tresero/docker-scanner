package report

import (
	"regexp"
	"strings"
	"testing"

	"docker-scanner/pkg/models"
)

// fixtureResults returns a small but representative set of ImageInfo records
// covering the rendering branches the template cares about: pinned vs latest,
// running/unknown/not-running, recommended vs none, downgrade, major jump,
// and security issues across all severities.
func fixtureResults() []models.ImageInfo {
	return []models.ImageInfo{
		{
			Image: models.Image{
				Project:  "massage",
				Service:  "db",
				Registry: "docker.io",
				Name:     "mariadb",
				Tag:      "10.11",
			},
			File:               "massage/docker-compose.yml",
			UsesLatest:         false,
			RunningVersion:     "10.11",
			RecommendedVersion: "12.2.2",
			RecommendedAge:     "1 week",
			MajorVersionJump:   true,
		},
		{
			Image: models.Image{
				Project:  "tunnel",
				Service:  "cloudflared",
				Registry: "docker.io",
				Name:     "cloudflare/cloudflared",
				Tag:      "latest",
			},
			File:               "tunnel/docker-compose.yml",
			UsesLatest:         true,
			RunningVersion:     "unknown",
			RecommendedVersion: "2026.3.0",
			RecommendedAge:     "1 month",
		},
		{
			Image: models.Image{
				Project:  "pocketbase",
				Service:  "pocketbase",
				Registry: "ghcr.io",
				Name:     "muchobien/pocketbase",
				Tag:      "latest",
			},
			File:               "pocketbase/docker-compose.yml",
			UsesLatest:         true,
			RunningVersion:     "0.36.5",
			RecommendedVersion: "0.17.6",
			RecommendedAge:     "age unknown",
			IsDowngrade:        true,
		},
		{
			Image: models.Image{
				Project:  "orphan",
				Service:  "ghost",
				Registry: "docker.io",
				Name:     "ghost",
				Tag:      "5",
			},
			File:           "orphan/docker-compose.yml",
			UsesLatest:     false,
			RunningVersion: "",
		},
		{
			Image: models.Image{
				Project:  "leaky",
				Service:  "app",
				Registry: "docker.io",
				Name:     "app",
				Tag:      "1.0",
			},
			File: "leaky/docker-compose.yml",
			SecurityIssues: []models.SecurityIssue{
				{Severity: "high", Description: "Hardcoded password", Location: "leaky/docker-compose.yml:12", Suggestion: "Move to .env"},
				{Severity: "medium", Description: "Hardcoded API key", Location: "leaky/docker-compose.yml:18", Suggestion: "Move to .env"},
				{Severity: "low", Description: "Missing .env file", Location: "leaky/", Suggestion: "Create .env"},
			},
		},
	}
}

func TestGenerateHTML_RendersTopLevelStructure(t *testing.T) {
	out := GenerateHTML(fixtureResults())

	mustContain(t, out, "<!DOCTYPE html>")
	mustContain(t, out, "Docker Compose Security &amp; Version Report")
	mustContain(t, out, "Generated:")
	mustContain(t, out, "<h2") // at least one section
}

func TestGenerateHTML_RendersSummaryCounts(t *testing.T) {
	out := GenerateHTML(fixtureResults())

	mustContain(t, out, "Projects scanned")
	mustContain(t, out, "Images found")
	mustContain(t, out, "Using <code")
	mustContain(t, out, "Security issues")
}

func TestGenerateHTML_RendersImagesTable(t *testing.T) {
	out := GenerateHTML(fixtureResults())

	// Headers
	for _, h := range []string{"Status", "Project", "Service", "Image", "Tag", "Running", "Recommended"} {
		mustContain(t, out, ">"+h+"<")
	}

	// Sample row data
	mustContain(t, out, "mariadb")
	mustContain(t, out, "cloudflare/cloudflared")
	mustContain(t, out, "muchobien/pocketbase")
}

func TestGenerateHTML_RendersStatusSymbols(t *testing.T) {
	out := GenerateHTML(fixtureResults())

	mustContain(t, out, "✅")
	mustContain(t, out, "⚠️")
	mustContain(t, out, "💥")
	mustContain(t, out, "⬇️")
}

func TestGenerateHTML_RendersRunningStates(t *testing.T) {
	out := GenerateHTML(fixtureResults())

	mustContain(t, out, "latest (unknown)")
	mustContain(t, out, "not running")
}

func TestGenerateHTML_RendersAgeAnnotations(t *testing.T) {
	out := GenerateHTML(fixtureResults())

	mustContain(t, out, "1 week")
	mustContain(t, out, "1 month")
	mustContain(t, out, "age unknown")
}

func TestGenerateHTML_RendersSecuritySections(t *testing.T) {
	out := GenerateHTML(fixtureResults())

	mustContain(t, out, ">HIGH<")
	mustContain(t, out, ">MEDIUM<")
	mustContain(t, out, ">LOW<")
	mustContain(t, out, "Hardcoded password")
	mustContain(t, out, "Hardcoded API key")
	mustContain(t, out, "Missing .env file")
}

func TestGenerateHTML_RendersUnknownVersionsSection(t *testing.T) {
	out := GenerateHTML(fixtureResults())

	mustContain(t, out, "Unknown Running Versions")
}

func TestGenerateHTML_NoSecurityIssuesShowsCleanMessage(t *testing.T) {
	clean := []models.ImageInfo{{
		Image: models.Image{Project: "p", Service: "s", Registry: "docker.io", Name: "n", Tag: "1.0"},
		File:  "p/docker-compose.yml",
	}}
	out := GenerateHTML(clean)

	mustContain(t, out, "No security issues found")
}

func TestGenerateHTML_RendersLegendSection(t *testing.T) {
	out := GenerateHTML(fixtureResults())

	mustContain(t, out, ">Legend<")
}

func TestGenerateHTML_LegendCoversAllSymbols(t *testing.T) {
	out := GenerateHTML(fixtureResults())

	legend := extractSection(t, out, "Legend", "Images")

	cases := []struct {
		name   string
		needle string
	}{
		{"pinned check", "✅"},
		{"floating tag warning", "⚠️"},
		{"major version jump", "💥"},
		{"downgrade arrow", "⬇️"},
		{"latest unknown phrase", "latest (unknown)"},
		{"not running phrase", "not running"},
		{"age unknown phrase", "age unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(legend, c.needle) {
				t.Errorf("legend section missing %s (%q)", c.name, c.needle)
			}
		})
	}
}

func TestGenerateHTML_LegendAppearsBeforeImagesTable(t *testing.T) {
	out := GenerateHTML(fixtureResults())

	legendIdx := strings.Index(out, ">Legend<")
	imagesIdx := strings.Index(out, ">Images<")

	if legendIdx == -1 {
		t.Fatal("Legend section not found")
	}
	if imagesIdx == -1 {
		t.Fatal("Images section not found")
	}
	if legendIdx >= imagesIdx {
		t.Errorf("Legend should appear before Images section (legend at %d, images at %d)", legendIdx, imagesIdx)
	}
}

func TestExtractSection_HandlesH2WithVariedFormatting(t *testing.T) {
	cases := []struct {
		name string
		html string
	}{
		{
			name: "current inline style",
			html: `<h2 style="font-size:20px;">Legend</h2><p>body</p><h2 style="font-size:20px;">Images</h2>`,
		},
		{
			name: "newline after opening tag",
			html: "<h2>\n  Legend\n</h2><p>body</p><h2>\n  Images\n</h2>",
		},
		{
			name: "wrapped in span",
			html: `<h2><span>Legend</span></h2><p>body</p><h2><span>Images</span></h2>`,
		},
		{
			name: "leading emoji prefix",
			html: `<h2>📋 Legend</h2><p>body</p><h2>🖼 Images</h2>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			section := extractSection(t, c.html, "Legend", "Images")
			if !strings.Contains(section, "body") {
				t.Errorf("expected extracted section to contain %q, got: %s", "body", section)
			}
			if strings.Contains(section, "Images") {
				t.Errorf("extracted section should stop before Images heading, got: %s", section)
			}
		})
	}
}

// mustContain fails the test if needle is not present in haystack.
func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q, but it did not", needle)
	}
}

// extractSection returns the substring of out between the heading containing
// startHeading and the next heading containing endHeading. Matches headings
// regardless of attributes, whitespace, or wrapping elements inside the <h2>.
// Used to scope assertions to a single section so a symbol appearing
// elsewhere in the document doesn't falsely satisfy a test.
func extractSection(t *testing.T, out, startHeading, endHeading string) string {
	t.Helper()

	startLoc := findHeadingLocation(out, startHeading)
	if startLoc == nil {
		t.Fatalf("section %q not found in output", startHeading)
	}

	// Search for the end heading in the content AFTER the start heading's
	// closing </h2>, otherwise the regex will match the start heading itself
	// when looking for the end pattern.
	bodyStart := startLoc[1]
	endLoc := findHeadingLocation(out[bodyStart:], endHeading)
	if endLoc == nil {
		return out[startLoc[0]:]
	}
	return out[startLoc[0] : bodyStart+endLoc[0]]
}

// findHeadingLocation returns [start, end] indices for an <h2>...</h2>
// heading whose text content contains the given heading string. Returns nil
// if not found. The match is tolerant of attributes on the <h2> tag and any
// markup or whitespace between the opening tag and the heading text.
func findHeadingLocation(s, heading string) []int {
	pattern := `<h2\b[^>]*>[\s\S]*?` + regexp.QuoteMeta(heading) + `[\s\S]*?</h2>`
	re := regexp.MustCompile(pattern)
	return re.FindStringIndex(s)
}