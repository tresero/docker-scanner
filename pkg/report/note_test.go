package report

import (
	"docker-scanner/pkg/models"
	"encoding/json"
	"strings"
	"testing"
)

const waitingNote = "newest is 2.11.9, pushed 1 hour ago; waiting for -safe-days 3"

func noteFixture() []models.ImageInfo {
	return []models.ImageInfo{{
		Image:              models.Image{Project: "web", Service: "caddy", Registry: "docker.io", Name: "caddy", Tag: "2.11.7"},
		File:               "web/docker-compose.yml",
		RunningVersion:     "2.11.7",
		RecommendationNote: waitingNote,
	}}
}

func TestReports_ShowRecommendationNote(t *testing.T) {
	tests := []struct {
		name     string
		generate func([]models.ImageInfo) string
	}{
		{"text", Generate},
		{"markdown", GenerateMarkdown},
		{"html", GenerateHTML},
		{"json", GenerateJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if out := tt.generate(noteFixture()); !strings.Contains(out, waitingNote) {
				t.Errorf("%s report does not contain the note:\n%s", tt.name, out)
			}
		})
	}
}

// TestGenerateJSON_ExistingFieldsUnchanged protects the JSON consumer: every
// existing key keeps its name, and new fields stay out when they're empty.
func TestGenerateJSON_ExistingFieldsUnchanged(t *testing.T) {
	info := noteFixture()[0]
	info.RecommendationNote = ""
	info.AvailableVersions = []string{"2.11.9"}
	info.RecommendedVersion = "2.11.9"
	info.RecommendedAge = "5 days"
	info.MajorVersionJump = true
	info.IsDowngrade = true
	info.UsesLatest = true

	var got struct {
		Images []map[string]any `json:"images"`
	}
	if err := json.Unmarshal([]byte(GenerateJSON([]models.ImageInfo{info})), &got); err != nil {
		t.Fatal(err)
	}
	image := got.Images[0]
	for _, key := range []string{
		"image", "file", "uses_latest", "running_version", "available_versions",
		"recommended_version", "recommended_age", "major_version_jump", "is_downgrade",
	} {
		if _, ok := image[key]; !ok {
			t.Errorf("JSON is missing existing key %q", key)
		}
	}
	if _, ok := image["recommendation_note"]; ok {
		t.Error("empty recommendation_note should be omitted")
	}
}

func TestReports_ShowNewerMajor(t *testing.T) {
	info := noteFixture()[0]
	info.RecommendationNote = ""
	info.RecommendedVersion = "11.4.9"
	info.NewerMajorVersion = "13.0.2"

	tests := []struct {
		name     string
		generate func([]models.ImageInfo) string
	}{
		{"text", Generate},
		{"markdown", GenerateMarkdown},
		{"html", GenerateHTML},
		{"json", GenerateJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if out := tt.generate([]models.ImageInfo{info}); !strings.Contains(out, "13.0.2") {
				t.Errorf("%s report does not mention the newer major:\n%s", tt.name, out)
			}
		})
	}
}

// TestClassifyImage_SpellingIsNotAnUpgrade comes from a real scan: bentopdf was
// flagged as an upgrade from 2.8.11 to v2.8.11, which is the same version.
func TestClassifyImage_SpellingIsNotAnUpgrade(t *testing.T) {
	tests := []struct {
		name        string
		tag         string
		recommended string
		want        ImageStatus
	}{
		{"same tag", "2.8.11", "2.8.11", StatusGood},
		{"v added", "2.8.11", "v2.8.11", StatusGood},
		{"v removed", "v2.8.11", "2.8.11", StatusGood},
		{"no recommendation", "2.8.11", "", StatusGood},
		{"higher version", "2.8.11", "2.8.12", StatusUpgrade},
		{"higher version with v added", "2.8.11", "v2.8.12", StatusUpgrade},
		{"variant change is not just spelling", "2.8.11", "v2.8.11-alpine", StatusUpgrade},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyImage(models.ImageInfo{
				Image:              models.Image{Tag: tt.tag},
				RecommendedVersion: tt.recommended,
			})
			if got != tt.want {
				t.Errorf("ClassifyImage = %v, want %v", got, tt.want)
			}
		})
	}
}
