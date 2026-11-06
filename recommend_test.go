package main

import (
	"docker-scanner/pkg/models"
	"docker-scanner/pkg/overrides"
	"docker-scanner/pkg/registry"
	"fmt"
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
			name:     "override off the pinned line falls back to the registry path",
			pin:      "1.4",
			versions: []models.RegistryVersion{{Tag: "2.0.0", ReleasedAt: old}, {Tag: "1.4.9", ReleasedAt: old}},
			want:     "1.4.9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := models.ImageInfo{Image: models.Image{Registry: "docker.io", Name: "app", Tag: tt.pin}}
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
