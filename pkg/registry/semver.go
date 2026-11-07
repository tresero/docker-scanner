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

// versionRegex is semverRegex with any suffix, including multi-part ones like
// 8.3.2-fpm-alpine. Tags that only fit this one are for pins of that variant.
var versionRegex = regexp.MustCompile(
	`^v?(\d+)\.(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-(.+))?$`,
)

// majorOnlyRegex matches pins like 8, v8 and 16-alpine. The digit limit keeps
// build IDs and dates such as 2026041305 and 2024-05-01 from reading as majors.
var majorOnlyRegex = regexp.MustCompile(`^v?(\d{1,3})(?:-([a-zA-Z].*))?$`)

// longNumericSuffix catches tags like 13.1.0-24643103163
var longNumericSuffix = regexp.MustCompile(`-\d{6,}$`)

// gitHashSuffix catches tags like 2.18.3-f32cb56 (hex hash after dash)
var gitHashSuffix = regexp.MustCompile(`-[0-9a-f]{7,}$`)

// wordPrefixRegex catches tags like "develop-4.0.17" or "version-4.0.17"
var wordPrefixRegex = regexp.MustCompile(`^[a-zA-Z]+-\d`)

// devSuffixes are tags to deprioritize
var devSuffixes = []string{"-dev", "-develop", "-rc", "-beta", "-alpha", "-snapshot", "-nightly", "-distroless", "-busybox", "-ubuntu"}

// archPrefixes are tag prefixes that indicate architecture-specific builds
var archPrefixes = []string{"amd64-", "arm64v8-", "arm32v7-", "arm64-", "arm-"}

// archSuffixes are tag suffixes that indicate architecture-specific builds
var archSuffixes = []string{"-amd64", "-arm64", "-arm64v8", "-arm32v7", "-armhf", "-arm"}

// IsSemver returns true if a tag looks like a real version number
func IsSemver(tag string) bool {
	return hasVersionShape(tag) && semverRegex.MatchString(tag)
}

