package registry

import (
	"docker-scanner/pkg/models"
	"testing"
)

func TestIsSemver(t *testing.T) {
	tests := []struct {
		tag  string
		want bool
	}{
		// Valid semver
		{"1.2.3", true},
		{"v1.2.3", true},
		{"1.2", true},
		{"v1.2", true},
		{"11.17.3", true},
		{"2026.3.0", true},
		{"0.21.0", true},
		{"v0.49.1", true},
		{"2.9.2-fat", true},
		{"1.52-dev", true},
		{"v3.11.2-distroless", true},
		{"4.0.17.2952", true},
		{"3.0.4.1002-ls75", true},

		// Invalid — pure numbers
		{"2026041305", false},
		{"latest", false},
		{"main", false},
		{"develop", false},
		{"stable", false},
		{"nightly", false},
		{"edge", false},

		// Invalid — arch prefixes
		{"amd64-4.0.17", false},
		{"arm64v8-develop", false},
		{"arm64-1.2.3", false},

		// Invalid — word prefixes
		{"develop-4.0.17", false},
		{"version-4.0.17", false},

		// Invalid — long numeric suffixes
		{"13.1.0-24643103163", false},

		// Invalid — git hash suffixes
		{"2.18.3-f32cb56", false},
		{"1.0.0-a1b2c3d", false},
		{"2.18.3-f32cb56a9345", false},

		// Invalid — no dots
		{"rocm", false},
		{"alpha", false},
		{"unstable", false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			got := IsSemver(tt.tag)
			if got != tt.want {
				t.Errorf("IsSemver(%q) = %v, want %v", tt.tag, got, tt.want)
			}
		})
	}
}

func TestSemverParts(t *testing.T) {
	tests := []struct {
		tag              string
		major, minor, patch int
	}{
		{"1.2.3", 1, 2, 3},
		{"v1.2.3", 1, 2, 3},
		{"11.17.3", 11, 17, 3},
		{"1.2", 1, 2, -1},
		{"v0.49.1", 0, 49, 1},
		{"2026.3.0", 2026, 3, 0},
		{"invalid", -1, -1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			maj, min, pat := SemverParts(tt.tag)
			if maj != tt.major || min != tt.minor || pat != tt.patch {
				t.Errorf("SemverParts(%q) = (%d, %d, %d), want (%d, %d, %d)",
					tt.tag, maj, min, pat, tt.major, tt.minor, tt.patch)
			}
		})
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int // positive = a > b, negative = a < b, 0 = equal
	}{
		// Higher major
		{"2.0.0", "1.0.0", 1},
		// Higher minor
		{"1.2.0", "1.1.0", 1},
		// Higher patch
		{"1.1.2", "1.1.1", 1},
		// Equal
		{"1.2.3", "1.2.3", 0},
		// Clean beats suffixed
		{"0.21.0", "0.21.0-rocm", 1},
		{"2.9.2", "2.9.2-fat", 1},
		// Suffixed loses to clean
		{"0.21.0-rocm", "0.21.0", -1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := CompareSemver(tt.a, tt.b)
			if (tt.want > 0 && got <= 0) || (tt.want < 0 && got >= 0) || (tt.want == 0 && got != 0) {
				t.Errorf("CompareSemver(%q, %q) = %d, want sign %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestFilterAndSortTags(t *testing.T) {
	tags := []string{
		"latest", "develop", "unstable",
		"1.0.0", "2.0.0", "1.5.0",
		"2.0.0-rc1", "amd64-2.0.0",
		"rocm", "alpha",
	}

	versions := FilterAndSortTags(tags)

	if len(versions) != 4 {
		t.Fatalf("expected 4 versions, got %d", len(versions))
	}

	// Should be sorted descending
	if versions[0].Tag != "2.0.0" {
		t.Errorf("expected first version to be 2.0.0, got %s", versions[0].Tag)
	}
	if versions[1].Tag != "2.0.0-rc1" {
		t.Errorf("expected second version to be 2.0.0-rc1, got %s", versions[1].Tag)
	}
	if versions[2].Tag != "1.5.0" {
		t.Errorf("expected third version to be 1.5.0, got %s", versions[2].Tag)
	}
}

func TestFilterAndSortTags_RemovesPartialWhenSpecificExists(t *testing.T) {
	tags := []string{
		"5.0.2", "5.0.1", "5.0", "5.0.0",
		"4.6.0", "4.6",
	}

	versions := FilterAndSortTags(tags)

	for _, v := range versions {
		if v.Tag == "5.0" || v.Tag == "4.6" {
			t.Errorf("partial tag %q should have been filtered out", v.Tag)
		}
	}

	// Should still include the specific versions
	tagSet := make(map[string]bool)
	for _, v := range versions {
		tagSet[v.Tag] = true
	}
	for _, want := range []string{"5.0.2", "5.0.1", "5.0.0", "4.6.0"} {
		if !tagSet[want] {
			t.Errorf("expected %q to be present", want)
		}
	}
}

func TestFilterAndSortTags_KeepsPartialWhenNoSpecific(t *testing.T) {
	// If a project only uses 2-part versioning, keep them
	tags := []string{"1.22", "1.21", "1.20"}

	versions := FilterAndSortTags(tags)

	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d: %v", len(versions), versions)
	}
}

func TestMatchDockerTag(t *testing.T) {
	versions := []models.RegistryVersion{
		{Tag: "13.0.1"},
		{Tag: "13.0.1-ubuntu"},
		{Tag: "12.4.3"},
		{Tag: "4.0.17.2952-ls309"},
	}

	tests := []struct {
		upstream string
		want     string
	}{
		// Exact match
		{"13.0.1", "13.0.1"},
		// Strip v prefix
		{"v13.0.1", "13.0.1"},
		// Strip build metadata
		{"v13.0.1+security-01", "13.0.1"},
		// Substring match (LinuxServer-style)
		{"v4.0.17.2952", "4.0.17.2952-ls309"},
		// No match
		{"v99.0.0", ""},
	}

	for _, tt := range tests {
		t.Run(tt.upstream, func(t *testing.T) {
			got := MatchDockerTag(versions, tt.upstream)
			if got != tt.want {
				t.Errorf("MatchDockerTag(%q) = %q, want %q", tt.upstream, got, tt.want)
			}
		})
	}
}