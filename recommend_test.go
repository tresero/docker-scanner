package main

import (
	"docker-scanner/pkg/models"
	"docker-scanner/pkg/overrides"
	"docker-scanner/pkg/registry"
	"docker-scanner/pkg/report"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

// fakeRegistry serves fixed versions and handles every image, so tests never
// reach a real registry.
type fakeRegistry struct {
	versions []models.RegistryVersion
	err      error
}

func (f *fakeRegistry) Name() string         { return "fake" }
func (f *fakeRegistry) Supports(string) bool { return true }
func (f *fakeRegistry) FetchVersions(models.Image) ([]models.RegistryVersion, error) {
	return f.versions, f.err
}

func newTestRecommender(versions []models.RegistryVersion) *recommender {
	return &recommender{
		registries: []registry.Registry{&fakeRegistry{versions: versions}},
		safeDays:   3,
		now:        func() time.Time { return testNow },
	}
}

// buildRegistry also says what a tag points to, and counts how often it's asked.
type buildRegistry struct {
	fakeRegistry
	build    registry.TagBuild
	buildErr error
	asked    int
}

func (b *buildRegistry) CurrentBuild(models.Image) (registry.TagBuild, error) {
	b.asked++
	return b.build, b.buildErr
}

func TestRecommend_NewerBuild(t *testing.T) {
	pushed := testNow.AddDate(0, 0, -3)
	const pushedText = "2026-11-03T12:00:00Z"
	moved := registry.TagBuild{Digest: "sha256:new", Pushed: pushed}
	tests := []struct {
		name          string
		tag           string
		running       string
		digests       []string
		builtDate     string
		build         registry.TagBuild
		buildErr      error
		lookupErr     error
		wantAvailable bool
		wantDate      string
		wantAsked     int
	}{
		{name: "the tag has moved past the running image", tag: "11.4", running: "11.4", digests: []string{"sha256:old"}, build: moved, wantAvailable: true, wantDate: pushedText, wantAsked: 1},
		{name: "the running image is current", tag: "11.4", running: "11.4", digests: []string{"sha256:new"}, build: moved, wantAsked: 1},
		{name: "a floating tag that moved", tag: "latest", running: "2.3.4", digests: []string{"sha256:old"}, build: moved, wantAvailable: true, wantDate: pushedText, wantAsked: 1},
		{name: "a running build date after the push isn't newer", tag: "11.4", running: "11.4", digests: []string{"sha256:old"}, builtDate: "2026-11-05T12:00:00Z", build: moved, wantAsked: 1},
		{name: "an unreadable running build date still compares digests", tag: "11.4", running: "11.4", digests: []string{"sha256:old"}, builtDate: "yesterday", build: moved, wantAvailable: true, wantDate: pushedText, wantAsked: 1},
		{name: "a registry failure claims nothing", tag: "11.4", running: "11.4", digests: []string{"sha256:old"}, buildErr: errors.New("status 429"), wantAsked: 1},
		{name: "no date from the registry", tag: "11.4", running: "11.4", digests: []string{"sha256:old"}, build: registry.TagBuild{Digest: "sha256:new"}, wantAvailable: true, wantAsked: 1},
		{name: "a locally built image isn't checked", tag: "11.4", running: "11.4", build: moved},
		{name: "a stopped container isn't checked", tag: "11.4", build: moved},
		{name: "a container on another tag isn't checked", tag: "1.2.4", running: "1.2.3", digests: []string{"sha256:old"}, build: moved},
		{name: "a failed lookup isn't checked", tag: "11.4", running: "11.4", digests: []string{"sha256:old"}, build: moved, lookupErr: errors.New("status 500")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &buildRegistry{build: tt.build, buildErr: tt.buildErr}
			fake.err = tt.lookupErr
			r := &recommender{registries: []registry.Registry{fake}, safeDays: 3, now: func() time.Time { return testNow }}
			info := models.ImageInfo{
				Image:            models.Image{Registry: "docker.io", Name: "mariadb", Tag: tt.tag},
				RunningVersion:   tt.running,
				RunningDigests:   tt.digests,
				RunningBuildDate: tt.builtDate,
			}

			r.recommend(&info)

			if info.NewerBuildAvailable != tt.wantAvailable {
				t.Errorf("NewerBuildAvailable = %v, want %v", info.NewerBuildAvailable, tt.wantAvailable)
			}
			if info.NewerBuildDate != tt.wantDate {
				t.Errorf("NewerBuildDate = %q, want %q", info.NewerBuildDate, tt.wantDate)
			}
			if fake.asked != tt.wantAsked {
				t.Errorf("asked the registry %d times, want %d", fake.asked, tt.wantAsked)
			}
		})
	}
}

