package main

import (
	"docker-scanner/pkg/models"
	"docker-scanner/pkg/overrides"
	"docker-scanner/pkg/registry"
	"fmt"
	"os"
	"time"
)

// recommender fills in the recommendation fields for one image at a time.
// Its dependencies are injected so tests need no network or real registries.
type recommender struct {
	registries []registry.Registry
	getter     overrides.Getter
	overrides  *overrides.Config
	safeDays   int
	verbose    bool
	now        func() time.Time
}

func (r *recommender) logf(format string, args ...any) {
	if r.verbose {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

func (r *recommender) recommend(info *models.ImageInfo) {
	imageName := info.Image.Registry + "/" + info.Image.Name

	// Override path: trust the upstream source for this image,
	// but still apply the safeDays cutoff.
	if override := r.overrides.Find(imageName); override != nil {
		r.logf("    Using override (%s → %s) for %s\n", override.Source, override.Repo, imageName)
		rec, err := overrides.Resolve(override, r.getter, r.safeDays)
		if err != nil {
			r.logf("    Override failed for %s, falling back to registry: %v\n", imageName, err)
			// Fall through to registry path below.
		} else if rec == nil {
			r.logf("    Override returned no release older than %d days for %s, falling back to registry\n", r.safeDays, imageName)
			// Fall through to registry path below.
		} else {
			// Override gave us an upstream version — find the
			// matching Docker tag so we recommend something
			// the user can actually pull.
			versions, verr := registry.Lookup(r.registries, info.Image)
			dockerTag := ""
			if verr == nil {
				dockerTag = registry.MatchDockerTag(versions, rec.Version)
			}
			if dockerTag != "" {
				info.RecommendedVersion = dockerTag
				info.RecommendedAge = rec.Age
				r.logf("    Override %s matched Docker tag %s\n", rec.Version, dockerTag)
				return
			}
			// No matching Docker tag found — the upstream version
			// may not be packaged yet. Fall through to the registry
			// path which picks the best available Docker tag.
			r.logf("    Override %s has no matching Docker tag, falling back to registry\n", rec.Version)
		}
	}

	// Registry path.
	r.logf("    Fetching versions for %s\n", imageName)

	versions, err := registry.Lookup(r.registries, info.Image)
	if err != nil {
		r.logf("    Warning: %v\n", err)
		return
	}

	var tags []string
	for _, v := range versions {
		tags = append(tags, v.Tag)
	}
	info.AvailableVersions = tags

	pick := registry.Pick(registry.PickInput{
		Versions:   versions,
		CurrentTag: info.Image.Tag,
		SafeDays:   r.safeDays,
		Now:        r.now(),
	})
	info.RecommendedVersion = pick.Version
	info.RecommendedAge = pick.Age
	info.MajorVersionJump = pick.MajorJump
	info.RecommendationNote = pick.Note

	if info.RunningVersion != "" && pick.Version != "" {
		info.IsDowngrade = registry.CompareSemver(pick.Version, info.RunningVersion) < 0
	}
}
