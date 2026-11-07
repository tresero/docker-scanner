package registry

import (
	"docker-scanner/pkg/models"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	alpineVariant    = regexp.MustCompile(`^alpine(\d+(\.\d+)*)?$`)
	linuxServerBuild = regexp.MustCompile(`^ls\d+$`)
	gitHashTag       = regexp.MustCompile(`^[0-9a-f]{7,}$`)
)

// SafePickResult holds the result of picking a safe version
type SafePickResult struct {
	Version   string
	Age       string
	MajorJump bool
	Note      string // why nothing newer is recommended; empty when there's nothing to explain

	// NewerMajor is the newest safe tag on a higher major, set when the pin
	// keeps the recommendation on its own line and so hides that major.
	NewerMajor string

	// Unverified means the scan couldn't confirm the image's status.
	Unverified bool
}

// PickInput is everything Pick needs. Now is injected so tests can use a
// fixed clock.
type PickInput struct {
	Versions       []models.RegistryVersion
	CurrentTag     string
	RunningVersion string // from the Docker daemon; empty when not running
	SafeDays       int
	Now            time.Time
}

// preReleaseSuffixes are tags that should never be recommended
var preReleaseSuffixes = []string{"-rc", "-beta", "-alpha", "-dev", "-develop", "-snapshot", "-pre", "-nightly"}

// isPreRelease returns true if a tag contains a pre-release suffix
func isPreRelease(tag string) bool {
	lower := strings.ToLower(tag)
	for _, suffix := range preReleaseSuffixes {
		if strings.Contains(lower, suffix) {
			return true
		}
	}
	return false
}

// PickSafeVersion is Pick at the current time.
func PickSafeVersion(versions []models.RegistryVersion, currentTag string, safeDays int) SafePickResult {
	return Pick(PickInput{Versions: versions, CurrentTag: currentTag, SafeDays: safeDays, Now: time.Now()})
}

// Pick selects the newest stable version published at least SafeDays ago and
// detects major version jumps. It keeps the pin's variant. A major or
// major.minor pin stays on its own line, and any newer major is reported
// separately in NewerMajor. It never recommends a version below the higher of
// the pin and the running version.
//
// Position fallback runs only when no stable tag has a date. If dates are
// known and nothing is old enough, it recommends nothing and says why.
func Pick(in PickInput) SafePickResult {
	if len(in.Versions) == 0 {
		return SafePickResult{Note: "no usable version tags found", Unverified: true}
	}

	var stable []models.RegistryVersion
	for _, v := range in.Versions {
		if !isPreRelease(v.Tag) {
			stable = append(stable, v)
		}
	}
	if len(stable) == 0 {
		return SafePickResult{Note: "only pre-release tags found", Unverified: true}
	}

	matching := filterVariant(stable, in.CurrentTag)
	if len(matching) == 0 {
		if variant := tagVariant(in.CurrentTag); variant != "" {
			return SafePickResult{Note: fmt.Sprintf("no tags found with variant %q", variant), Unverified: true}
		}
		return SafePickResult{Note: "no plain version tags found", Unverified: true}
	}

	floor := versionFloor(in.CurrentTag, in.RunningVersion)

	major, minor, onLine := pinLine(in.CurrentTag)
	if !onLine {
		return selectFromFloor(matching, in, floor)
	}

	var line, higherMajor []models.RegistryVersion
	for _, v := range matching {
		tagMajor, _, _ := SemverParts(v.Tag)
		switch {
		case onPinLine(v.Tag, major, minor):
			line = append(line, v)
		case tagMajor > major && compareVersionNumbers(v.Tag, floor) > 0:
			higherMajor = append(higherMajor, v)
		}
	}

	newerMajor := selectSafe(higherMajor, in).Version
	if len(line) == 0 {
		return SafePickResult{
			Note:       fmt.Sprintf("no tags found on the %s line", lineName(major, minor)),
			NewerMajor: newerMajor,
			Unverified: true,
		}
	}

	result := selectFromFloor(line, in, floor)
	result.NewerMajor = newerMajor
	return result
}