// hasVersionShape rejects tags that are builds of a version rather than a
// version: architecture-specific, word-prefixed, or build-ID and git-hash tagged.
func hasVersionShape(tag string) bool {
	lower := strings.ToLower(tag)

	// Skip arch-specific prefix tags
	for _, prefix := range archPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}

	// Skip arch-specific suffix tags
	for _, suffix := range archSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return false
		}
	}

	// Skip tags with word prefixes like "develop-4.0.17" or "version-4.0.17"
	// but allow "v1.2.3"
	if wordPrefixRegex.MatchString(tag) && !strings.HasPrefix(lower, "v") {
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

// isCandidateTag returns true if a tag can be recommended to a pin of its
// variant. Unlike isCleanVersion it keeps variants such as -ubuntu and
// -fpm-alpine; Pick only offers them to pins of that variant.
func isCandidateTag(tag string) bool {
	return !isUnsafeTag(tag) && hasVersionShape(tag) && versionRegex.MatchString(tag) && !isPreRelease(tag)
}

// IsCleanTag returns true if the tag has no dev/rc/nightly/distroless suffix
func IsCleanTag(tag string) bool {
	lower := strings.ToLower(tag)
	for _, suffix := range devSuffixes {
		if strings.Contains(lower, suffix) {
			return false
		}
	}
	return true
}

// isCleanVersion returns true if a tag is both a valid semver and has no
// dev/nightly/distroless suffix. Used to filter the available versions list.
func isCleanVersion(tag string) bool {
	return IsSemver(tag) && IsCleanTag(tag)
}

// SemverParts extracts major, minor, patch from a tag.
// For 4-part versions (e.g., 4.0.17.2952), only uses first 3 parts.
// Major-only tags such as 8-alpine have just a major.
// Returns -1 for missing parts.
func SemverParts(tag string) (int, int, int) {
	matches := versionRegex.FindStringSubmatch(tag)
	if matches == nil {
		if majorOnly := majorOnlyRegex.FindStringSubmatch(tag); majorOnly != nil {
			return atoi(majorOnly[1]), -1, -1
		}
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

// compareVersionNumbers compares only the version numbers of two tags,
// ignoring any variant.
// Returns positive if a > b, negative if a < b, 0 if equal.
func compareVersionNumbers(a, b string) int {
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

	return fourthPart(a) - fourthPart(b)
}

// fourthPart returns the 4th version part (4.0.17.2952), or 0 if there isn't one.
func fourthPart(tag string) int {
	matches := versionRegex.FindStringSubmatch(tag)
	if matches == nil || matches[4] == "" {
		return 0
	}
	return atoi(matches[4])
}

// linuxServerBuildNumber returns the build number of a tag like 1.5.6-ls309, or
// 0 if it has none. It's only for the lower limit, not for sorting: LinuxServer
// restarts the counter, so -ls2 can be newer than -ls329 and registry order is
// the better guide.
func linuxServerBuildNumber(tag string) int {
	matches := versionRegex.FindStringSubmatch(tag)
	if matches == nil || !linuxServerBuild.MatchString(matches[5]) {
		return 0
	}
	return atoi(matches[5][2:])
}

// CompareSemver compares two semver tags.
// Returns positive if a > b, negative if a < b, 0 if equal.
func CompareSemver(a, b string) int {
	if c := compareVersionNumbers(a, b); c != 0 {
		return c
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
// removes unsafe tags, and sorts descending. Picking needs the whole list; use
// DisplayVersions to limit what's shown.
// This is the single source of truth for tag processing.
func FilterAndSortTags(tags []string) []models.RegistryVersion {
	var versions []models.RegistryVersion

	for _, tag := range tags {
		if !isCandidateTag(tag) {
			continue
		}
		versions = append(versions, models.RegistryVersion{Tag: tag})
	}

	versions = removePartialTags(versions, true)

	sort.SliceStable(versions, func(i, j int) bool {
		return CompareSemver(versions[i].Tag, versions[j].Tag) > 0
	})

	return versions
}

// FilterAndSortDockerHubTags is like FilterAndSortTags but accepts
// the Docker Hub response format with timestamps.
func FilterAndSortDockerHubTags(tags []DockerHubTag) []models.RegistryVersion {
	var versions []models.RegistryVersion

	for _, tag := range tags {
		if !isCandidateTag(tag.Name) {
			continue
		}
		v := models.RegistryVersion{
			Tag:      tag.Name,
			Released: tag.LastUpdated,
			Digest:   tag.Digest,
		}
		v.ReleasedAt = parseTimestamp(tag.LastUpdated)
		versions = append(versions, v)
	}

	versions = removePartialTags(versions, true)

	sort.SliceStable(versions, func(i, j int) bool {
		return CompareSemver(versions[i].Tag, versions[j].Tag) > 0
	})

	return versions
}

// DisplayVersions returns the tags to show as available versions: the newest
// config.VersionFetchLimit clean ones. Picking uses a wider list so that it can
// serve variant pins; this list stays as it always was.
func DisplayVersions(versions []models.RegistryVersion) []string {
	var clean []models.RegistryVersion
	for _, v := range versions {
		if isCleanVersion(v.Tag) {
			clean = append(clean, v)
		}
	}

	var tags []string
	for _, v := range removePartialTags(clean, false) {
		if len(tags) == config.VersionFetchLimit {
			break
		}
		tags = append(tags, v.Tag)
	}
	return tags
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

// removePartialTags filters out version tags that are less specific than
// other tags with the same prefix. For example:
//   - "5.0" is removed when "5.0.2" exists (2-part when 3-part exists)
//   - "6.1.1" is removed when "6.1.1.10360-ls303" exists (3-part when 4-part exists)
//
// Partial tags are floating — the registry updates them to the latest in
// the series — so recommending them as pinned versions is misleading.
//
// With matchVariant, a 2-part tag is only removed by a 3-part tag of the same
// variant, so 9.2-alpine stays when the only 9.2.x tags are plain.
func removePartialTags(versions []models.RegistryVersion, matchVariant bool) []models.RegistryVersion {
	type line struct {
		major, minor int
		variant      string
	}
	lineOf := func(matches []string, tag string) line {
		l := line{major: atoi(matches[1]), minor: atoi(matches[2])}
		if matchVariant {
			l.variant = tagVariant(tag)
		}
		return l
	}

	// Collect lines that have a 3-part version
	has3Part := make(map[line]bool)
	// Collect major.minor.patch triples that have a 4-part version
	has4Part := make(map[[3]int]bool)

	for _, v := range versions {
		matches := versionRegex.FindStringSubmatch(v.Tag)
		if matches == nil {
			continue
		}
		if matches[3] != "" {
			has3Part[lineOf(matches, v.Tag)] = true
			if matches[4] != "" {
				has4Part[[3]int{atoi(matches[1]), atoi(matches[2]), atoi(matches[3])}] = true
			}
		}
	}

	var filtered []models.RegistryVersion
	for _, v := range versions {
		matches := versionRegex.FindStringSubmatch(v.Tag)
		if matches == nil {
			filtered = append(filtered, v)
			continue
		}

		// 2-part tag: skip if a 3-part tag exists on the same line
		if matches[3] == "" && has3Part[lineOf(matches, v.Tag)] {
			continue
		}

		// 3-part tag with no suffix: skip if a 4-part tag exists with same major.minor.patch
		if matches[3] != "" && matches[4] == "" && matches[5] == "" {
			if has4Part[[3]int{atoi(matches[1]), atoi(matches[2]), atoi(matches[3])}] {
				continue
			}
		}

		filtered = append(filtered, v)
	}
	return filtered
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