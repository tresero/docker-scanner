package report

import (
	"docker-scanner/pkg/models"
	"encoding/json"
	"testing"
)

// TestGenerateJSON_Status checks that each image's status is the verdict that
// colours its row in the other reports, so a script reading the JSON needn't
// work it out again.
func TestGenerateJSON_Status(t *testing.T) {
	tests := []struct {
		name string
		info models.ImageInfo
		want string
	}{
		{"pinned and up to date", models.ImageInfo{Image: models.Image{Tag: "2.7.1"}, RunningVersion: "2.7.1", RecommendedVersion: "2.7.1"}, "good"},
		{"a line pin already running the recommendation", models.ImageInfo{Image: models.Image{Tag: "11.4"}, RunningVersion: "11.4", ResolvedRunningVersion: "11.4.13", RecommendedVersion: "11.4.13"}, "good"},
		{"a v prefix is the same version", models.ImageInfo{Image: models.Image{Tag: "v2.7.0"}, RecommendedVersion: "2.7.0"}, "good"},
		{"nothing newer to recommend", models.ImageInfo{Image: models.Image{Tag: "8.10.3"}, RunningVersion: "8.10.3"}, "good"},
		{"a container problem leaves the version status alone", models.ImageInfo{Image: models.Image{Tag: "2.7.1"}, RecommendedVersion: "2.7.1", ContainerState: "unhealthy"}, "good"},
		{"a newer build leaves the version status alone", models.ImageInfo{Image: models.Image{Tag: "8.10.3"}, NewerBuildAvailable: true}, "good"},
		{"a tag mismatch where compose already has the recommendation", models.ImageInfo{Image: models.Image{Tag: "1.2.4"}, RunningVersion: "1.2.3", RecommendedVersion: "1.2.4", TagMismatch: true}, "good"},
		{"a tag mismatch with a newer version recommended", models.ImageInfo{Image: models.Image{Tag: "1.2.4"}, RunningVersion: "1.2.3", RecommendedVersion: "1.2.5", TagMismatch: true}, "upgrade"},
		{"upgrade available", models.ImageInfo{Image: models.Image{Tag: "3.0.1"}, RunningVersion: "3.0.1", RecommendedVersion: "3.1.0"}, "upgrade"},
		{"floating tag", models.ImageInfo{Image: models.Image{Tag: "latest"}, UsesLatest: true, RecommendedVersion: "18.4"}, "floating"},
		{"not verified", models.ImageInfo{Image: models.Image{Tag: "3.0.1"}, Unverified: true}, "unverified"},
		{"downgrade", models.ImageInfo{Image: models.Image{Tag: "2.0"}, IsDowngrade: true, RecommendedVersion: "1.9"}, "downgrade"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got struct {
				Images []struct {
					Status string `json:"status"`
				} `json:"images"`
			}
			if err := json.Unmarshal([]byte(GenerateJSON([]models.ImageInfo{tt.info})), &got); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if len(got.Images) != 1 {
				t.Fatalf("got %d images, want 1", len(got.Images))
			}
			if got.Images[0].Status != tt.want {
				t.Errorf("status = %q, want %q", got.Images[0].Status, tt.want)
			}
		})
	}
}

// TestGenerateJSON_StatusLeavesInputAlone checks that writing the JSON doesn't
// change the caller's results.
func TestGenerateJSON_StatusLeavesInputAlone(t *testing.T) {
	results := []models.ImageInfo{{Image: models.Image{Tag: "3.0.1"}, RecommendedVersion: "3.1.0"}}
	GenerateJSON(results)
	if results[0].Status != "" {
		t.Errorf("results[0].Status = %q after GenerateJSON, want it unchanged", results[0].Status)
	}
}
