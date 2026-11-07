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
			for _, want := range []string{"couldn't check", "401"} {
				if !strings.Contains(info.RecommendationNote, want) {
					t.Errorf("RecommendationNote = %q, want it to mention %q", info.RecommendationNote, want)
				}
			}
		})
	}
}