func TestMarkTagMismatch(t *testing.T) {
	info := models.ImageInfo{
		Image:                  models.Image{Name: "postgres", Tag: "16.14-alpine"},
		RunningVersion:         "16-alpine",
		ResolvedRunningVersion: "16.15-alpine",
	}
	markTagMismatch(&info)
	if !info.TagMismatch || !info.TagMismatchDowngrade {
		t.Errorf("TagMismatch = %v, TagMismatchDowngrade = %v, want both true", info.TagMismatch, info.TagMismatchDowngrade)
	}
}

// lookupCounter counts registry lookups.
type lookupCounter struct {
	fakeRegistry
	lookups int
}

func (c *lookupCounter) FetchVersions(models.Image) ([]models.RegistryVersion, error) {
	c.lookups++
	return []models.RegistryVersion{{Tag: "9.9.9", ReleasedAt: testNow.AddDate(0, -1, 0)}}, nil
}

func TestRecommend_UnresolvedVariableSkipsTheLookup(t *testing.T) {
	counter := &lookupCounter{}
	r := &recommender{registries: []registry.Registry{counter}, safeDays: 3, now: func() time.Time { return testNow }}
	const note = "unresolved variable: TAG"
	info := models.ImageInfo{
		Image:               models.Image{Registry: "ghcr.io", Name: "org/app", Tag: "${TAG}"},
		UnresolvedVariables: []string{"TAG"},
		Unverified:          true,
		RecommendationNote:  note,
	}

	r.recommend(&info)

	if counter.lookups != 0 {
		t.Errorf("made %d registry lookups, want none", counter.lookups)
	}
	if info.RecommendedVersion != "" {
		t.Errorf("RecommendedVersion = %q, want empty", info.RecommendedVersion)
	}
	if info.RecommendationNote != note {
		t.Errorf("RecommendationNote = %q, want %q", info.RecommendationNote, note)
	}
	if got := report.ClassifyImage(info); got != report.StatusUnverified {
		t.Errorf("ClassifyImage = %v, want StatusUnverified", got)
	}
}

func TestRecommend_NoteReachesImageInfo(t *testing.T) {
	r := newTestRecommender([]models.RegistryVersion{
		{Tag: "2.11.9", ReleasedAt: testNow.Add(-time.Hour)},
	})
	info := models.ImageInfo{Image: models.Image{Registry: "docker.io", Name: "caddy", Tag: "2.11.7"}}

	r.recommend(&info)

	if info.RecommendedVersion != "" {
		t.Errorf("RecommendedVersion = %q, want empty", info.RecommendedVersion)
	}
	if info.RecommendationNote == "" {
		t.Error("RecommendationNote is empty, want the reason nothing was recommended")
	}
}

// TestRecommend_DisplayCapDoesNotLimitPicking runs the #24 case end to end:
// the shown list is capped at 10 but the pick comes from further down.
func TestRecommend_DisplayCapDoesNotLimitPicking(t *testing.T) {
	var versions []models.RegistryVersion
	for i := 19; i >= 1; i-- {
		versions = append(versions, models.RegistryVersion{
			Tag:        fmt.Sprintf("2.%d.0", i),
			ReleasedAt: testNow.Add(-time.Hour),
		})
	}
	versions = append(versions, models.RegistryVersion{Tag: "2.0.0", ReleasedAt: testNow.AddDate(0, 0, -10)})

	info := models.ImageInfo{Image: models.Image{Registry: "docker.io", Name: "app", Tag: "1.0.0"}}
	newTestRecommender(versions).recommend(&info)

	if len(info.AvailableVersions) != 10 {
		t.Errorf("AvailableVersions has %d tags, want 10", len(info.AvailableVersions))
	}
	if info.RecommendedVersion != "2.0.0" {
		t.Errorf("RecommendedVersion = %q, want 2.0.0", info.RecommendedVersion)
	}
}

