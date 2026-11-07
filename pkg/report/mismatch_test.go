package report

import (
	"docker-scanner/pkg/models"
	"encoding/json"
	"strings"
	"testing"
)

func mismatchImage(tag, running, resolved string, mismatch, downgrade bool) models.ImageInfo {
	return models.ImageInfo{
		Image:                  models.Image{Registry: "docker.io", Name: "app", Tag: tag, Service: "app", Project: "web"},
		RunningVersion:         running,
		ResolvedRunningVersion: resolved,
		ContainerState:         "running",
		TagMismatch:            mismatch,
		TagMismatchDowngrade:   downgrade,
	}
}

func TestTagMismatchText(t *testing.T) {
	tests := []struct {
		name string
		info models.ImageInfo
		want string
	}{
		{"no mismatch", mismatchImage("1.2.3", "1.2.3", "", false, false), ""},
		{"compose ahead", mismatchImage("1.2.4", "1.2.3", "", true, false), "compose 1.2.4, running 1.2.3: next up -d will change this"},
		{"compose behind", mismatchImage("1.2.3", "1.2.4", "", true, true), "compose 1.2.3, running 1.2.4: next up -d will downgrade this"},
		{"names the real version when it's known", mismatchImage("16.14-alpine", "16-alpine", "16.15-alpine", true, true), "compose 16.14-alpine, running 16.15-alpine: next up -d will downgrade this"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tagMismatchText(tt.info); got != tt.want {
				t.Errorf("tagMismatchText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTagMismatch_EveryFormat(t *testing.T) {
	results := []models.ImageInfo{mismatchImage("1.2.4", "1.2.3", "", true, false), mismatchImage("1.2.3", "1.2.4", "", true, true)}
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
			for _, want := range []string{"🔀", "next up -d will change this", "next up -d will downgrade this", "Tag mismatches"} {
				if !strings.Contains(out, want) {
					t.Errorf("%s report is missing %q", f.name, want)
				}
			}
		})
	}
}

func TestTagMismatch_NoSummaryLineWhenThereAreNone(t *testing.T) {
	results := []models.ImageInfo{mismatchImage("1.2.3", "1.2.3", "", false, false)}
	for name, out := range map[string]string{"text": Generate(results), "markdown": GenerateMarkdown(results)} {
		if strings.Contains(out, "Tag mismatches") {
			t.Errorf("%s report has a mismatch line with none", name)
		}
	}
}

func TestTagMismatch_JSONIsAdditive(t *testing.T) {
	results := []models.ImageInfo{mismatchImage("1.2.3", "1.2.4", "", true, true), mismatchImage("1.2.3", "1.2.3", "", false, false)}
	var parsed struct {
		TagMismatchCount int              `json:"tag_mismatch_count"`
		Images           []map[string]any `json:"images"`
	}
	if err := json.Unmarshal([]byte(GenerateJSON(results)), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.TagMismatchCount != 1 {
		t.Errorf("tag_mismatch_count = %d, want 1", parsed.TagMismatchCount)
	}
	if parsed.Images[0]["tag_mismatch"] != true || parsed.Images[0]["tag_mismatch_downgrade"] != true {
		t.Errorf("image = %v", parsed.Images[0])
	}
	for _, field := range []string{"tag_mismatch", "tag_mismatch_downgrade"} {
		if _, ok := parsed.Images[1][field]; ok {
			t.Errorf("%s is present with no mismatch, want it omitted", field)
		}
	}
}

func TestTagMismatch_DoesntChangeVersionStatus(t *testing.T) {
	with := mismatchImage("1.2.4", "1.2.3", "", true, false)
	without := with
	without.TagMismatch = false
	if ClassifyImage(with) != ClassifyImage(without) {
		t.Error("a mismatch changed the version status")
	}
}
