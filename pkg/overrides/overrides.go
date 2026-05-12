// Package overrides provides per-image version-source overrides. When the
// scanner can't reliably determine the latest version from the container
// registry alone (e.g., LinuxServer wraps upstream apps and tags with their
// own build numbers), an override tells the scanner to look elsewhere —
// typically the project's GitHub releases.
package overrides

import (
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Source enumerates where to look up versions for an image.
type Source string

const (
	SourceGitHubRelease Source = "github_release"
)

// validSources is the set of source types this version of docker-scanner
// knows how to handle. Listed explicitly so a typo or future-source name
// produces a loud parse error rather than silently doing nothing.
var validSources = map[Source]bool{
	SourceGitHubRelease: true,
}

// Override is a single image → version-source mapping.
type Override struct {
	Image  string `yaml:"image"`
	Source Source `yaml:"source"`
	Repo   string `yaml:"repo"`
}

// Config is the top-level parsed override file.
type Config struct {
	Overrides []Override `yaml:"overrides"`
}

// Parse turns YAML bytes into a Config. Validates that:
//   - the YAML itself is syntactically valid
//   - every override has an image and a known source
//   - github_release sources have a repo
//   - no two entries target the same image
//
// Failing loud at parse time is deliberate: a silently-broken override
// would just produce the same wrong recommendation the user was trying to
// fix, with no signal that anything was wrong.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	seen := make(map[string]bool)
	for i, o := range cfg.Overrides {
		if err := validateOverride(o, i); err != nil {
			return nil, err
		}
		if seen[o.Image] {
			return nil, fmt.Errorf("duplicate override for image %q", o.Image)
		}
		seen[o.Image] = true
	}

	return &cfg, nil
}

// validateOverride checks a single entry. The index is included in error
// messages to help the user find the broken entry in a large file.
func validateOverride(o Override, index int) error {
	if o.Image == "" {
		return fmt.Errorf("override at index %d: missing 'image' field", index)
	}
	if o.Source == "" {
		return fmt.Errorf("override for image %q: missing 'source' field", o.Image)
	}
	if !validSources[o.Source] {
		return fmt.Errorf("override for image %q: unknown source %q (supported: github_release)", o.Image, o.Source)
	}
	if o.Source == SourceGitHubRelease && o.Repo == "" {
		return fmt.Errorf("override for image %q: github_release source requires 'repo' field", o.Image)
	}
	return nil
}

// Find returns the override for the given image name, or nil if none
// matches. Safe to call on a nil receiver — returns nil. Match is exact
// and case-sensitive per the OCI image-name spec.
func (c *Config) Find(image string) *Override {
	if c == nil {
		return nil
	}
	for i := range c.Overrides {
		if c.Overrides[i].Image == image {
			return &c.Overrides[i]
		}
	}
	return nil
}

// Release is a parsed GitHub release response. Only the fields the
// scanner cares about are extracted; everything else is ignored.
type Release struct {
	TagName     string
	Prerelease  bool
	PublishedAt time.Time
}

// githubRelease mirrors the subset of the GitHub API response we care
// about. Separate from the public Release type so the wire format and the
// domain type can evolve independently.
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
}

// releasesURL builds the GitHub API endpoint that returns ALL releases
// (newest first), not just the most recent stable one. Walking this list
// lets us apply the same safeDays cutoff the registry path uses, instead
// of blindly accepting whatever GitHub considers "latest" right now.
//
// Note: this is /releases (plural), not /releases/latest. /releases/latest
// excludes pre-releases entirely, which would silently break for projects
// like Sonarr that label all their releases as pre-release.
func releasesURL(repo string) string {
	return fmt.Sprintf("https://api.github.com/repos/%s/releases", repo)
}

// parseReleasesList turns the GitHub /releases response into a slice of
// Release values in the order GitHub returned them (newest first).
// Returns an error if the JSON is invalid.
func parseReleasesList(body []byte) ([]Release, error) {
	var raw []githubRelease
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse GitHub releases list: %w", err)
	}

	out := make([]Release, 0, len(raw))
	for _, gr := range raw {
		rel := Release{
			TagName:    gr.TagName,
			Prerelease: gr.Prerelease,
		}
		if gr.PublishedAt != "" {
			if t, err := time.Parse(time.RFC3339, gr.PublishedAt); err == nil {
				rel.PublishedAt = t
			}
		}
		out = append(out, rel)
	}
	return out, nil
}

// parseLatestRelease turns a GitHub /releases/latest response into a
// Release. Kept for compatibility with existing callers; new code should
// prefer parseReleasesList + pickSafeRelease.
func parseLatestRelease(body []byte) (*Release, error) {
	var gr githubRelease
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, fmt.Errorf("parse GitHub release: %w", err)
	}
	if gr.TagName == "" {
		return nil, fmt.Errorf("GitHub release has no tag_name")
	}

	rel := &Release{
		TagName:    gr.TagName,
		Prerelease: gr.Prerelease,
	}
	if gr.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, gr.PublishedAt); err == nil {
			rel.PublishedAt = t
		}
	}
	return rel, nil
}

// pickSafeRelease walks a newest-first releases list and returns the first
// release that is at least safeDays old. Returns nil if no release qualifies
// (either the list is empty, or every release is too recent, or every
// release has no published_at to compare against).
//
// Pre-release flag is intentionally NOT used as a filter: many projects
// (Sonarr, Radarr) label every release as pre-release. Filtering would
// silently break the override for those projects.
func pickSafeRelease(releases []Release, now time.Time, safeDays int) *Release {
	cutoff := now.AddDate(0, 0, -safeDays)

	for i := range releases {
		r := releases[i]
		if r.PublishedAt.IsZero() {
			continue
		}
		if !r.PublishedAt.After(cutoff) {
			return &r
		}
	}
	return nil
}

// Getter fetches a URL and returns the response body. The HTTP plumbing
// is injected so Lookup can be tested without real network calls.
type Getter func(url string) ([]byte, error)

// Lookup resolves an override to a Release by calling the appropriate
// upstream source. The getter is injected; production callers pass an
// http-backed implementation, tests pass a fake.
//
// Kept for compatibility with the single-release flow; new code should use
// Resolve which applies the safeDays rule.
func Lookup(o *Override, get Getter) (*Release, error) {
	switch o.Source {
	case SourceGitHubRelease:
		body, err := get(releaseURL(o.Repo))
		if err != nil {
			return nil, fmt.Errorf("fetch GitHub release for %s: %w", o.Image, err)
		}
		return parseLatestRelease(body)
	default:
		return nil, fmt.Errorf("unsupported source %q for image %s", o.Source, o.Image)
	}
}

// releaseURL builds the /releases/latest endpoint. Used by Lookup only.
// Most code should use releasesURL (no "latest") to get the full list.
func releaseURL(repo string) string {
	return fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
}