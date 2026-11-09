package parser

import (
	"docker-scanner/pkg/models"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDockerfileBase(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		args           map[string]string
		want           string
		wantUnresolved []string
	}{
		{name: "one FROM", content: "FROM python:3.12-slim\nRUN true\n", want: "python:3.12-slim"},
		{name: "lowercase instruction", content: "from python:3.12\n", want: "python:3.12"},
		{name: "no FROM", content: "# nothing here\nRUN true\n", want: ""},
		{name: "an empty file", content: "", want: ""},
		{name: "comments and blank lines first", content: "# syntax=docker/dockerfile:1\n\n# base\nFROM alpine:3.20\n", want: "alpine:3.20"},
		{name: "a commented-out FROM is ignored", content: "FROM alpine:3.20\n# FROM alpine:3.19\n", want: "alpine:3.20"},
		{name: "a platform flag", content: "FROM --platform=linux/amd64 alpine:3.20\n", want: "alpine:3.20"},
		{name: "a line continuation", content: "FROM \\\n  alpine:3.20\n", want: "alpine:3.20"},
		{name: "windows line endings", content: "FROM alpine:3.20\r\nRUN true\r\n", want: "alpine:3.20"},

		{name: "the final stage wins", content: "FROM golang:1.22 AS build\nFROM alpine:3.20\n", want: "alpine:3.20"},
		{name: "a final stage built on an earlier one", content: "FROM python:3.12 AS base\nRUN true\nFROM base\n", want: "python:3.12"},
		{name: "aliases chain", content: "FROM python:3.12 AS a\nFROM a AS b\nFROM b\n", want: "python:3.12"},
		{name: "aliases are case-insensitive", content: "FROM python:3.12 as Base\nFROM base\n", want: "python:3.12"},
		{name: "scratch", content: "FROM golang:1.22 AS build\nFROM scratch\n", want: "scratch"},

		{name: "an ARG default", content: "ARG PY=3.12\nFROM python:${PY}-slim\n", want: "python:3.12-slim"},
		{name: "an ARG with no braces", content: "ARG PY=3.12\nFROM python:$PY\n", want: "python:3.12"},
		{name: "a quoted ARG default", content: "ARG PY=\"3.12\"\nFROM python:${PY}\n", want: "python:3.12"},
		{name: "a build arg overrides the default", content: "ARG PY=3.12\nFROM python:${PY}\n", args: map[string]string{"PY": "3.13"}, want: "python:3.13"},
		{name: "a build arg with no default in the file", content: "ARG PY\nFROM python:${PY}\n", args: map[string]string{"PY": "3.13"}, want: "python:3.13"},
		{name: "an inline default", content: "FROM python:${PY:-3.11}\n", want: "python:3.11"},
		{name: "an ARG built from another", content: "ARG MAJOR=3\nARG PY=${MAJOR}.12\nFROM python:${PY}\n", want: "python:3.12"},
		{name: "an undeclared build arg isn't visible", content: "FROM python:${PY}\n", args: map[string]string{"PY": "3.13"}, want: "python:${PY}", wantUnresolved: []string{"PY"}},
		{name: "an ARG with no value is unresolved", content: "ARG PY\nFROM python:${PY}\n", want: "python:${PY}", wantUnresolved: []string{"PY"}},
		{name: "an ARG after FROM doesn't apply to it", content: "FROM python:${PY}\nARG PY=3.12\n", want: "python:${PY}", wantUnresolved: []string{"PY"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, unresolved := dockerfileBase(tt.content, tt.args)
			if got != tt.want {
				t.Errorf("base = %q, want %q", got, tt.want)
			}
			if fmt.Sprint(unresolved) != fmt.Sprint(tt.wantUnresolved) {
				t.Errorf("unresolved = %v, want %v", unresolved, tt.wantUnresolved)
			}
		})
	}
}

