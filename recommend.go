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

	r.logf("    Fetching versions for %s\n", imageName)

	versions, err := registry.Lookup(r.registries, info.Image)
	if err != nil {
		r.logf("    Warning: %v\n", err)
		info.RecommendationNote = fmt.Sprintf("couldn't check for updates: %v", err)
		info.Unverified = true
		return
	}

	running, runningKnown := registry.ResolveRunning(versions, info.Image.Tag, info.RunningVersion, info.RunningDigests, info.RunningLabelVersion)
	if runningKnown && running != info.RunningVersion {
		info.ResolvedRunningVersion = running
	}

	// Override path: trust the upstream source for this image, but still apply
	// the safeDays cutoff. An image whose real version is unknown skips it, since
	// nothing could show that its release isn't a downgrade.
	if override := r.overrides.Find(imageName); override != nil && runningKnown {
		if r.applyOverride(info, override, versions, running) {
			return
		}
	}

	// Registry path.
	info.AvailableVersions = registry.DisplayVersions(versions)

	input := registry.PickInput{
		Versions:       versions,
		CurrentTag:     info.Image.Tag,
		RunningVersion: running,
		RunningUnknown: !runningKnown,
		SafeDays:       r.safeDays,
		Now:            r.now(),
	}
	if !runningKnown {
		input.RunningVersion = ""
	}

	pick := registry.Pick(input)
	info.RecommendedVersion = pick.Version
	info.RecommendedAge = pick.Age
	info.MajorVersionJump = pick.MajorJump
	info.RecommendationNote = pick.Note
	info.NewerMajorVersion = pick.NewerMajor
	info.Unverified = pick.Unverified
}

// applyOverride recommends an upstream release if a registry tag matches it. It
// returns false if the image should fall through to the registry path.
func (r *recommender) applyOverride(info *models.ImageInfo, override *overrides.Override, versions []models.RegistryVersion, running string) bool {
	imageName := info.Image.Registry + "/" + info.Image.Name
	r.logf("    Using override (%s → %s) for %s\n", override.Source, override.Repo, imageName)

	rec, err := overrides.Resolve(override, r.getter, r.safeDays)
	if err != nil {
		r.logf("    Override failed for %s, falling back to registry: %v\n", imageName, err)
		return false
	}
	if rec == nil {
		r.logf("    Override returned no release older than %d days for %s, falling back to registry\n", r.safeDays, imageName)
		return false
	}

	// Find the matching Docker tag so we recommend something the user can
	// actually pull. The upstream version may not be packaged yet.
	dockerTag, majorJump := registry.MatchOverride(versions, rec.Version, info.Image.Tag, running)
	if dockerTag == "" {
		r.logf("    Override %s has no matching Docker tag, falling back to registry\n", rec.Version)
		return false
	}

	info.RecommendedVersion = dockerTag
	info.RecommendedAge = rec.Age
	info.MajorVersionJump = majorJump
	r.logf("    Override %s matched Docker tag %s\n", rec.Version, dockerTag)
	return true
}
