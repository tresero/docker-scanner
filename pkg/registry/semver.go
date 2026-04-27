package registry

import (
	"docker-scanner/internal/config"
	"docker-scanner/pkg/models"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// semverRegex matches tags that look like real version numbers:
// 1.2.3, v1.2.3, 1.2, v1.2, 1.2.3.4 (4-part like .NET apps)
// with optional short suffixes like -alpine, -slim, -ls75
// Requires at least one dot to avoid matching pure numbers like "2026041305"
var semverRegex = regexp.MustCompile(
	`^v?(\d+)\.(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-([\w]+(?:\.[\w]+)*))?$`,
)

// longNumericSuffix catches tags like 13.1.0-24643103163
var longNumericSuffix = regexp.MustCompile(`-\d{6,}$`)

// gitHashSuffix catches tags like 2.18.3-f32cb56 (hex hash after dash)
var gitHashSuffix = regexp.MustCompile(`-[0-9a-f]{7,}$`)

// wordPrefixRegex catches tags like "develop-4.0.17" or "version-4.0.17"
var wordPrefixRegex = regexp.MustCompile(`^[a-zA-Z]+-\d`)

// devSuffixes are tags to deprioritize
var devSuffixes = []string{"-dev", "-rc", "-beta", "-alpha", "-snapshot", "-distroless", "-busybox", "-ubuntu"}

// archPrefixes are tag prefixes that indicate architecture-specific builds
var archPrefixes = []string{"amd64-", "arm64v8-", "arm32v7-", "arm64-", "arm-"}

// IsSemver returns true if a tag looks like a real version number
func IsSemver(tag string) bool {
	lower := strings.ToLower(tag)

	// Skip arch-specific tags
	for _, prefix := range archPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}

	// Skip tags with word prefixes like "develop-4.0.17" or "version-4.0.17"
	// but allow "v1.2.3"
	if wordPrefixRegex.MatchString(tag) && !strings.HasPrefix(lower, "v") {
		return false
	}

	if !semverRegex.MatchString(tag) {
		return false
	}

	// Reject tags with very long numeric suffixes (build IDs)
	if longNumericSuffix.MatchString(tag) {
		return false
	}

	// Reject tags with git hash suffixes (e.g., 2.18.3-f32cb56)
	if gitHashSuffix.MatchString(tag) {
		return false
	}

	return true
}

// IsCleanTag returns true if the tag has no dev/rc/distroless suffix
func IsCleanTag(tag string) bool {
	lower := strings.ToLower(tag)
	for _, suffix := range devSuffixes {
		if strings.Contains(lower, suffix) {
			return false
		}
	}
	return true
}

// SemverParts extracts major, minor, patch from a tag.
// For 4-part versions (e.g., 4.0.17.2952), only uses first 3 parts.
// Returns -1 for missing parts.
func SemverParts(tag string) (int, int, int) {
	matches := semverRegex.FindStringSubmatch(tag)
	if matches == nil {
		return -1, -1, -1
	}

	major := atoi(matches[1])
	minor := atoi(matches[2])
	patch := -1

	if matches[3] != "" {
		patch = atoi(matches[3])
	}

	return major, minor, patch
}

// CompareSemver compares two semver tags.
// Returns positive if a > b, negative if a < b, 0 if equal.
func CompareSemver(a, b string) int {
	aMaj, aMin, aPat := SemverParts(a)
	bMaj, bMin, bPat := SemverParts(b)

	if aMaj != bMaj {
		return aMaj - bMaj
	}
	if aMin != bMin {
		return aMin - bMin
	}

	if aPat == -1 {
		aPat = 0
	}
	if bPat == -1 {
		bPat = 0
	}
	if aPat != bPat {
		return aPat - bPat
	}

	// Prefer clean tags over any suffixed ones
	// e.g., "0.21.0" > "0.21.0-rocm", "2.9.2" > "2.9.2-fat"
	aHasSuffix := strings.Contains(strings.TrimPrefix(a, "v"), "-")
	bHasSuffix := strings.Contains(strings.TrimPrefix(b, "v"), "-")
	if !aHasSuffix && bHasSuffix {
		return 1
	}
	if aHasSuffix && !bHasSuffix {
		return -1
	}

	return 0
}

// FilterAndSortTags takes raw tag strings, filters to valid semver,
// removes unsafe tags, sorts descending, and limits results.
// This is the single source of truth for tag processing.
func FilterAndSortTags(tags []string) []models.RegistryVersion {
	var versions []models.RegistryVersion

	for _, tag := range tags {
		if isUnsafeTag(tag) || !IsSemver(tag) {
			continue
		}
		versions = append(versions, models.RegistryVersion{Tag: tag})
	}

	sort.Slice(versions, func(i, j int) bool {
		return CompareSemver(versions[i].Tag, versions[j].Tag) > 0
	})

	if len(versions) > config.VersionFetchLimit {
		versions = versions[:config.VersionFetchLimit]
	}

	return versions
}

// FilterAndSortDockerHubTags is like FilterAndSortTags but accepts
// the Docker Hub response format with timestamps.
func FilterAndSortDockerHubTags(tags []DockerHubTag) []models.RegistryVersion {
	var versions []models.RegistryVersion

	for _, tag := range tags {
		if isUnsafeTag(tag.Name) || !IsSemver(tag.Name) {
			continue
		}
		v := models.RegistryVersion{
			Tag:      tag.Name,
			Released: tag.LastUpdated,
		}
		v.ReleasedAt = parseTimestamp(tag.LastUpdated)
		versions = append(versions, v)
	}

	sort.Slice(versions, func(i, j int) bool {
		return CompareSemver(versions[i].Tag, versions[j].Tag) > 0
	})

	if len(versions) > config.VersionFetchLimit {
		versions = versions[:config.VersionFetchLimit]
	}

	return versions
}

// parseTimestamp tries common registry timestamp formats
func parseTimestamp(s string) time.Time {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05.999999Z",
		"2006-01-02T15:04:05Z",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func isUnsafeTag(tag string) bool {
	lower := strings.ToLower(tag)
	for _, unsafe := range config.UnsafeTagPatterns {
		if lower == unsafe {
			return true
		}
	}
	return false
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}