func TestRecommend_NewerMajorReachesImageInfo(t *testing.T) {
	info := models.ImageInfo{Image: models.Image{Registry: "docker.io", Name: "mariadb", Tag: "11.4"}}
	newTestRecommender([]models.RegistryVersion{
		{Tag: "13.0.2", ReleasedAt: testNow.AddDate(0, 0, -10)},
		{Tag: "11.4.9", ReleasedAt: testNow.AddDate(0, 0, -10)},
	}).recommend(&info)

	if info.RecommendedVersion != "11.4.9" {
		t.Errorf("RecommendedVersion = %q, want 11.4.9", info.RecommendedVersion)
	}
	if info.NewerMajorVersion != "13.0.2" {
		t.Errorf("NewerMajorVersion = %q, want 13.0.2", info.NewerMajorVersion)
	}
}

// overrideRecommender serves one GitHub release, published long enough ago to
// pass safe-days at any real clock, through a fake getter.
func overrideRecommender(versions []models.RegistryVersion, image string) *recommender {
	r := newTestRecommender(versions)
	r.overrides = &overrides.Config{Overrides: []overrides.Override{
		{Image: image, Source: overrides.SourceGitHubRelease, Repo: "example/app"},
	}}
	r.getter = func(string) ([]byte, error) {
		return []byte(`[{"tag_name":"v2.0.0","published_at":"2020-01-01T00:00:00Z"}]`), nil
	}
	return r
}

func TestRecommend_Override(t *testing.T) {
	old := testNow.AddDate(0, 0, -10)
	tests := []struct {
		name     string
		pin      string
		running  string
		versions []models.RegistryVersion
		want     string
		wantJump bool
	}{
		{
			name:     "override keeps the pinned variant and flags the major jump",
			pin:      "1.0.0-alpine",
			versions: []models.RegistryVersion{{Tag: "2.0.0", ReleasedAt: old}, {Tag: "2.0.0-alpine", ReleasedAt: old}},
			want:     "2.0.0-alpine",
			wantJump: true,
		},
		{
			name:     "override below the running version falls back to the registry path",
			pin:      "1.0.0",
			running:  "3.0.0",
			versions: []models.RegistryVersion{{Tag: "3.0.0", ReleasedAt: old}, {Tag: "2.0.0", ReleasedAt: old}},
			want:     "3.0.0",
			wantJump: true,
		},
		{
			name:     "override off the pinned line falls back to the registry path",
			pin:      "1.4",
			versions: []models.RegistryVersion{{Tag: "2.0.0", ReleasedAt: old}, {Tag: "1.4.9", ReleasedAt: old}},
			want:     "1.4.9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := models.ImageInfo{Image: models.Image{Registry: "docker.io", Name: "app", Tag: tt.pin}, RunningVersion: tt.running}
			overrideRecommender(tt.versions, "docker.io/app").recommend(&info)
			if info.RecommendedVersion != tt.want {
				t.Errorf("RecommendedVersion = %q, want %q", info.RecommendedVersion, tt.want)
			}
			if info.MajorVersionJump != tt.wantJump {
				t.Errorf("MajorVersionJump = %v, want %v", info.MajorVersionJump, tt.wantJump)
			}
		})
	}
}

// TestRecommend_NeverBelowRunning checks the fields and status the report sees.
func TestRecommend_NeverBelowRunning(t *testing.T) {
	yesterday := testNow.AddDate(0, 0, -1)
	yearAgo := testNow.AddDate(-1, 0, 0)
	tests := []struct {
		name       string
		image      string
		pin        string
		running    string
		versions   []models.RegistryVersion
		wantStatus report.ImageStatus
	}{
		{
			name:       "phpmyadmin re-pushed yesterday is up to date",
			image:      "phpmyadmin",
			pin:        "5.2.3",
			running:    "5.2.3",
			versions:   []models.RegistryVersion{{Tag: "5.2.3", ReleasedAt: yesterday}, {Tag: "5.2.2", ReleasedAt: yearAgo}},
			wantStatus: report.StatusGood,
		},
		{
			name:       "redis pushed today gets no move down",
			image:      "redis",
			pin:        "8.10.3-alpine",
			running:    "8.10.3-alpine",
			versions:   []models.RegistryVersion{{Tag: "8.10.3-alpine", ReleasedAt: testNow}, {Tag: "8.10.2-alpine", ReleasedAt: yearAgo}},
			wantStatus: report.StatusGood,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := models.ImageInfo{
				Image:          models.Image{Registry: "docker.io", Name: tt.image, Tag: tt.pin},
				RunningVersion: tt.running,
			}
			newTestRecommender(tt.versions).recommend(&info)

			if info.RecommendedVersion != "" {
				t.Errorf("RecommendedVersion = %q, want empty", info.RecommendedVersion)
			}
			if info.IsDowngrade {
				t.Error("IsDowngrade = true, want false")
			}
			if got := report.ClassifyImage(info); got != tt.wantStatus {
				t.Errorf("ClassifyImage = %v, want %v", got, tt.wantStatus)
			}
		})
	}
}

