package main

import (
	"docker-scanner/pkg/models"
	"docker-scanner/pkg/registry"
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
