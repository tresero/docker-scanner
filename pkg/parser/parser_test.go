package parser

import (
	"docker-scanner/pkg/models"
	"os"
	"path/filepath"
	"testing"
)

func writeCompose(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParse_ExtractsImages(t *testing.T) {
	dir := t.TempDir()
	composePath := writeCompose(t, dir, `
services:
  web:
    image: nginx:1.25
  db:
    image: postgres:15-alpine
`)

	project := models.Project{
		Name:         "test",
		Path:         dir,
		ComposeFiles: []string{composePath},
	}

	images, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
}

func TestParse_DetectsLatest(t *testing.T) {
	dir := t.TempDir()
	composePath := writeCompose(t, dir, `
services:
  app:
    image: myapp:latest
`)

	project := models.Project{
		Name:         "test",
		Path:         dir,
		ComposeFiles: []string{composePath},
	}

	images, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if !images[0].UsesLatest {
		t.Error("expected UsesLatest to be true")
	}
}

func TestParse_ImplicitLatest(t *testing.T) {
	dir := t.TempDir()
	composePath := writeCompose(t, dir, `
services:
  app:
    image: myapp
`)

	project := models.Project{
		Name:         "test",
		Path:         dir,
		ComposeFiles: []string{composePath},
	}

	images, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	if !images[0].UsesLatest {
		t.Error("expected implicit latest to be detected")
	}
	if images[0].Image.Tag != "latest" {
		t.Errorf("expected tag 'latest', got %q", images[0].Image.Tag)
	}
}

func TestParse_DockerHubRegistry(t *testing.T) {
	dir := t.TempDir()
	composePath := writeCompose(t, dir, `
services:
  db:
    image: postgres:15
`)

	project := models.Project{
		Name:         "test",
		Path:         dir,
		ComposeFiles: []string{composePath},
	}

	images, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	if images[0].Image.Registry != "docker.io" {
		t.Errorf("expected registry 'docker.io', got %q", images[0].Image.Registry)
	}
	if images[0].Image.Name != "postgres" {
		t.Errorf("expected name 'postgres', got %q", images[0].Image.Name)
	}
	if images[0].Image.Tag != "15" {
		t.Errorf("expected tag '15', got %q", images[0].Image.Tag)
	}
}

func TestParse_GHCRRegistry(t *testing.T) {
	dir := t.TempDir()
	composePath := writeCompose(t, dir, `
services:
  app:
    image: ghcr.io/owner/repo:v1.2.3
`)

	project := models.Project{
		Name:         "test",
		Path:         dir,
		ComposeFiles: []string{composePath},
	}

	images, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	if images[0].Image.Registry != "ghcr.io" {
		t.Errorf("expected registry 'ghcr.io', got %q", images[0].Image.Registry)
	}
	if images[0].Image.Name != "owner/repo" {
		t.Errorf("expected name 'owner/repo', got %q", images[0].Image.Name)
	}
	if images[0].Image.Tag != "v1.2.3" {
		t.Errorf("expected tag 'v1.2.3', got %q", images[0].Image.Tag)
	}
}

func TestParse_LSCRRegistry(t *testing.T) {
	dir := t.TempDir()
	composePath := writeCompose(t, dir, `
services:
  sonarr:
    image: lscr.io/linuxserver/sonarr:latest
`)

	project := models.Project{
		Name:         "test",
		Path:         dir,
		ComposeFiles: []string{composePath},
	}

	images, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	if images[0].Image.Registry != "lscr.io" {
		t.Errorf("expected registry 'lscr.io', got %q", images[0].Image.Registry)
	}
	if images[0].Image.Name != "linuxserver/sonarr" {
		t.Errorf("expected name 'linuxserver/sonarr', got %q", images[0].Image.Name)
	}
}

func TestParse_SkipsServicesWithoutImage(t *testing.T) {
	dir := t.TempDir()
	composePath := writeCompose(t, dir, `
services:
  web:
    image: nginx:latest
  builder:
    build: ./app
`)

	project := models.Project{
		Name:         "test",
		Path:         dir,
		ComposeFiles: []string{composePath},
	}

	images, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	if len(images) != 1 {
		t.Errorf("expected 1 image (skip build-only), got %d", len(images))
	}
}

func TestParse_SetsProjectAndService(t *testing.T) {
	dir := t.TempDir()
	composePath := writeCompose(t, dir, `
services:
  caddy:
    image: ghcr.io/serfriz/caddy-cloudflare:latest
`)

	project := models.Project{
		Name:         "myproject",
		Path:         dir,
		ComposeFiles: []string{composePath},
	}

	images, err := Parse(project)
	if err != nil {
		t.Fatal(err)
	}

	if images[0].Image.Project != "myproject" {
		t.Errorf("expected project 'myproject', got %q", images[0].Image.Project)
	}
	if images[0].Image.Service != "caddy" {
		t.Errorf("expected service 'caddy', got %q", images[0].Image.Service)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	composePath := writeCompose(t, dir, `not: [valid: yaml: {{`)

	project := models.Project{
		Name:         "test",
		Path:         dir,
		ComposeFiles: []string{composePath},
	}

	_, err := Parse(project)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestGetRawEnvironment_ListFormat(t *testing.T) {
	dir := t.TempDir()
	path := writeCompose(t, dir, `
services:
  app:
    image: myapp
    environment:
      - FOO=bar
      - BAZ=${BAZ}
`)

	envVars, err := GetRawEnvironment(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(envVars["app"]) != 2 {
		t.Errorf("expected 2 env vars, got %d", len(envVars["app"]))
	}
}

func TestGetRawEnvironment_MapFormat(t *testing.T) {
	dir := t.TempDir()
	path := writeCompose(t, dir, `
services:
  app:
    image: myapp
    environment:
      FOO: bar
      BAZ: baz
`)

	envVars, err := GetRawEnvironment(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(envVars["app"]) != 2 {
		t.Errorf("expected 2 env vars, got %d", len(envVars["app"]))
	}
}

func TestGetRunningVersion_NonexistentContainer(t *testing.T) {
	version := GetRunningVersion("nonexistent-container-xyz")
	if version != "" {
		t.Errorf("expected empty string for nonexistent container, got %q", version)
	}
}

// TestParse_FloatingTagsAreUnsafe pins down the rule: tags that don't look
// like a recognizable version are flagged as unsafe (UsesLatest=true).
// Compound tags like "16-alpine" are allowed because they're a common,
// intentional pinning pattern.
func TestParse_FloatingTagsAreUnsafe(t *testing.T) {
	cases := []struct {
		tag        string
		usesLatest bool
		why        string
	}{
		// Floating / unsafe tags
		{"latest", true, "the canonical floating tag"},
		{"main", true, "git branch name"},
		{"master", true, "git branch name"},
		{"dev", true, "channel name"},
		{"stable", true, "floating channel pointer"},
		{"nightly", true, "build channel"},
		{"edge", true, "build channel"},
		{"apache", true, "variant selector with no version"},
		{"fpm", true, "variant selector"},
		{"alpine", true, "OS variant with no version"},
		{"bookworm", true, "OS codename"},
		{"jammy", true, "OS codename"},

		// Pinned / safe tags - semver
		{"1", false, "major-only semver"},
		{"1.2", false, "major.minor semver"},
		{"1.2.3", false, "full semver"},
		{"v1.2.3", false, "v-prefixed semver"},
		{"1.2.3-rc1", false, "semver with pre-release"},
		{"1.2.3-rc.1", false, "semver with dotted pre-release"},
		{"1.2.3+build.5", false, "semver with build metadata"},

		// Pinned / safe tags - compound (version + variant)
		{"16-alpine", false, "major version with variant suffix"},
		{"8.5-fpm", false, "version with variant suffix"},
		{"1.2.3-bookworm", false, "full semver with OS variant"},
		{"v1.2.3-arm64", false, "v-prefixed semver with arch suffix"},

		// Pinned / safe tags - dates
		{"2024.05.01", false, "dotted date"},
		{"20240501", false, "compact date"},
		{"2024-05-01", false, "ISO date"},
		{"24.05", false, "year.month date (calver)"},
		{"2026.3.0", false, "calver with patch"},

		// Pinned / safe tags - git hash
		{"a1b2c3d", false, "7-char git hash"},
		{"abc12345", false, "8-char git hash"},
		{"deadbeefcafe1234", false, "long git hash"},
	}

	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			dir := t.TempDir()
			composePath := writeCompose(t, dir, `
services:
  app:
    image: myapp:`+c.tag+`
`)
			project := models.Project{
				Name:         "test",
				Path:         dir,
				ComposeFiles: []string{composePath},
			}

			images, err := Parse(project)
			if err != nil {
				t.Fatal(err)
			}
			if len(images) != 1 {
				t.Fatalf("expected 1 image, got %d", len(images))
			}
			got := images[0].UsesLatest
			if got != c.usesLatest {
				t.Errorf("tag %q: UsesLatest=%v, want %v (%s)", c.tag, got, c.usesLatest, c.why)
			}
		})
	}
}