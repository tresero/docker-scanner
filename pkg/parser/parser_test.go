package parser

import (
	"docker-scanner/pkg/models"
	"errors"
	"fmt"
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

	images, err := Parse(project, nil)
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

	images, err := Parse(project, nil)
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

	images, err := Parse(project, nil)
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

	images, err := Parse(project, nil)
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

	images, err := Parse(project, nil)
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

	images, err := Parse(project, nil)
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

	images, err := Parse(project, nil)
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

	images, err := Parse(project, nil)
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

	_, err := Parse(project, nil)
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

// fakeDocker answers docker CLI calls by format string, so no test needs a real
// Docker daemon. A format with no answer behaves like a failed command.
func fakeDocker(t *testing.T, answers map[string]string) {
	t.Helper()
	original := dockerOutput
	t.Cleanup(func() { dockerOutput = original })
	dockerOutput = func(args ...string) ([]byte, error) {
		for i, arg := range args {
			if arg == "--format" && i+1 < len(args) {
				if answer, ok := answers[args[i+1]]; ok {
					return []byte(answer + "\n"), nil
				}
			}
		}
		return nil, errors.New("exit status 1")
	}
}

func TestGetRunningVersion(t *testing.T) {
	const (
		running  = "{{.State.Running}}"
		image    = "{{.Config.Image}}"
		imageID  = "{{index .Image}}"
		repoTags = "{{index .RepoTags 0}}"
		version  = `{{index .Config.Labels "org.opencontainers.image.version"}}`
		refName  = `{{index .Config.Labels "org.opencontainers.image.ref.name"}}`
	)
	tests := []struct {
		name    string
		answers map[string]string
		want    string
	}{
		{"nonexistent container", nil, ""},
		{"stopped container", map[string]string{running: "false"}, ""},
		{"image inspect fails", map[string]string{running: "true"}, "unknown"},
		{"pinned tag is returned", map[string]string{running: "true", image: "redis:8.10.3-alpine"}, "8.10.3-alpine"},
		{"latest uses the version label", map[string]string{running: "true", image: "app:latest", version: "2.3.4"}, "2.3.4"},
		{"no tag uses the version label", map[string]string{running: "true", image: "app", version: "2.3.4"}, "2.3.4"},
		{"latest with no label is unknown", map[string]string{running: "true", image: "app:latest", version: ""}, "unknown"},
		{"latest with no value is unknown", map[string]string{running: "true", image: "app:latest", version: "<no value>"}, "unknown"},
		{"failed label lookup is unknown", map[string]string{running: "true", image: "app:latest"}, "unknown"},
		{"a base image's version label is unknown", map[string]string{running: "true", image: "app:latest", version: "24.04", refName: "ubuntu"}, "unknown"},
		{"digest image resolves through repo tags", map[string]string{running: "true", image: "sha256:abc", imageID: "sha256:def", repoTags: "app:1.2.3"}, "1.2.3"},
		{"digest image with no id is unknown", map[string]string{running: "true", image: "sha256:abc"}, "unknown"},
		{"digest image with no repo tags is unknown", map[string]string{running: "true", image: "sha256:abc", imageID: "sha256:def"}, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeDocker(t, tt.answers)
			if got := GetRunningVersion("app"); got != tt.want {
				t.Errorf("GetRunningVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetRunningImageDetails(t *testing.T) {
	const (
		imageID     = "{{.Image}}"
		repoDigests = "{{json .RepoDigests}}"
		version     = `{{index .Config.Labels "org.opencontainers.image.version"}}`
		refName     = `{{index .Config.Labels "org.opencontainers.image.ref.name"}}`
	)
	tests := []struct {
		name        string
		answers     map[string]string
		wantDigests []string
		wantLabel   string
	}{
		{"nothing answers", nil, nil, ""},
		{
			"one digest and no label",
			map[string]string{imageID: "sha256:id", repoDigests: `["valkey/valkey@sha256:abc"]`},
			[]string{"sha256:abc"}, "",
		},
		{
			"several digests",
			map[string]string{imageID: "sha256:id", repoDigests: `["a/b@sha256:one","docker.n8n.io/a/b@sha256:two"]`},
			[]string{"sha256:one", "sha256:two"}, "",
		},
		{
			"no digests for a locally built image",
			map[string]string{imageID: "sha256:id", repoDigests: `[]`},
			nil, "",
		},
		{
			"an unparseable digest list is ignored",
			map[string]string{imageID: "sha256:id", repoDigests: `not json`},
			nil, "",
		},
		{
			"a malformed digest entry is skipped",
			map[string]string{imageID: "sha256:id", repoDigests: `["no-at-sign","a/b@sha256:ok"]`},
			[]string{"sha256:ok"}, "",
		},
		{"a version label", map[string]string{version: "11.4.9"}, nil, "11.4.9"},
		{"no value label", map[string]string{version: "<no value>"}, nil, ""},
		{"a base image's label is ignored", map[string]string{version: "24.04", refName: "ubuntu"}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeDocker(t, tt.answers)
			digests, label := GetRunningImageDetails("app")
			if fmt.Sprint(digests) != fmt.Sprint(tt.wantDigests) {
				t.Errorf("digests = %v, want %v", digests, tt.wantDigests)
			}
			if label != tt.wantLabel {
				t.Errorf("label = %q, want %q", label, tt.wantLabel)
			}
		})
	}
}

func TestGetRunningImageBuildDate(t *testing.T) {
	const (
		imageID = "{{.Image}}"
		created = "{{.Created}}"
	)
	tests := []struct {
		name    string
		answers map[string]string
		want    string
	}{
		{"nothing answers", nil, ""},
		{"no image inspect", map[string]string{imageID: "sha256:id"}, ""},
		{"nanoseconds are dropped", map[string]string{imageID: "sha256:id", created: "2026-08-01T10:20:30.123456789Z"}, "2026-08-01T10:20:30Z"},
		{"a time zone becomes UTC", map[string]string{imageID: "sha256:id", created: "2026-08-01T10:20:30+02:00"}, "2026-08-01T08:20:30Z"},
		{"an unreadable date is empty", map[string]string{imageID: "sha256:id", created: "yesterday"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeDocker(t, tt.answers)
			if got := GetRunningImageBuildDate("app"); got != tt.want {
				t.Errorf("GetRunningImageBuildDate = %q, want %q", got, tt.want)
			}
		})
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

			images, err := Parse(project, nil)
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