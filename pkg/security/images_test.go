package security

import (
	"docker-scanner/pkg/models"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestCheckImages_EachIssueOnce comes from a real scan: a password in one
// service was reported on every service in the compose file, so security_count
// counted it once per service.
func TestCheckImages_EachIssueOnce(t *testing.T) {
	tests := []struct {
		name    string
		compose string
		envFile bool
		rows    []string            // services that have a row, as the parser would give them
		want    map[string][]string // issue types per service row
	}{
		{
			name:    "a secret goes only to its own service",
			compose: "services:\n  web:\n    image: nginx:1.27.3\n  db:\n    image: postgres:16.14\n    environment:\n      - POSTGRES_PASSWORD=hunter2\n",
			rows:    []string{"web", "db"},
			want:    map[string][]string{"db": {"hardcoded_secret"}},
		},
		{
			name:    "two secrets in one service are both kept",
			compose: "services:\n  web:\n    image: nginx:1.27.3\n  db:\n    image: postgres:16.14\n    environment:\n      - POSTGRES_PASSWORD=hunter2\n      - API_KEY=abc123\n",
			rows:    []string{"web", "db"},
			want:    map[string][]string{"db": {"hardcoded_secret", "hardcoded_secret"}},
		},
		{
			name:    "an issue about the whole file is reported once, on the first service by name",
			compose: "services:\n  web:\n    image: nginx:1.27.3\n    environment:\n      - HOST=${HOST}\n  app:\n    image: nginx:1.27.3\n",
			rows:    []string{"web", "app"},
			want:    map[string][]string{"app": {"missing_envfile"}},
		},
		{
			name:    "a secret in a service with no row goes to the first service by name",
			compose: "services:\n  web:\n    image: nginx:1.27.3\n  app:\n    image: nginx:1.27.3\n  helper:\n    environment:\n      - DB_PASSWORD=hunter2\n",
			rows:    []string{"web", "app"},
			want:    map[string][]string{"app": {"hardcoded_secret"}},
		},
		{
			name:    "a file with nothing wrong adds nothing",
			compose: "services:\n  web:\n    image: nginx:1.27.3\n  db:\n    image: postgres:16.14\n    environment:\n      - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}\n",
			envFile: true,
			rows:    []string{"web", "db"},
			want:    map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeTestCompose(t, dir, tt.compose)
			if tt.envFile {
				if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("POSTGRES_PASSWORD=x\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			var images []models.ImageInfo
			for _, service := range tt.rows {
				images = append(images, models.ImageInfo{File: path, Image: models.Image{Service: service}})
			}

			CheckImages(DefaultCheckers(), images)

			got := map[string][]string{}
			for _, image := range images {
				for _, issue := range image.SecurityIssues {
					got[image.Image.Service] = append(got[image.Image.Service], issue.Type)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("issues by service = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCheckImages_KeepsFilesApart checks that two compose files in one project
// each keep their own issues.
func TestCheckImages_KeepsFilesApart(t *testing.T) {
	first := writeTestCompose(t, t.TempDir(), "services:\n  db:\n    image: postgres:16.14\n    environment:\n      - POSTGRES_PASSWORD=hunter2\n")
	second := writeTestCompose(t, t.TempDir(), "services:\n  db:\n    image: postgres:16.14\n")
	images := []models.ImageInfo{
		{File: first, Image: models.Image{Service: "db"}},
		{File: second, Image: models.Image{Service: "db"}},
	}

	CheckImages(DefaultCheckers(), images)

	if len(images[0].SecurityIssues) != 1 || len(images[1].SecurityIssues) != 0 {
		t.Errorf("issues = %d and %d, want 1 in the first file and 0 in the second", len(images[0].SecurityIssues), len(images[1].SecurityIssues))
	}
}

// TestCheckImages_SkipsStandaloneDockerfiles checks that a row with no service,
// a Dockerfile with no compose file, isn't checked.
func TestCheckImages_SkipsStandaloneDockerfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(path, []byte("FROM alpine:3.20\nENV DB_PASSWORD=hunter2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	images := []models.ImageInfo{{File: path, Dockerfile: path}}

	CheckImages(DefaultCheckers(), images)

	if len(images[0].SecurityIssues) != 0 {
		t.Errorf("got %v, want no issues", images[0].SecurityIssues)
	}
}
