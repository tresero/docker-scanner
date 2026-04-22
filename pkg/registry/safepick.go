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
}

// preReleaseSuffixes are tags that should never be recommended
var preReleaseSuffixes = []string{"-rc", "-beta", "-alpha", "-dev", "-snapshot", "-pre"}

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

// PickSafeVersion selects the best version that has been published
// for at least safeDays. It also detects major version jumps.
//
// Logic:
//  1. Skip any pre-release versions (rc, beta, alpha, dev)
//  2. Skip any version released less than safeDays ago
//  3. The first version that passes both checks is the recommendation
//  4. If the current tag has a parseable version, check for major version jumps
func PickSafeVersion(versions []models.RegistryVersion, currentTag string, safeDays int) SafePickResult {
	if len(versions) == 0 {
		return SafePickResult{}
	}

	now := time.Now()
	cutoff := now.AddDate(0, 0, -safeDays)

	// First pass: find newest stable version that's old enough (with dates)
	for _, v := range versions {
		if isPreRelease(v.Tag) {
			continue
		}

		if v.ReleasedAt.IsZero() {
			continue
		}

		if v.ReleasedAt.Before(cutoff) {
			return SafePickResult{
				Version:   v.Tag,
				Age:       formatAge(now, v.ReleasedAt),
				MajorJump: isMajorJump(currentTag, v.Tag),
			}
		}
	}

	// Fallback: no dates available or everything is too new
	return positionFallback(versions, currentTag, safeDays)
}

// positionFallback skips the newest N stable versions when no dates are available
// For safeDays=3, skip 1 version (the very latest)
// For safeDays=7, skip 2 versions, etc.
func positionFallback(versions []models.RegistryVersion, currentTag string, safeDays int) SafePickResult {
	skip := int(math.Max(1, float64(safeDays/3)))

	// Filter to stable versions only
	var stable []models.RegistryVersion
	for _, v := range versions {
		if !isPreRelease(v.Tag) {
			stable = append(stable, v)
		}
	}

	if len(stable) == 0 {
		return SafePickResult{}
	}

	if skip >= len(stable) {
		skip = len(stable) - 1
	}

	picked := stable[skip]
	age := "age unknown"
	if !picked.ReleasedAt.IsZero() {
		age = formatAge(time.Now(), picked.ReleasedAt)
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