// A build that adds no layers keeps its base's Created date, as seen on real builds,
// so that date doesn't say when it was built.
func TestParse_BuildDateIsOnlyKnownWithOwnLayers(t *testing.T) {
	tests := []struct {
		name       string
		dockerfile string
		want       bool
	}{
		{name: "a bare FROM", dockerfile: "FROM alpine:3.20\n", want: true},
		{name: "only metadata", dockerfile: "FROM alpine:3.20\nENV A=1\nCMD [\"sleep\",\"1\"]\nLABEL a=b\n", want: true},
		{name: "a RUN", dockerfile: "FROM alpine:3.20\nRUN true\n"},
		{name: "a COPY", dockerfile: "FROM alpine:3.20\nCOPY . /app\n"},
		{name: "an ADD", dockerfile: "FROM alpine:3.20\nADD a.tar /\n"},
		{name: "lowercase instructions", dockerfile: "from alpine:3.20\nrun true\n"},
		{name: "a RUN in an unrelated earlier stage doesn't count", dockerfile: "FROM golang:1.22 AS build\nRUN go build\nFROM alpine:3.20\nCMD [\"x\"]\n", want: true},
		{name: "a RUN in the stage the final one builds on counts", dockerfile: "FROM alpine:3.20 AS base\nRUN true\nFROM base\nCMD [\"x\"]\n"},
		{name: "a RUN before the last FROM of an alias chain counts", dockerfile: "FROM alpine:3.20 AS a\nRUN true\nFROM a AS b\nFROM b\n"},
		{name: "a metadata-only chain", dockerfile: "FROM alpine:3.20 AS a\nENV A=1\nFROM a\n", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			composePath := writeCompose(t, dir, "services:\n  app:\n    build: .\n")
			writeFile(t, filepath.Join(dir, "Dockerfile"), tt.dockerfile)
			images, err := Parse(models.Project{Name: "test", Path: dir, ComposeFiles: []string{composePath}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if images[0].NoOwnLayers != tt.want {
				t.Errorf("NoOwnLayers = %v, want %v", images[0].NoOwnLayers, tt.want)
			}
		})
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestParse_BuildServices(t *testing.T) {
	tests := []struct {
		name           string
		service        string
		files          map[string]string
		dotEnv         string
		wantRegistry   string
		wantName       string
		wantTag        string
		wantDockerfile string
		wantLocal      string
		wantFloating   bool
		wantUnverified bool
		wantSkip       bool
		wantUnresolved []string
	}{
		{
			name:         "a build string",
			service:      "build: ./app",
			files:        map[string]string{"app/Dockerfile": "FROM mediawiki:1.43\n"},
			wantRegistry: "docker.io", wantName: "mediawiki", wantTag: "1.43", wantDockerfile: "app/Dockerfile",
		},
		{
			name:         "context and dockerfile keys",
			service:      "build:\n      context: ./app\n      dockerfile: docker/Prod.Dockerfile",
			files:        map[string]string{"app/docker/Prod.Dockerfile": "FROM python:3.12-slim\n"},
			wantRegistry: "docker.io", wantName: "python", wantTag: "3.12-slim", wantDockerfile: "app/docker/Prod.Dockerfile",
		},
		{
			name:         "only a dockerfile key uses the compose folder",
			service:      "build:\n      dockerfile: Other.Dockerfile",
			files:        map[string]string{"Other.Dockerfile": "FROM alpine:3.20\n"},
			wantRegistry: "docker.io", wantName: "alpine", wantTag: "3.20", wantDockerfile: "Other.Dockerfile",
		},
		{
			name:         "image and build reports the base, not the local name",
			service:      "image: myapp:production\n    build: .",
			files:        map[string]string{"Dockerfile": "FROM mediawiki:1.43\n"},
			wantRegistry: "docker.io", wantName: "mediawiki", wantTag: "1.43", wantDockerfile: "Dockerfile", wantLocal: "myapp:production",
		},
		{
			name:         "a floating base is flagged",
			service:      "build: .",
			files:        map[string]string{"Dockerfile": "FROM wordpress:php8.4-apache\n"},
			wantRegistry: "docker.io", wantName: "wordpress", wantTag: "php8.4-apache", wantDockerfile: "Dockerfile", wantFloating: true,
		},
		{
			name:         "build args from a map",
			service:      "build:\n      context: .\n      args:\n        PY: \"3.13\"",
			files:        map[string]string{"Dockerfile": "ARG PY=3.12\nFROM python:${PY}\n"},
			wantRegistry: "docker.io", wantName: "python", wantTag: "3.13", wantDockerfile: "Dockerfile",
		},
		{
			name:         "build args from a list",
			service:      "build:\n      context: .\n      args:\n        - PY=3.13",
			files:        map[string]string{"Dockerfile": "ARG PY=3.12\nFROM python:${PY}\n"},
			wantRegistry: "docker.io", wantName: "python", wantTag: "3.13", wantDockerfile: "Dockerfile",
		},
		{
			name:         "build args use Compose variables",
			service:      "build:\n      context: .\n      args:\n        PY: ${PYTHON_VERSION}",
			files:        map[string]string{"Dockerfile": "ARG PY=3.12\nFROM python:${PY}\n"},
			dotEnv:       "PYTHON_VERSION=3.11\n",
			wantRegistry: "docker.io", wantName: "python", wantTag: "3.11", wantDockerfile: "Dockerfile",
		},
		{
			name:         "a list arg with no value comes from the environment",
			service:      "build:\n      context: .\n      args:\n        - PY",
			files:        map[string]string{"Dockerfile": "ARG PY=3.12\nFROM python:${PY}\n"},
			dotEnv:       "PY=3.10\n",
			wantRegistry: "docker.io", wantName: "python", wantTag: "3.10", wantDockerfile: "Dockerfile",
		},
		{
			name:         "an inline dockerfile",
			service:      "build:\n      context: .\n      dockerfile_inline: |\n        FROM alpine:3.20",
			wantRegistry: "docker.io", wantName: "alpine", wantTag: "3.20", wantDockerfile: "docker-compose.yml (inline)",
		},
		{
			name:         "a base on another registry",
			service:      "build: .",
			files:        map[string]string{"Dockerfile": "FROM ghcr.io/org/base:1.2.3\n"},
			wantRegistry: "ghcr.io", wantName: "org/base", wantTag: "1.2.3", wantDockerfile: "Dockerfile",
		},
		{
			name:         "an unresolved ARG",
			service:      "build: .",
			files:        map[string]string{"Dockerfile": "ARG PY\nFROM python:${PY}\n"},
			wantRegistry: "docker.io", wantName: "python", wantTag: "${PY}", wantDockerfile: "Dockerfile",
			wantUnverified: true, wantSkip: true, wantUnresolved: []string{"PY"},
		},
		{
			name:         "FROM scratch has nothing to check",
			service:      "build: .",
			files:        map[string]string{"Dockerfile": "FROM scratch\n"},
			wantRegistry: "local", wantName: "scratch", wantDockerfile: "Dockerfile", wantSkip: true,
		},
		{
			name:         "no FROM says so",
			service:      "image: myapp:1.0\n    build: .",
			files:        map[string]string{"Dockerfile": "RUN true\n"},
			wantRegistry: "local", wantName: "myapp:1.0", wantDockerfile: "Dockerfile", wantLocal: "myapp:1.0", wantUnverified: true, wantSkip: true,
		},
		{
			name:         "a missing Dockerfile says so",
			service:      "build: ./gone",
			wantRegistry: "local", wantName: "app", wantDockerfile: "gone/Dockerfile", wantUnverified: true, wantSkip: true,
		},
		{
			name:         "a remote build context can't be read",
			service:      "build: https://github.com/org/repo.git",
			wantRegistry: "local", wantName: "app", wantUnverified: true, wantSkip: true,
		},
		{
			name:         "a git@ build context can't be read",
			service:      "build: git@github.com:org/repo.git",
			wantRegistry: "local", wantName: "app", wantUnverified: true, wantSkip: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			composePath := writeCompose(t, dir, "services:\n  app:\n    "+tt.service+"\n")
			for name, content := range tt.files {
				writeFile(t, filepath.Join(dir, name), content)
			}
			if tt.dotEnv != "" {
				writeFile(t, filepath.Join(dir, ".env"), tt.dotEnv)
			}

			images, err := Parse(models.Project{Name: "test", Path: dir, ComposeFiles: []string{composePath}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(images) != 1 {
				t.Fatalf("got %d images, want 1", len(images))
			}
			got := images[0]

			if got.Image.Registry != tt.wantRegistry || got.Image.Name != tt.wantName || got.Image.Tag != tt.wantTag {
				t.Errorf("image = %s/%s:%s, want %s/%s:%s", got.Image.Registry, got.Image.Name, got.Image.Tag, tt.wantRegistry, tt.wantName, tt.wantTag)
			}
			if got.Image.Service != "app" {
				t.Errorf("Service = %q, want app", got.Image.Service)
			}
			wantDockerfile := tt.wantDockerfile
			if wantDockerfile != "" {
				wantDockerfile = filepath.Join(dir, wantDockerfile)
			}
			if got.Dockerfile != wantDockerfile {
				t.Errorf("Dockerfile = %q, want %q", got.Dockerfile, wantDockerfile)
			}
			if got.LocalImage != tt.wantLocal {
				t.Errorf("LocalImage = %q, want %q", got.LocalImage, tt.wantLocal)
			}
			if got.UsesLatest != tt.wantFloating {
				t.Errorf("UsesLatest = %v, want %v", got.UsesLatest, tt.wantFloating)
			}
			if got.Unverified != tt.wantUnverified {
				t.Errorf("Unverified = %v, want %v (note %q)", got.Unverified, tt.wantUnverified, got.RecommendationNote)
			}
			if got.SkipLookup != tt.wantSkip {
				t.Errorf("SkipLookup = %v, want %v", got.SkipLookup, tt.wantSkip)
			}
			if (tt.wantUnverified || tt.wantSkip) && got.RecommendationNote == "" {
				t.Error("RecommendationNote is empty, want the reason")
			}
			if fmt.Sprint(got.UnresolvedVariables) != fmt.Sprint(tt.wantUnresolved) {
				t.Errorf("UnresolvedVariables = %v, want %v", got.UnresolvedVariables, tt.wantUnresolved)
			}
		})
	}
}

func TestParse_StandaloneDockerfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	writeFile(t, path, "FROM golang:1.22 AS build\nFROM alpine:3.20\n")

	images, err := Parse(models.Project{Name: "tool", Path: dir, Dockerfiles: []string{path}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 {
		t.Fatalf("got %d images, want 1", len(images))
	}
	got := images[0]
	if got.Image.Name != "alpine" || got.Image.Tag != "3.20" || got.Image.Project != "tool" {
		t.Errorf("image = %+v", got.Image)
	}
	if got.Image.Service != "" {
		t.Errorf("Service = %q, want none", got.Image.Service)
	}
	if got.Dockerfile != path || got.File != path {
		t.Errorf("Dockerfile = %q, File = %q, want %q", got.Dockerfile, got.File, path)
	}
}

func TestParse_UnreadableStandaloneDockerfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Dockerfile")
	images, err := Parse(models.Project{Name: "tool", Path: dir, Dockerfiles: []string{path}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || !images[0].Unverified || !images[0].SkipLookup {
		t.Errorf("images = %+v, want one unverified row that skips the lookup", images)
	}
}

func TestDropReferencedDockerfiles(t *testing.T) {
	root := t.TempDir()
	composePath := filepath.Join(root, "stack", "docker-compose.yml")
	writeFile(t, composePath, "services:\n  app:\n    build: ../shared\n  web:\n    build:\n      context: ./web\n      dockerfile: Dockerfile\n")
	shared := filepath.Join(root, "shared", "Dockerfile")
	web := filepath.Join(root, "stack", "web", "Dockerfile")
	tool := filepath.Join(root, "tool", "Dockerfile")

	projects := []models.Project{
		{Name: "stack", Path: filepath.Dir(composePath), ComposeFiles: []string{composePath}},
		{Name: "shared", Path: filepath.Dir(shared), Dockerfiles: []string{shared}},
		{Name: "web", Path: filepath.Dir(web), Dockerfiles: []string{web}},
		{Name: "tool", Path: filepath.Dir(tool), Dockerfiles: []string{tool}},
	}
	got := DropReferencedDockerfiles(projects, nil)

	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if fmt.Sprint(names) != "[stack tool]" {
		t.Errorf("projects = %v, want [stack tool]", names)
	}
}

// TestDropReferencedDockerfiles_KeepsOneBuiltInline keeps a folder's Dockerfile
// when the compose service that points at its folder builds from
// dockerfile_inline, because that row reports the inline content instead.
func TestDropReferencedDockerfiles_KeepsOneBuiltInline(t *testing.T) {
	root := t.TempDir()
	composePath := filepath.Join(root, "app", "docker-compose.yml")
	writeFile(t, composePath, "services:\n  app:\n    build:\n      context: ../other\n      dockerfile_inline: \"FROM alpine:3.20\"\n")
	other := filepath.Join(root, "other", "Dockerfile")
	writeFile(t, other, "FROM alpine:3.10\n")

	projects := []models.Project{
		{Name: "app", Path: filepath.Dir(composePath), ComposeFiles: []string{composePath}},
		{Name: "other", Path: filepath.Dir(other), Dockerfiles: []string{other}},
	}
	got := DropReferencedDockerfiles(projects, nil)

	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if fmt.Sprint(names) != "[app other]" {
		t.Errorf("projects = %v, want [app other]", names)
	}
}

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		ref          string
		wantRegistry string
		wantName     string
		wantTag      string
	}{
		{"nginx", "docker.io", "nginx", "latest"},
		{"nginx:1.25", "docker.io", "nginx", "1.25"},
		{"org/app:1.2", "docker.io", "org/app", "1.2"},
		{"ghcr.io/org/app:1.2", "ghcr.io", "org/app", "1.2"},
		{"localhost:5000/app", "localhost:5000", "app", "latest"},
		{"localhost:5000/app:1.2", "localhost:5000", "app", "1.2"},
		{"registry.example.com:5000/org/app:1.2", "registry.example.com:5000", "org/app", "1.2"},
		{"python:3.12@sha256:abc", "docker.io", "python", "3.12"},
		{"python@sha256:abc", "docker.io", "python", "latest"},
		{"ghcr.io/org/app:${TAG}", "ghcr.io", "org/app", "${TAG}"},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got := parseImageRef(tt.ref)
			if got.Registry != tt.wantRegistry || got.Name != tt.wantName || got.Tag != tt.wantTag {
				t.Errorf("parseImageRef = %s %s %s, want %s %s %s", got.Registry, got.Name, got.Tag, tt.wantRegistry, tt.wantName, tt.wantTag)
			}
		})
	}
}

func TestParse_DigestPins(t *testing.T) {
	tests := []struct {
		name         string
		image        string
		wantSkip     bool
		wantFloating bool
	}{
		{name: "a tag and digest checks the tag", image: "python:3.12@sha256:abc"},
		{name: "a digest alone has no tag to compare", image: "python@sha256:abc", wantSkip: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			composePath := writeCompose(t, dir, "services:\n  app:\n    image: "+tt.image+"\n")
			images, err := Parse(models.Project{Name: "t", Path: dir, ComposeFiles: []string{composePath}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if images[0].UsesLatest != tt.wantFloating {
				t.Errorf("UsesLatest = %v, want %v", images[0].UsesLatest, tt.wantFloating)
			}
			if images[0].SkipLookup != tt.wantSkip {
				t.Errorf("SkipLookup = %v, want %v", images[0].SkipLookup, tt.wantSkip)
			}
		})
	}
}
