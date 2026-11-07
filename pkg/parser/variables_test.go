package parser

import (
	"docker-scanner/pkg/models"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func lookupFrom(vars map[string]string) LookupEnv {
	return func(name string) (string, bool) {
		value, ok := vars[name]
		return value, ok
	}
}

func TestExpandVariables(t *testing.T) {
	vars := map[string]string{"TAG": "1.2.3", "EMPTY": "", "REGISTRY": "ghcr.io", "INNER": "9.9.9"}
	tests := []struct {
		name           string
		ref            string
		want           string
		wantUnresolved []string
	}{
		{name: "no variables", ref: "nginx:1.25", want: "nginx:1.25"},
		{name: "braces", ref: "app:${TAG}", want: "app:1.2.3"},
		{name: "no braces", ref: "app:$TAG", want: "app:1.2.3"},
		{name: "several variables", ref: "${REGISTRY}/app:${TAG}", want: "ghcr.io/app:1.2.3"},

		{name: "colon default ignored when set", ref: "app:${TAG:-0.0.1}", want: "app:1.2.3"},
		{name: "colon default used when unset", ref: "app:${NOPE:-0.0.1}", want: "app:0.0.1"},
		{name: "colon default used when empty", ref: "app:${EMPTY:-0.0.1}", want: "app:0.0.1"},
		{name: "dash default ignored when set", ref: "app:${TAG-0.0.1}", want: "app:1.2.3"},
		{name: "dash default used when unset", ref: "app:${NOPE-0.0.1}", want: "app:0.0.1"},
		{name: "dash default not used when empty", ref: "app${EMPTY-:0.0.1}", want: "app"},
		{name: "issue example, default contains the colon", ref: "ghcr.io/formbricks/hub${HUB_IMAGE_REF:-:0.8.1}", want: "ghcr.io/formbricks/hub:0.8.1"},
		{name: "nested default", ref: "app:${NOPE:-${INNER}}", want: "app:9.9.9"},
		{name: "nested default with its own default", ref: "app:${NOPE:-${ALSO_NOPE:-4.5.6}}", want: "app:4.5.6"},
		{name: "empty default", ref: "app${NOPE:-}", want: "app"},

		{name: "required and set", ref: "app:${TAG:?set a tag}", want: "app:1.2.3"},
		{name: "required and unset", ref: "app:${NOPE:?set a tag}", want: "app:${NOPE}", wantUnresolved: []string{"NOPE"}},
		{name: "required without colon and unset", ref: "app:${NOPE?set a tag}", want: "app:${NOPE}", wantUnresolved: []string{"NOPE"}},
		{name: "alternate when set", ref: "app:${TAG:+7.7.7}", want: "app:7.7.7"},
		{name: "alternate when unset", ref: "app${NOPE:+:7.7.7}", want: "app"},

		{name: "unset with no default", ref: "app:${NOPE}", want: "app:${NOPE}", wantUnresolved: []string{"NOPE"}},
		{name: "unset without braces", ref: "app:$NOPE", want: "app:${NOPE}", wantUnresolved: []string{"NOPE"}},
		{name: "empty with no default", ref: "app:${EMPTY}", want: "app:${EMPTY}", wantUnresolved: []string{"EMPTY"}},
		{name: "unset in a nested default", ref: "app:${NOPE:-${ALSO_NOPE}}", want: "app:${ALSO_NOPE}", wantUnresolved: []string{"ALSO_NOPE"}},
		{name: "each unset variable once", ref: "${A}/${B}:${A}", want: "${A}/${B}:${A}", wantUnresolved: []string{"A", "B"}},

		{name: "escaped dollar", ref: "app:$$TAG", want: "app:$TAG"},
		{name: "lone dollar is kept", ref: "app:1$", want: "app:1$"},
		{name: "no closing brace is unresolved", ref: "app:${TAG", want: "app:${TAG", wantUnresolved: []string{"${TAG"}},
		{name: "an invalid name is unresolved", ref: "app:${1BAD}", want: "app:${1BAD}", wantUnresolved: []string{"${1BAD}"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, unresolved := expandVariables(tt.ref, lookupFrom(vars))
			if got != tt.want {
				t.Errorf("expanded = %q, want %q", got, tt.want)
			}
			if fmt.Sprint(unresolved) != fmt.Sprint(tt.wantUnresolved) {
				t.Errorf("unresolved = %v, want %v", unresolved, tt.wantUnresolved)
			}
		})
	}
}

func TestParse_ExpandsVariables(t *testing.T) {
	tests := []struct {
		name           string
		image          string
		dotEnv         string
		shell          map[string]string
		wantRegistry   string
		wantName       string
		wantTag        string
		wantFloating   bool
		wantUnresolved []string
	}{
		{
			name:         "the issue example uses its default",
			image:        "ghcr.io/formbricks/hub${HUB_IMAGE_REF:-:0.8.1}",
			wantRegistry: "ghcr.io", wantName: "formbricks/hub", wantTag: "0.8.1",
		},
		{
			name: "a value from the project's .env", image: "app:${TAG}", dotEnv: "TAG=1.2.3\n",
			wantRegistry: "docker.io", wantName: "app", wantTag: "1.2.3",
		},
		{
			name: "the shell wins over .env", image: "app:${TAG}", dotEnv: "TAG=1.2.3\n", shell: map[string]string{"TAG": "4.5.6"},
			wantRegistry: "docker.io", wantName: "app", wantTag: "4.5.6",
		},
		{
			name: ".env wins over a default", image: "app:${TAG:-0.0.1}", dotEnv: "TAG=1.2.3\n",
			wantRegistry: "docker.io", wantName: "app", wantTag: "1.2.3",
		},
		{
			name: ".env formats: comments, export, quotes", image: "${A}/${B}:${C}",
			dotEnv:       "# a comment\n\nexport A=ghcr.io\nB=\"org/app\"\nC='1.2.3' # pinned\nnot a variable\n",
			wantRegistry: "ghcr.io", wantName: "org/app", wantTag: "1.2.3",
		},
		{
			name: "a floating value is still flagged", image: "app:${TAG}", dotEnv: "TAG=latest\n",
			wantRegistry: "docker.io", wantName: "app", wantTag: "latest", wantFloating: true,
		},
		{
			name: "no value is unresolved and keeps the variable name", image: "ghcr.io/org/app:${TAG}",
			wantRegistry: "ghcr.io", wantName: "org/app", wantTag: "${TAG}", wantUnresolved: []string{"TAG"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			composePath := writeCompose(t, dir, "services:\n  app:\n    image: "+tt.image+"\n")
			if tt.dotEnv != "" {
				if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(tt.dotEnv), 0644); err != nil {
					t.Fatal(err)
				}
			}

			images, err := Parse(models.Project{Name: "test", Path: dir, ComposeFiles: []string{composePath}}, lookupFrom(tt.shell))
			if err != nil {
				t.Fatal(err)
			}
			got := images[0]

			if got.Image.Registry != tt.wantRegistry || got.Image.Name != tt.wantName || got.Image.Tag != tt.wantTag {
				t.Errorf("image = %s/%s:%s, want %s/%s:%s", got.Image.Registry, got.Image.Name, got.Image.Tag, tt.wantRegistry, tt.wantName, tt.wantTag)
			}
			if got.UsesLatest != tt.wantFloating {
				t.Errorf("UsesLatest = %v, want %v", got.UsesLatest, tt.wantFloating)
			}
			if fmt.Sprint(got.UnresolvedVariables) != fmt.Sprint(tt.wantUnresolved) {
				t.Errorf("UnresolvedVariables = %v, want %v", got.UnresolvedVariables, tt.wantUnresolved)
			}
			if wantUnverified := tt.wantUnresolved != nil; got.Unverified != wantUnverified {
				t.Errorf("Unverified = %v, want %v", got.Unverified, wantUnverified)
			}
		})
	}
}

func TestParse_NilLookupStillReadsDotEnv(t *testing.T) {
	dir := t.TempDir()
	composePath := writeCompose(t, dir, "services:\n  app:\n    image: app:${TAG}\n")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TAG=1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	images, err := Parse(models.Project{Name: "test", Path: dir, ComposeFiles: []string{composePath}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if images[0].Image.Tag != "1.2.3" {
		t.Errorf("tag = %q, want 1.2.3", images[0].Image.Tag)
	}
}
