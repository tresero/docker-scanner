package registry

import (
	"docker-scanner/pkg/models"
	"fmt"
	"math"
	"strings"
	"time"
)

// SafePickResult holds the result of picking a safe version
type SafePickResult struct {
	Version   string
	Age       string
	MajorJump bool
	Note      string // why nothing newer is recommended; empty when there's nothing to explain
}

// PickInput is everything Pick needs. Now is injected so tests can use a
// fixed clock.
type PickInput struct {
	Versions   []models.RegistryVersion
	CurrentTag string
	SafeDays   int
	Now        time.Time
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
// detects major version jumps.
//
// Position fallback runs only when no stable tag has a date. If dates are
// known and nothing is old enough, it recommends nothing and says why.
func Pick(in PickInput) SafePickResult {
	if len(in.Versions) == 0 {
		return SafePickResult{Note: "no usable version tags found"}
	}

	var stable []models.RegistryVersion
	for _, v := range in.Versions {
		if !isPreRelease(v.Tag) {
			stable = append(stable, v)
		}
	}
	if len(stable) == 0 {
		return SafePickResult{Note: "only pre-release tags found"}
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

	return SafePickResult{Note: fmt.Sprintf("newest is %s, pushed %s ago; waiting for -safe-days %d",
		newestDated.Tag, formatAge(in.Now, newestDated.ReleasedAt), in.SafeDays)}
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