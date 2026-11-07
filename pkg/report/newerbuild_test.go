package report

import (
	"docker-scanner/pkg/models"
	"encoding/json"
	"strings"
	"testing"
)

func newerBuildImage(available bool, pushed, built string) models.ImageInfo {
	return models.ImageInfo{
		Image:               models.Image{Registry: "docker.io", Name: "mariadb", Tag: "11.4", Service: "db", Project: "web"},
		RunningVersion:      "11.4",
		ContainerState:      "running",
		NewerBuildAvailable: available,
		NewerBuildDate:      pushed,
		RunningBuildDate:    built,
	}
}

func TestNewerBuildText(t *testing.T) {
	const pushed, built = "2026-11-03T01:02:03Z", "2026-08-01T10:20:30Z"
	tests := []struct {
		name string
		info models.ImageInfo
		want string
	}{
		{"no newer build", newerBuildImage(false, "", built), ""},
		{"both dates", newerBuildImage(true, pushed, built), "newer build of 11.4 (pushed 2026-11-03; running image built 2026-08-01)"},
		{"only the push date", newerBuildImage(true, pushed, ""), "newer build of 11.4 (pushed 2026-11-03)"},
		{"only the build date", newerBuildImage(true, "", built), "newer build of 11.4 (running image built 2026-08-01)"},
		{"no dates", newerBuildImage(true, "", ""), "newer build of 11.4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newerBuildText(tt.info); got != tt.want {
				t.Errorf("newerBuildText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewerBuild_EveryFormat(t *testing.T) {
	results := []models.ImageInfo{newerBuildImage(true, "2026-11-03T01:02:03Z", "2026-08-01T10:20:30Z")}
	formats := []struct {
		name     string
		generate func([]models.ImageInfo) string
	}{
		{"text", Generate},
		{"markdown", GenerateMarkdown},
		{"html", GenerateHTML},
	}
	for _, f := range formats {
		t.Run(f.name, func(t *testing.T) {
			out := f.generate(results)
			for _, want := range []string{"🔄", "newer build of 11.4", "2026-11-03", "Newer builds"} {
				if !strings.Contains(out, want) {
					t.Errorf("%s report is missing %q", f.name, want)
				}
			}
		})
	}
}

func TestNewerBuild_NoSummaryLineWhenThereAreNone(t *testing.T) {
	results := []models.ImageInfo{newerBuildImage(false, "", "")}
	for name, out := range map[string]string{"text": Generate(results), "markdown": GenerateMarkdown(results)} {
		if strings.Contains(out, "Newer builds") {
			t.Errorf("%s report has a newer builds line with none", name)
		}
	}
}

func TestNewerBuild_DoesntChangeVersionStatus(t *testing.T) {
	info := newerBuildImage(true, "", "")
	info.RecommendedVersion = "11.4"
	without := info
	without.NewerBuildAvailable = false
	if ClassifyImage(info) != ClassifyImage(without) {
		t.Error("a newer build changed the version status")
	}
}

func TestNewerBuild_JSONIsAdditive(t *testing.T) {
	results := []models.ImageInfo{newerBuildImage(true, "2026-11-03T01:02:03Z", "2026-08-01T10:20:30Z"), newerBuildImage(false, "", "")}
	var parsed struct {
		NewerBuildCount int              `json:"newer_build_count"`
		Images          []map[string]any `json:"images"`
	}
	if err := json.Unmarshal([]byte(GenerateJSON(results)), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.NewerBuildCount != 1 {
		t.Errorf("newer_build_count = %d, want 1", parsed.NewerBuildCount)
	}
	if parsed.Images[0]["newer_build_available"] != true || parsed.Images[0]["newer_build_date"] != "2026-11-03T01:02:03Z" || parsed.Images[0]["running_build_date"] != "2026-08-01T10:20:30Z" {
		t.Errorf("image = %v", parsed.Images[0])
	}
	for _, field := range []string{"newer_build_available", "newer_build_date", "running_build_date"} {
		if _, ok := parsed.Images[1][field]; ok {
			t.Errorf("%s is present with nothing to say, want it omitted", field)
		}
	}
}
