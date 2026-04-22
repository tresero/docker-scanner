package registry

import "testing"

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