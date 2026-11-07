package report

import (
	"docker-scanner/pkg/models"
	"encoding/json"
	"strings"
	"testing"
)

func baseImage(state, recommended string) models.ImageInfo {
	return models.ImageInfo{
		Image:              models.Image{Registry: "docker.io", Name: "python", Tag: "3.12-slim", Service: "app", Project: "web"},
		File:               "/srv/web/docker-compose.yml",
		Dockerfile:         "/srv/web/Dockerfile",
		LocalImage:         "myapp:production",
		ContainerState:     state,
		RecommendedVersion: recommended,
	}
}

func TestBaseImageText(t *testing.T) {
	newer := baseImage("running", "3.12-slim")
	newer.NewerBuildAvailable = true
	noLocal := baseImage("running", "3.12-slim")
	noLocal.LocalImage = ""
	tests := []struct {
		name string
		info models.ImageInfo
		want string
	}{
		{"a pulled image", models.ImageInfo{Image: models.Image{Tag: "1.0"}}, ""},
		{"current base", baseImage("running", "3.12-slim"), "base image of myapp:production, from /srv/web/Dockerfile"},
		{"no local name", noLocal, "base image, from /srv/web/Dockerfile"},
		{"a newer version", baseImage("running", "3.12.5-slim"), "base image of myapp:production, from /srv/web/Dockerfile; rebuild from the newer base to upgrade"},
		{"a newer build of the same tag", newer, "base image of myapp:production, from /srv/web/Dockerfile; rebuild from the newer base to upgrade"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := baseImageText(tt.info); got != tt.want {
				t.Errorf("baseImageText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBaseImage_RunningColumn(t *testing.T) {
	standalone := baseImage("", "")
	standalone.Image.Service = ""
	tests := []struct {
		name string
		info models.ImageInfo
		want string
	}{
		{"a running built service", baseImage("running", ""), "running"},
		{"a crashed built service", func() models.ImageInfo { r := baseImage("exited", ""); code := 1; r.ExitCode = &code; return r }(), "exited (1)"},
		{"a built service never created", baseImage("not created", ""), "not created"},
		{"a standalone Dockerfile isn't a service", standalone, "not a service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runningLabel(tt.info); got != tt.want {
				t.Errorf("runningLabel = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBaseImage_EveryFormat(t *testing.T) {
	results := []models.ImageInfo{baseImage("running", "3.12.5-slim")}
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
			for _, want := range []string{"🏗️", "rebuild from the newer base", "myapp:production"} {
				if !strings.Contains(out, want) {
					t.Errorf("%s report is missing %q", f.name, want)
				}
			}
		})
	}
}

func TestBaseImage_JSONIsAdditive(t *testing.T) {
	var parsed struct {
		Images []map[string]any `json:"images"`
	}
	results := []models.ImageInfo{baseImage("running", "3.12.5-slim"), {Image: models.Image{Name: "nginx", Tag: "1.25"}}}
	if err := json.Unmarshal([]byte(GenerateJSON(results)), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Images[0]["dockerfile"] != "/srv/web/Dockerfile" || parsed.Images[0]["local_image"] != "myapp:production" {
		t.Errorf("image = %v", parsed.Images[0])
	}
	if _, ok := parsed.Images[0]["SkipLookup"]; ok {
		t.Error("SkipLookup leaked into JSON")
	}
	for _, field := range []string{"dockerfile", "local_image"} {
		if _, ok := parsed.Images[1][field]; ok {
			t.Errorf("%s is present on a pulled image, want it omitted", field)
		}
	}
}
