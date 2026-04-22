package registry

import (
    "docker-scanner/pkg/models"
    "testing"
    "time"
)

func TestPickSafeVersion_SkipsTooNew(t *testing.T) {
    now := time.Now()
    versions := []models.RegistryVersion{
	{Tag: "2.0.0", ReleasedAt: now.Add(-1 * time.Hour)},       // 1 hour ago — too new
	{Tag: "1.9.0", ReleasedAt: now.Add(-4 * 24 * time.Hour)},  // 4 days ago — safe
	{Tag: "1.8.0", ReleasedAt: now.Add(-10 * 24 * time.Hour)}, // 10 days ago
    }

    result := PickSafeVersion(versions, "latest", 3)

    if result.Version != "1.9.0" {
	t.Errorf("expected 1.9.0, got %s", result.Version)
    }
}

func TestPickSafeVersion_SkipsPreRelease(t *testing.T) {
    now := time.Now()
    versions := []models.RegistryVersion{
	{Tag: "2.0.0", ReleasedAt: now.Add(-4 * 24 * time.Hour)},      // safe but newest
	{Tag: "2.0.0-rc1", ReleasedAt: now.Add(-5 * 24 * time.Hour)},  // pre-release
	{Tag: "2.0.0-beta", ReleasedAt: now.Add(-6 * 24 * time.Hour)}, // pre-release
	{Tag: "1.9.0", ReleasedAt: now.Add(-10 * 24 * time.Hour)},     // stable
    }

    result := PickSafeVersion(versions, "latest", 3)

    if result.Version != "2.0.0" {
	t.Errorf("expected 2.0.0, got %s", result.Version)
    }
}

func TestPickSafeVersion_AllTooNew(t *testing.T) {
    now := time.Now()
    versions := []models.RegistryVersion{
	{Tag: "2.0.0", ReleasedAt: now.Add(-1 * time.Hour)},
	{Tag: "1.9.0", ReleasedAt: now.Add(-2 * time.Hour)},
    }

    // Everything is too new, falls back to position-based
    result := PickSafeVersion(versions, "latest", 3)

    // Position fallback skips first, takes second
    if result.Version != "1.9.0" {
	t.Errorf("expected 1.9.0 from position fallback, got %s", result.Version)
    }
}

func TestPickSafeVersion_DetectsMajorJump(t *testing.T) {
    now := time.Now()
    versions := []models.RegistryVersion{
	{Tag: "3.0.0", ReleasedAt: now.Add(-5 * 24 * time.Hour)},
	{Tag: "2.5.0", ReleasedAt: now.Add(-30 * 24 * time.Hour)},
    }

    result := PickSafeVersion(versions, "2.5.0", 3)

    if result.Version != "3.0.0" {
	t.Errorf("expected 3.0.0, got %s", result.Version)
    }
    if !result.MajorJump {
	t.Error("expected MajorJump to be true")
    }
}

func TestPickSafeVersion_NoMajorJump(t *testing.T) {
    now := time.Now()
    versions := []models.RegistryVersion{
	{Tag: "2.6.0", ReleasedAt: now.Add(-5 * 24 * time.Hour)},
	{Tag: "2.5.0", ReleasedAt: now.Add(-30 * 24 * time.Hour)},
    }

    result := PickSafeVersion(versions, "2.5.0", 3)

    if result.MajorJump {
	t.Error("expected MajorJump to be false")
    }
}

func TestPickSafeVersion_LatestTagNoMajorJump(t *testing.T) {
    now := time.Now()
    versions := []models.RegistryVersion{
	{Tag: "3.0.0", ReleasedAt: now.Add(-5 * 24 * time.Hour)},
    }

    result := PickSafeVersion(versions, "latest", 3)

    if result.MajorJump {
	t.Error("expected MajorJump to be false when current tag is 'latest'")
    }
}

func TestPickSafeVersion_Empty(t *testing.T) {
    result := PickSafeVersion(nil, "latest", 3)

    if result.Version != "" {
	t.Errorf("expected empty version, got %s", result.Version)
    }
}

func TestPickSafeVersion_PositionFallbackSkipsPreRelease(t *testing.T) {
    // No dates — uses position fallback
    versions := []models.RegistryVersion{
	{Tag: "2.0.0"},
	{Tag: "2.0.0-rc1"},
	{Tag: "1.9.0"},
	{Tag: "1.8.0"},
    }

    result := PickSafeVersion(versions, "latest", 3)

    // Should skip first stable (2.0.0) and all pre-release, pick 1.9.0
    if result.Version != "1.9.0" {
	t.Errorf("expected 1.9.0, got %s", result.Version)
    }
}

func TestIsPreRelease(t *testing.T) {
    tests := []struct {
	tag  string
	want bool
    }{
	{"1.0.0", false},
	{"1.0.0-rc1", true},
	{"1.0.0-beta", true},
	{"1.0.0-alpha", true},
	{"1.0.0-dev", true},
	{"1.0.0-snapshot", true},
	{"1.0.0-pre", true},
	{"1.0.0-fat", false},
	{"1.0.0-alpine", false},
	{"1.0.0-rocm", false},
	{"v3.11.2-distroless", false},
    }

    for _, tt := range tests {
	t.Run(tt.tag, func(t *testing.T) {
	    got := isPreRelease(tt.tag)
	    if got != tt.want {
		t.Errorf("isPreRelease(%q) = %v, want %v", tt.tag, got, tt.want)
	    }
	})
    }
}

func TestFormatAge(t *testing.T) {
    now := time.Now()
    tests := []struct {
	released time.Time
	want     string
    }{
	{now.Add(-2 * time.Hour), "2 hours"},
	{now.Add(-24 * time.Hour), "1 day"},
	{now.Add(-3 * 24 * time.Hour), "3 days"},
	{now.Add(-7 * 24 * time.Hour), "1 week"},
	{now.Add(-14 * 24 * time.Hour), "2 weeks"},
	{now.Add(-35 * 24 * time.Hour), "1 month"},
	{now.Add(-400 * 24 * time.Hour), "1 year"},
    }

    for _, tt := range tests {
	t.Run(tt.want, func(t *testing.T) {
	    got := formatAge(now, tt.released)
	    if got != tt.want {
		t.Errorf("formatAge() = %q, want %q", got, tt.want)
	    }
	})
    }
}