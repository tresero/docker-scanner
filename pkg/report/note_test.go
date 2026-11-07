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

// TestClassifyImage_UnverifiedIsNotUpToDate comes from a real scan: an image the
// scanner never managed to check showed a green tick.
func TestClassifyImage_UnverifiedIsNotUpToDate(t *testing.T) {
	tests := []struct {
		name string
		info models.ImageInfo
		want ImageStatus
	}{
		{"unverified pinned image", models.ImageInfo{Image: models.Image{Tag: "3.0.1"}, Unverified: true}, StatusUnverified},
		{"floating tag stays floating", models.ImageInfo{Image: models.Image{Tag: "latest"}, UsesLatest: true, Unverified: true}, StatusFloating},
		{"verified pinned image", models.ImageInfo{Image: models.Image{Tag: "3.0.1"}}, StatusGood},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyImage(tt.info); got != tt.want {
				t.Errorf("ClassifyImage = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReports_ShowUnverifiedStatus(t *testing.T) {
	unverified := models.ImageInfo{
		Image:              models.Image{Project: "n8n", Service: "n8n", Registry: "docker.n8n.io", Name: "n8nio/n8n", Tag: "3.0.1"},
		RunningVersion:     "3.0.1",
		RecommendationNote: "couldn't check for updates",
		Unverified:         true,
	}
	tests := []struct {
		name     string
		generate func([]models.ImageInfo) string
		want     []string
		notWant  []string
	}{
		{"text", Generate, []string{"❓", "Not verified:        1"}, []string{"Up to date:          1"}},
		{"markdown", GenerateMarkdown, []string{":grey_question:", "| Not verified | **1** :grey_question: |"}, []string{"| Up to date | 1"}},
		{"html", GenerateHTML, []string{"❓", "Not verified"}, nil},
		{"json", GenerateJSON, []string{`"unverified": true`}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := tt.generate([]models.ImageInfo{unverified})
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("%s report is missing %q:\n%s", tt.name, want, out)
				}
			}
			for _, bad := range tt.notWant {
				if strings.Contains(out, bad) {
					t.Errorf("%s report should not contain %q", tt.name, bad)
				}
			}
		})
	}
}

// TestGenerateJSON_UnverifiedLeavesExistingCountsAlone protects the JSON consumer.
func TestGenerateJSON_UnverifiedLeavesExistingCountsAlone(t *testing.T) {
	var got struct {
		LatestCount    int `json:"latest_count"`
		UpgradeCount   int `json:"upgrade_count"`
		DowngradeCount int `json:"downgrade_count"`
		ImageCount     int `json:"image_count"`
	}
	out := GenerateJSON([]models.ImageInfo{{Image: models.Image{Tag: "3.0.1"}, Unverified: true}})
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.LatestCount != 0 || got.UpgradeCount != 0 || got.DowngradeCount != 0 || got.ImageCount != 1 {
		t.Errorf("counts = %+v", got)
	}
}