// selectFromFloor applies the age rule to the candidates at or above floor, and
// explains the result when it isn't an upgrade.
func selectFromFloor(candidates []models.RegistryVersion, in PickInput, floor string) SafePickResult {
	if floor == "" {
		return selectSafe(candidates, in)
	}

	var eligible []models.RegistryVersion
	for _, v := range candidates {
		if notBelow(v.Tag, floor) {
			eligible = append(eligible, v)
		}
	}
	if len(eligible) == 0 {
		return SafePickResult{Note: fmt.Sprintf("no tags at or above %s found; can't confirm it's current", floor), Unverified: true}
	}

	result := selectSafe(eligible, in)
	newest := eligible[0]

	if compareVersionNumbers(newest.Tag, floor) == 0 {
		// Nothing newer exists, so the tag's age doesn't matter: rebuilds reset
		// it, and the only alternatives are older versions.
		if result.Version == "" {
			result.Note = fmt.Sprintf("already on the newest version (%s)", floor)
		}
		return result
	}

	// A newer tag exists but we're staying put, so say what we're waiting for.
	if result.Version != "" && compareVersionNumbers(result.Version, floor) == 0 && !newest.ReleasedAt.IsZero() {
		result.Note = waitingNote(newest, in)
	}
	return result
}

// versionFloor returns the higher of the pin and the running version, or "" if
// neither is a readable version.
func versionFloor(currentTag, runningVersion string) string {
	floor := ""
	for _, tag := range []string{currentTag, runningVersion} {
		if major, _, _ := SemverParts(tag); major < 0 {
			continue
		}
		if floor == "" || !notBelow(floor, tag) {
			floor = tag
		}
	}
	return floor
}

// notBelow reports whether a tag is at or above the lower limit, counting
// LinuxServer build numbers within the same version.
func notBelow(tag, floor string) bool {
	if floor == "" {
		return true
	}
	if c := compareVersionNumbers(tag, floor); c != 0 {
		return c > 0
	}
	return linuxServerBuildNumber(tag) >= linuxServerBuildNumber(floor)
}

func waitingNote(newest models.RegistryVersion, in PickInput) string {
	return fmt.Sprintf("newest is %s, pushed %s ago; waiting for -safe-days %d",
		newest.Tag, formatAge(in.Now, newest.ReleasedAt), in.SafeDays)
}

// selectSafe applies the age rule to stable tags sorted newest first.
func selectSafe(stable []models.RegistryVersion, in PickInput) SafePickResult {
	if len(stable) == 0 {
		return SafePickResult{}
	}

	cutoff := in.Now.AddDate(0, 0, -in.SafeDays)

	var newestDated *models.RegistryVersion
	for i, v := range stable {
		if v.ReleasedAt.IsZero() {
			continue
		}
		if newestDated == nil {
			newestDated = &stable[i]
		}
		if v.ReleasedAt.Before(cutoff) {
			return SafePickResult{
				Version:   v.Tag,
				Age:       formatAge(in.Now, v.ReleasedAt),
				MajorJump: isMajorJump(in.CurrentTag, v.Tag),
			}
		}
	}

	if newestDated == nil {
		return positionFallback(stable, in.CurrentTag, in.SafeDays, in.Now)
	}

	return SafePickResult{Note: waitingNote(*newestDated, in)}
}

// filterVariant keeps the tags that fit the pin's variant, sorted newest first.
// Among equal versions the closest variant comes first, then the pin's spelling:
// some images publish both 2.8.11 and v2.8.11, in no reliable order.
func filterVariant(stable []models.RegistryVersion, currentTag string) []models.RegistryVersion {
	var matching []models.RegistryVersion
	for _, v := range stable {
		if variantFits(currentTag, v.Tag) {
			matching = append(matching, v)
		}
	}

	pinVariant, pinV := tagVariant(currentTag), hasVPrefix(currentTag)
	sort.SliceStable(matching, func(i, j int) bool {
		a, b := matching[i].Tag, matching[j].Tag
		if c := compareVersionNumbers(a, b); c != 0 {
			return c > 0
		}
		if ca, cb := variantCloseness(pinVariant, a), variantCloseness(pinVariant, b); ca != cb {
			return ca > cb
		}
		return hasVPrefix(a) == pinV && hasVPrefix(b) != pinV
	})
	return matching
}

func hasVPrefix(tag string) bool {
	return strings.HasPrefix(tag, "v")
}

// variantFits reports whether a tag has the pin's variant. The one exception
// to an exact match is an Alpine release at a higher version (alpine3.23 to
// alpine3.24), which isn't generalised: pg16 to pg17 would break a database.
func variantFits(currentTag, tag string) bool {
	pinVariant, variant := tagVariant(currentTag), tagVariant(tag)
	if pinVariant == variant {
		return true
	}
	if !alpineVariant.MatchString(pinVariant) || !alpineVariant.MatchString(variant) {
		return false
	}
	major, _, _ := SemverParts(currentTag)
	return major >= 0 && compareVersionNumbers(tag, currentTag) > 0
}

// variantCloseness ranks a tag's variant against the pin's: an exact match, then
// an Alpine release when the pin also names one, then anything else.
func variantCloseness(pinVariant, tag string) int {
	variant := tagVariant(tag)
	switch {
	case variant == pinVariant:
		return 2
	case variant != "alpine" && pinVariant != "alpine":
		return 1
	}
	return 0
}

