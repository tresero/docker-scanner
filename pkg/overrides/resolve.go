package overrides

import (
	"fmt"
	"time"
)

// Recommendation is the result of resolving an override — what the
// scanner should display to the user. Shape mirrors the registry-path
// equivalent (registry.SafePickResult) so the integration site can
// treat both paths uniformly.
type Recommendation struct {
	Version    string
	Age        string
	Prerelease bool
}

// Resolve looks up an override and returns a Recommendation, applying the
// same safeDays rule as the registry path: only releases older than
// safeDays are eligible. Returns nil with no error if no release qualifies
// (everything is too recent) so the caller can decide whether to leave
// the recommendation blank or fall back.
func Resolve(o *Override, get Getter, safeDays int) (*Recommendation, error) {
	switch o.Source {
	case SourceGitHubRelease:
		body, err := get(releasesURL(o.Repo))
		if err != nil {
			return nil, fmt.Errorf("fetch GitHub releases for %s: %w", o.Image, err)
		}
		releases, err := parseReleasesList(body)
		if err != nil {
			return nil, err
		}
		picked := pickSafeRelease(releases, time.Now(), safeDays)
		if picked == nil {
			return nil, nil
		}
		return &Recommendation{
			Version:    picked.TagName,
			Age:        formatAge(time.Now(), picked.PublishedAt),
			Prerelease: picked.Prerelease,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported source %q for image %s", o.Source, o.Image)
	}
}

// formatAge produces a human-readable age string. A zero released time
// returns "age unknown" — that's the convention the rest of the report
// uses for missing dates.
//
// Note: this duplicates registry.formatAge intentionally. Sharing would
// require either an awkward cross-package import or a new internal
// package; not worth the shuffle for 20 lines. If a third caller appears,
// extract it then.
func formatAge(now, released time.Time) string {
	if released.IsZero() {
		return "age unknown"
	}

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