// TestRecommend_FailedLookupSaysSo comes from a real scan: docker.n8n.io returned
// 401 and the image showed a green tick with no explanation.
func TestRecommend_FailedLookupSaysSo(t *testing.T) {
	unauthorized := errors.New("docker.n8n.io: registry returned status 401")
	tests := []struct {
		name  string
		setup func(r *recommender)
	}{
		{"registry path", func(*recommender) {}},
		{"override path falling through to the registry path", func(r *recommender) {
			r.overrides = &overrides.Config{Overrides: []overrides.Override{
				{Image: "docker.n8n.io/n8nio/n8n", Source: overrides.SourceGitHubRelease, Repo: "n8n-io/n8n"},
			}}
			r.getter = func(string) ([]byte, error) {
				return []byte(`[{"tag_name":"v3.1.0","published_at":"2020-01-01T00:00:00Z"}]`), nil
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRecommender(nil)
			r.registries = []registry.Registry{&fakeRegistry{err: unauthorized}}
			tt.setup(r)

			info := models.ImageInfo{Image: models.Image{Registry: "docker.n8n.io", Name: "n8nio/n8n", Tag: "3.0.1"}}
			r.recommend(&info)

			if info.RecommendedVersion != "" {
				t.Errorf("RecommendedVersion = %q, want empty", info.RecommendedVersion)
			}
			if got := report.ClassifyImage(info); got != report.StatusUnverified {
				t.Errorf("ClassifyImage = %v, want StatusUnverified", got)
			}
			for _, want := range []string{"couldn't check", "401"} {
				if !strings.Contains(info.RecommendationNote, want) {
					t.Errorf("RecommendationNote = %q, want it to mention %q", info.RecommendationNote, want)
				}
			}
		})
	}
}

// TestRecommend_FindsTheRealRunningVersion covers tags that don't say what's
// running. The postgres and mariadb digests match what Docker Hub publishes, and
// a real container's digest was confirmed to match them.
func TestRecommend_FindsTheRealRunningVersion(t *testing.T) {
	dayOld := testNow.AddDate(0, 0, -1)
	old := testNow.AddDate(0, -4, 0)
	postgres := []models.RegistryVersion{
		{Tag: "18.4-alpine", ReleasedAt: old, Digest: "sha256:pg18"},
		{Tag: "16.15-alpine", ReleasedAt: dayOld, Digest: "sha256:pg1615"},
		{Tag: "16.14-alpine", ReleasedAt: old, Digest: "sha256:pg1614"},
	}
	mariadb := []models.RegistryVersion{
		{Tag: "11.4.13", ReleasedAt: old, Digest: "sha256:m13"},
		{Tag: "11.4.12", ReleasedAt: old.AddDate(0, -1, 0), Digest: "sha256:m12"},
	}
	tests := []struct {
		name         string
		pin          string
		usesLatest   bool
		running      string
		digests      []string
		label        string
		versions     []models.RegistryVersion
		want         string
		wantResolved string
		wantMajor    string
		wantStatus   report.ImageStatus
		wantNote     string
	}{
		{
			name: "really on 16.15 is never offered 16.14", pin: "16-alpine", running: "16-alpine",
			digests: []string{"sha256:pg1615"}, versions: postgres,
			wantResolved: "16.15-alpine", wantMajor: "18.4-alpine", wantStatus: report.StatusGood,
			wantNote: "already on the newest version",
		},
		{
			name: "really on 16.14 stays there while 16.15 waits", pin: "16-alpine", running: "16-alpine",
			digests: []string{"sha256:pg1614"}, versions: postgres,
			want: "16.14-alpine", wantResolved: "16.14-alpine", wantMajor: "18.4-alpine", wantStatus: report.StatusGood,
			wantNote: "newest is 16.15-alpine",
		},
		{
			name: "an image rebuilt since it was pulled can't be placed", pin: "16-alpine", running: "16-alpine",
			digests: []string{"sha256:pulled-months-ago"}, versions: postgres,
			wantMajor: "18.4-alpine", wantStatus: report.StatusUnverified,
			wantNote: "can't tell which version is running",
		},
		{
			name: "nothing running can take the newest safe tag", pin: "16-alpine",
			versions: postgres,
			want:     "16.14-alpine", wantMajor: "18.4-alpine", wantStatus: report.StatusUpgrade,
		},
		{
			name: "11.4 really on 11.4.13 is up to date, not an upgrade", pin: "11.4", running: "11.4",
			digests: []string{"sha256:m13"}, versions: mariadb,
			want: "11.4.13", wantResolved: "11.4.13", wantStatus: report.StatusGood,
		},
		{
			name: "11.4 really on 11.4.12 has an upgrade", pin: "11.4", running: "11.4",
			digests: []string{"sha256:m12"}, versions: mariadb,
			want: "11.4.13", wantResolved: "11.4.12", wantStatus: report.StatusUpgrade,
		},
		{
			name: "a version label places a partial tag", pin: "11.4", running: "11.4",
			label: "11.4.13", versions: mariadb,
			want: "11.4.13", wantResolved: "11.4.13", wantStatus: report.StatusGood,
		},
		{
			name: "a floating tag with a digest match is held to it", pin: "latest", usesLatest: true, running: "unknown",
			digests: []string{"sha256:m13"}, versions: mariadb,
			want: "11.4.13", wantResolved: "11.4.13", wantStatus: report.StatusFloating,
		},
		{
			name: "a floating tag that can't be placed gets no recommendation", pin: "latest", usesLatest: true, running: "unknown",
			versions:   mariadb,
			wantStatus: report.StatusFloating,
			wantNote:   "can't tell which version is running",
		},
		{
			name: "a full version pin needs no lookup", pin: "11.4.12", running: "11.4.12",
			versions: mariadb,
			want:     "11.4.13", wantStatus: report.StatusUpgrade,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := models.ImageInfo{
				Image:               models.Image{Registry: "docker.io", Name: "app", Tag: tt.pin},
				UsesLatest:          tt.usesLatest,
				RunningVersion:      tt.running,
				RunningDigests:      tt.digests,
				RunningLabelVersion: tt.label,
			}
			newTestRecommender(tt.versions).recommend(&info)

			if info.RecommendedVersion != tt.want {
				t.Errorf("RecommendedVersion = %q, want %q (note %q)", info.RecommendedVersion, tt.want, info.RecommendationNote)
			}
			if info.ResolvedRunningVersion != tt.wantResolved {
				t.Errorf("ResolvedRunningVersion = %q, want %q", info.ResolvedRunningVersion, tt.wantResolved)
			}
			if info.NewerMajorVersion != tt.wantMajor {
				t.Errorf("NewerMajorVersion = %q, want %q", info.NewerMajorVersion, tt.wantMajor)
			}
			if got := report.ClassifyImage(info); got != tt.wantStatus {
				t.Errorf("ClassifyImage = %v, want %v", got, tt.wantStatus)
			}
			if !strings.Contains(info.RecommendationNote, tt.wantNote) {
				t.Errorf("RecommendationNote = %q, want it to mention %q", info.RecommendationNote, tt.wantNote)
			}
		})
	}
}

