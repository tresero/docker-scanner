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