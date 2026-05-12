package overrides

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse_ValidConfig(t *testing.T) {
	yaml := []byte(`overrides:
  - image: lscr.io/linuxserver/sonarr
    source: github_release
    repo: Sonarr/Sonarr
  - image: docker.io/grafana/grafana
    source: github_release
    repo: grafana/grafana
`)

	cfg, err := Parse(yaml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Overrides) != 2 {
		t.Fatalf("expected 2 overrides, got %d", len(cfg.Overrides))
	}
	if cfg.Overrides[0].Image != "lscr.io/linuxserver/sonarr" {
		t.Errorf("first override image: got %q", cfg.Overrides[0].Image)
	}
	if cfg.Overrides[0].Source != "github_release" {
		t.Errorf("first override source: got %q", cfg.Overrides[0].Source)
	}
	if cfg.Overrides[0].Repo != "Sonarr/Sonarr" {
		t.Errorf("first override repo: got %q", cfg.Overrides[0].Repo)
	}
}

func TestParse_EmptyConfig(t *testing.T) {
	// Empty file or just `overrides: []` should parse cleanly with zero entries.
	yaml := []byte(`overrides: []`)

	cfg, err := Parse(yaml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Overrides) != 0 {
		t.Errorf("expected 0 overrides, got %d", len(cfg.Overrides))
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	yaml := []byte(`{{{not valid yaml`)

	_, err := Parse(yaml)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestParse_MissingImage(t *testing.T) {
	// Image is the lookup key — without it the entry is useless.
	yaml := []byte(`overrides:
  - source: github_release
    repo: foo/bar
`)

	_, err := Parse(yaml)
	if err == nil {
		t.Error("expected error when override entry is missing 'image' field")
	}
	if err != nil && !strings.Contains(err.Error(), "image") {
		t.Errorf("error should mention missing image field, got: %v", err)
	}
}

func TestParse_MissingSource(t *testing.T) {
	yaml := []byte(`overrides:
  - image: docker.io/foo
    repo: foo/bar
`)

	_, err := Parse(yaml)
	if err == nil {
		t.Error("expected error when override entry is missing 'source' field")
	}
}

func TestParse_UnknownSource(t *testing.T) {
	// Future-proofing: refuse to silently ignore an entry pointing at a
	// source type we haven't implemented. If the user wrote 'gitlab_release'
	// expecting it to work, failing loudly is better than skipping silently.
	yaml := []byte(`overrides:
  - image: docker.io/foo
    source: gitlab_release
    repo: foo/bar
`)

	_, err := Parse(yaml)
	if err == nil {
		t.Error("expected error for unknown source type")
	}
	if err != nil && !strings.Contains(err.Error(), "gitlab_release") {
		t.Errorf("error should name the unknown source, got: %v", err)
	}
}

func TestParse_DuplicateImages(t *testing.T) {
	// Two entries for the same image is almost certainly a mistake. The
	// first or second would silently win, and the user wouldn't know.
	yaml := []byte(`overrides:
  - image: docker.io/foo
    source: github_release
    repo: a/b
  - image: docker.io/foo
    source: github_release
    repo: c/d
`)

	_, err := Parse(yaml)
	if err == nil {
		t.Error("expected error when two override entries target the same image")
	}
}

func TestParse_MissingRepoForGithubSource(t *testing.T) {
	// github_release without a repo can't function. Fail loud.
	yaml := []byte(`overrides:
  - image: docker.io/foo
    source: github_release
`)

	_, err := Parse(yaml)
	if err == nil {
		t.Error("expected error when github_release source has no repo")
	}
}
func TestFind_ExactMatch(t *testing.T) {
	cfg := &Config{
		Overrides: []Override{
			{Image: "lscr.io/linuxserver/sonarr", Source: SourceGitHubRelease, Repo: "Sonarr/Sonarr"},
			{Image: "docker.io/grafana/grafana", Source: SourceGitHubRelease, Repo: "grafana/grafana"},
		},
	}

	got := cfg.Find("docker.io/grafana/grafana")
	if got == nil {
		t.Fatal("expected to find override for grafana")
	}
	if got.Repo != "grafana/grafana" {
		t.Errorf("got repo %q, want grafana/grafana", got.Repo)
	}
}

func TestFind_NoMatch(t *testing.T) {
	cfg := &Config{
		Overrides: []Override{
			{Image: "docker.io/grafana/grafana", Source: SourceGitHubRelease, Repo: "grafana/grafana"},
		},
	}

	got := cfg.Find("docker.io/postgres")
	if got != nil {
		t.Errorf("expected nil for unmatched image, got %+v", got)
	}
}

func TestFind_EmptyConfig(t *testing.T) {
	cfg := &Config{}
	got := cfg.Find("docker.io/whatever")
	if got != nil {
		t.Errorf("expected nil from empty config, got %+v", got)
	}
}

func TestFind_NilReceiverSafe(t *testing.T) {
	// Callers shouldn't have to check for nil Config before calling Find.
	// A nil config means "no overrides loaded" — Find should return nil.
	var cfg *Config
	got := cfg.Find("docker.io/whatever")
	if got != nil {
		t.Errorf("expected nil from nil config, got %+v", got)
	}
}

func TestFind_CaseSensitive(t *testing.T) {
	// Docker image names are case-sensitive per the OCI spec, so the
	// matcher must be too. ghcr.io's owner names ARE case-folded but the
	// image name portion isn't, and we don't want false matches.
	cfg := &Config{
		Overrides: []Override{
			{Image: "ghcr.io/Sonarr/sonarr", Source: SourceGitHubRelease, Repo: "Sonarr/Sonarr"},
		},
	}

	if cfg.Find("ghcr.io/sonarr/sonarr") != nil {
		t.Error("matcher should be case-sensitive")
	}
	if cfg.Find("ghcr.io/Sonarr/sonarr") == nil {
		t.Error("exact match should hit")
	}
}

func TestParseLatestRelease_ExtractsTagName(t *testing.T) {
	// Real shape from GitHub's /repos/{owner}/{repo}/releases/latest endpoint.
	body := []byte(`{
		"tag_name": "v1.42.1",
		"name": "Release v1.42.1",
		"prerelease": false,
		"published_at": "2026-04-15T20:01:40Z"
	}`)

	rel, err := parseLatestRelease(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.TagName != "v1.42.1" {
		t.Errorf("got tag %q, want v1.42.1", rel.TagName)
	}
	if rel.Prerelease {
		t.Errorf("got prerelease=true, want false")
	}
}

func TestParseLatestRelease_PreReleaseFlag(t *testing.T) {
	body := []byte(`{
		"tag_name": "v2.0.0-rc1",
		"prerelease": true,
		"published_at": "2026-04-15T20:01:40Z"
	}`)

	rel, err := parseLatestRelease(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rel.Prerelease {
		t.Error("expected prerelease=true")
	}
}

func TestParseLatestRelease_ParsesTimestamp(t *testing.T) {
	body := []byte(`{
		"tag_name": "v1.0.0",
		"published_at": "2026-04-15T20:01:40Z"
	}`)

	rel, err := parseLatestRelease(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.PublishedAt.IsZero() {
		t.Error("expected published_at to be parsed into a non-zero time")
	}
	if rel.PublishedAt.Year() != 2026 {
		t.Errorf("got year %d, want 2026", rel.PublishedAt.Year())
	}
}

func TestParseLatestRelease_MissingTag(t *testing.T) {
	// A release without a tag_name is useless to us — that's our entire
	// reason for hitting this endpoint.
	body := []byte(`{
		"name": "Some release",
		"prerelease": false
	}`)

	_, err := parseLatestRelease(body)
	if err == nil {
		t.Error("expected error when tag_name is missing")
	}
}

func TestParseLatestRelease_InvalidJSON(t *testing.T) {
	body := []byte(`not json at all`)

	_, err := parseLatestRelease(body)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestReleaseURL_BuildsCorrectEndpoint(t *testing.T) {
	url := releaseURL("Sonarr/Sonarr")
	want := "https://api.github.com/repos/Sonarr/Sonarr/releases/latest"
	if url != want {
		t.Errorf("got %q, want %q", url, want)
	}
}
func TestLookup_GitHubReleaseSuccess(t *testing.T) {
	override := &Override{
		Image:  "lscr.io/linuxserver/sonarr",
		Source: SourceGitHubRelease,
		Repo:   "Sonarr/Sonarr",
	}

	fakeGet := func(url string) ([]byte, error) {
		want := "https://api.github.com/repos/Sonarr/Sonarr/releases/latest"
		if url != want {
			t.Errorf("getter called with %q, want %q", url, want)
		}
		return []byte(`{
			"tag_name": "v4.0.17.2953",
			"prerelease": true,
			"published_at": "2026-03-25T10:00:00Z"
		}`), nil
	}

	rel, err := Lookup(override, fakeGet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.TagName != "v4.0.17.2953" {
		t.Errorf("got tag %q, want v4.0.17.2953", rel.TagName)
	}
}

func TestLookup_PropagatesGetterError(t *testing.T) {
	override := &Override{
		Image:  "docker.io/foo",
		Source: SourceGitHubRelease,
		Repo:   "foo/bar",
	}

	fakeGet := func(url string) ([]byte, error) {
		return nil, fmt.Errorf("network is down")
	}

	_, err := Lookup(override, fakeGet)
	if err == nil {
		t.Error("expected error when getter fails")
	}
}

func TestLookup_PropagatesParseError(t *testing.T) {
	override := &Override{
		Image:  "docker.io/foo",
		Source: SourceGitHubRelease,
		Repo:   "foo/bar",
	}

	fakeGet := func(url string) ([]byte, error) {
		return []byte(`{"name": "no tag here"}`), nil
	}

	_, err := Lookup(override, fakeGet)
	if err == nil {
		t.Error("expected error when response has no tag")
	}
}
func TestLoad_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-overrides.yml")
	content := `overrides:
  - image: docker.io/foo
    source: github_release
    repo: foo/bar
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(LoadOptions{ExplicitPath: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Overrides) != 1 {
		t.Errorf("expected 1 override, got %d", len(cfg.Overrides))
	}
	if cfg.Overrides[0].Image != "docker.io/foo" {
		t.Errorf("got image %q", cfg.Overrides[0].Image)
	}
}

func TestLoad_ExplicitPathMissing(t *testing.T) {
	// User passed --overrides FILE and FILE doesn't exist.
	// This should be an error — they asked for a file we can't find.
	_, err := Load(LoadOptions{ExplicitPath: "/nonexistent/path/foo.yml"})
	if err == nil {
		t.Error("expected error when explicit path doesn't exist")
	}
}

func TestLoad_ExplicitPathInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yml")
	if err := os.WriteFile(path, []byte(`{{{not yaml`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(LoadOptions{ExplicitPath: path})
	if err == nil {
		t.Error("expected error for invalid YAML in explicit file")
	}
}

func TestLoad_FallsBackToBundledDefaults(t *testing.T) {
	// No explicit path, no XDG file — should load the bundled defaults
	// embedded in the binary. The defaults file should at minimum parse
	// cleanly and contain some entries.
	cfg, err := Load(LoadOptions{
		ExplicitPath:  "",
		XDGConfigHome: "/nonexistent", // force fallback to bundled
		HomeDir:       "/nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error loading bundled defaults: %v", err)
	}
	if len(cfg.Overrides) == 0 {
		t.Error("expected bundled defaults to contain at least one override")
	}
}

func TestLoad_XDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	xdgDir := filepath.Join(dir, "docker-scanner")
	if err := os.MkdirAll(xdgDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(xdgDir, "overrides.yml")
	content := `overrides:
  - image: docker.io/from-xdg
    source: github_release
    repo: x/y
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(LoadOptions{XDGConfigHome: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Overrides) != 1 || cfg.Overrides[0].Image != "docker.io/from-xdg" {
		t.Errorf("expected single from-xdg override, got %+v", cfg.Overrides)
	}
}

func TestLoad_HomeConfigDir(t *testing.T) {
	// Fall back from XDG to ~/.config when XDG_CONFIG_HOME is unset.
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".config", "docker-scanner")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfgDir, "overrides.yml")
	content := `overrides:
  - image: docker.io/from-home
    source: github_release
    repo: x/y
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(LoadOptions{HomeDir: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Overrides) != 1 || cfg.Overrides[0].Image != "docker.io/from-home" {
		t.Errorf("expected single from-home override, got %+v", cfg.Overrides)
	}
}

func TestLoad_ExplicitPathBeatsXDG(t *testing.T) {
	// Precedence: explicit > XDG > home > bundled
	dir := t.TempDir()

	xdgDir := filepath.Join(dir, "xdg", "docker-scanner")
	if err := os.MkdirAll(xdgDir, 0755); err != nil {
		t.Fatal(err)
	}
	xdgPath := filepath.Join(xdgDir, "overrides.yml")
	if err := os.WriteFile(xdgPath, []byte("overrides:\n  - image: docker.io/xdg\n    source: github_release\n    repo: x/y\n"), 0644); err != nil {
		t.Fatal(err)
	}

	explicitPath := filepath.Join(dir, "explicit.yml")
	if err := os.WriteFile(explicitPath, []byte("overrides:\n  - image: docker.io/explicit\n    source: github_release\n    repo: x/y\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(LoadOptions{
		ExplicitPath:  explicitPath,
		XDGConfigHome: filepath.Join(dir, "xdg"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Overrides[0].Image != "docker.io/explicit" {
		t.Errorf("explicit should win over XDG, got %q", cfg.Overrides[0].Image)
	}
}

func TestFormatAge_RecentDateGivesDays(t *testing.T) {
	// Pinned reference time so the test isn't flaky
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	released := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	got := formatAge(now, released)
	if got != "3 days" {
		t.Errorf("got %q, want '3 days'", got)
	}
}

func TestFormatAge_ZeroTimeGivesUnknown(t *testing.T) {
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	var zero time.Time

	got := formatAge(now, zero)
	if got != "age unknown" {
		t.Errorf("got %q, want 'age unknown'", got)
	}
}

// The Resolve signature gained a safeDays parameter and now hits
// /releases (list) instead of /releases/latest (single).

func TestResolve_ReturnsRecommendation(t *testing.T) {
	override := &Override{
		Image:  "lscr.io/linuxserver/sonarr",
		Source: SourceGitHubRelease,
		Repo:   "Sonarr/Sonarr",
	}
	// Use a date far enough in the past to pass safeDays=0 cutoff.
	// (safeDays=0 means "no cutoff", so anything qualifies.)
	fakeGet := func(url string) ([]byte, error) {
		return []byte(`[
			{"tag_name": "v4.0.17.2953", "prerelease": true, "published_at": "2026-03-25T10:00:00Z"}
		]`), nil
	}

	rec, err := Resolve(override, fakeGet, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec == nil {
		t.Fatal("expected a recommendation, got nil")
	}
	if rec.Version != "v4.0.17.2953" {
		t.Errorf("got version %q, want v4.0.17.2953", rec.Version)
	}
	if rec.Age == "" {
		t.Error("expected a non-empty age string")
	}
	if !rec.Prerelease {
		t.Error("expected Prerelease=true")
	}
}

func TestResolve_NoPublishedAtMeansSkipped(t *testing.T) {
	// A release without a published_at can't be safety-checked,
	// so pickSafeRelease skips it. With only one such release in the
	// list, Resolve returns nil (no eligible release).
	override := &Override{
		Image:  "docker.io/foo",
		Source: SourceGitHubRelease,
		Repo:   "foo/bar",
	}
	fakeGet := func(url string) ([]byte, error) {
		return []byte(`[{"tag_name": "v1.0.0"}]`), nil
	}

	rec, err := Resolve(override, fakeGet, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec != nil {
		t.Errorf("expected nil recommendation when release has no date, got %+v", rec)
	}
}

func TestResolve_PropagatesError(t *testing.T) {
	override := &Override{
		Image:  "docker.io/foo",
		Source: SourceGitHubRelease,
		Repo:   "foo/bar",
	}
	fakeGet := func(url string) ([]byte, error) {
		return []byte(`not valid json`), nil
	}

	_, err := Resolve(override, fakeGet, 0)
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}