// TestRecommend_OverrideRespectsTheRealRunningVersion keeps the override path to
// the same limit as the registry path.
func TestRecommend_OverrideRespectsTheRealRunningVersion(t *testing.T) {
	old := testNow.AddDate(0, 0, -10)
	versions := []models.RegistryVersion{
		{Tag: "3.0.0", ReleasedAt: old, Digest: "sha256:three"},
		{Tag: "2.0.0", ReleasedAt: old, Digest: "sha256:two"},
	}
	tests := []struct {
		name     string
		digests  []string
		want     string
		wantNote string
	}{
		{"running above the override's release keeps the registry's answer", []string{"sha256:three"}, "3.0.0", ""},
		{"can't be placed, so the override doesn't get to guess", nil, "", "can't tell which version is running"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := models.ImageInfo{
				Image:          models.Image{Registry: "docker.io", Name: "app", Tag: "latest"},
				RunningVersion: "unknown",
				RunningDigests: tt.digests,
			}
			overrideRecommender(versions, "docker.io/app").recommend(&info)
			if tt.want == "" && info.RecommendedVersion != "" {
				t.Errorf("RecommendedVersion = %q, want empty", info.RecommendedVersion)
			}
			if tt.want != "" && info.RecommendedVersion != tt.want {
				t.Errorf("RecommendedVersion = %q, want %q", info.RecommendedVersion, tt.want)
			}
			if !strings.Contains(info.RecommendationNote, tt.wantNote) {
				t.Errorf("RecommendationNote = %q, want it to mention %q", info.RecommendationNote, tt.wantNote)
			}
		})
	}
}