// tagVariant returns the variant a tag belongs to: alpine for 8.1-alpine,
// apache for a word-only tag like apache, and "" for plain versions. A floating
// channel, a date, a git hash and a LinuxServer build number (-ls309) aren't
// variants.
func tagVariant(tag string) string {
	lower := strings.ToLower(tag)
	if matches := versionRegex.FindStringSubmatch(lower); matches != nil {
		return versionSuffixVariant(matches[5])
	}
	if matches := majorOnlyRegex.FindStringSubmatch(lower); matches != nil {
		return versionSuffixVariant(matches[2])
	}
	if lower == "" || isUnsafeTag(lower) || gitHashTag.MatchString(lower) || lower[0] < 'a' || lower[0] > 'z' {
		return ""
	}
	return lower
}

func versionSuffixVariant(suffix string) string {
	if linuxServerBuild.MatchString(suffix) {
		return ""
	}
	return suffix
}

// pinLine returns the line a major (8-alpine) or major.minor (11.4) pin stays
// on, with minor -1 for a major pin. ok is false for any other kind of tag.
func pinLine(tag string) (major, minor int, ok bool) {
	major, minor, patch := SemverParts(tag)
	return major, minor, major >= 0 && patch < 0
}

func onPinLine(tag string, major, minor int) bool {
	tagMajor, tagMinor, _ := SemverParts(tag)
	return tagMajor == major && (minor < 0 || tagMinor == minor)
}

func lineName(major, minor int) string {
	if minor < 0 {
		return fmt.Sprintf("%d.x", major)
	}
	return fmt.Sprintf("%d.%d", major, minor)
}

// MatchOverride finds the registry tag for an upstream override version,
// holding it to the same pin rules as Pick. Returns "" if no tag qualifies.
func MatchOverride(versions []models.RegistryVersion, upstreamVersion, currentTag, runningVersion string) (tag string, majorJump bool) {
	floor := versionFloor(currentTag, runningVersion)

	var fitting []models.RegistryVersion
	for _, v := range filterVariant(versions, currentTag) {
		if fitsPin(currentTag, v.Tag) && notBelow(v.Tag, floor) {
			fitting = append(fitting, v)
		}
	}

	tag = MatchDockerTag(fitting, upstreamVersion)
	if tag == "" {
		return "", false
	}
	return tag, isMajorJump(currentTag, tag)
}

// fitsPin reports whether a tag can be recommended for the pin.
func fitsPin(currentTag, tag string) bool {
	if !variantFits(currentTag, tag) {
		return false
	}
	major, minor, onLine := pinLine(currentTag)
	return !onLine || onPinLine(tag, major, minor)
}

// positionFallback skips the newest N stable versions when no dates are available
// For safeDays=3, skip 1 version (the very latest)
// For safeDays=7, skip 2 versions, etc.
func positionFallback(stable []models.RegistryVersion, currentTag string, safeDays int, now time.Time) SafePickResult {
	skip := int(math.Max(1, float64(safeDays/3)))

	if skip >= len(stable) {
		skip = len(stable) - 1
	}

	picked := stable[skip]
	age := "age unknown"
	if !picked.ReleasedAt.IsZero() {
		age = formatAge(now, picked.ReleasedAt)
	}

	return SafePickResult{
		Version:   picked.Tag,
		Age:       age,
		MajorJump: isMajorJump(currentTag, picked.Tag),
	}
}

// isMajorJump checks if the recommended version has a different major
// version than the current tag
func isMajorJump(currentTag, recommendedTag string) bool {
	if currentTag == "latest" || currentTag == "" {
		return false
	}

	curMaj, _, _ := SemverParts(currentTag)
	recMaj, _, _ := SemverParts(recommendedTag)

	if curMaj == -1 || recMaj == -1 {
		return false
	}

	return curMaj != recMaj
}

// formatAge returns a human-readable age string
func formatAge(now, released time.Time) string {
	diff := now.Sub(released)
	days := int(diff.Hours() / 24)

	switch {
	case days == 0:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	case days == 1:
		return "1 day"
	case days < 7:
		return fmt.Sprintf("%d days", days)
	case days < 30:
		weeks := days / 7
		if weeks == 1 {
			return "1 week"
		}
		return fmt.Sprintf("%d weeks", weeks)
	case days < 365:
		months := days / 30
		if months == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", months)
	default:
		years := days / 365
		if years == 1 {
			return "1 year"
		}
		return fmt.Sprintf("%d years", years)
	}
}