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
	// The parser already said why: looking up a partial or local name could only mislead.
	if info.SkipLookup {
		return
	}

	imageName := info.Image.Registry + "/" + info.Image.Name

	r.logf("    Fetching versions for %s\n", imageName)

	versions, err := registry.Lookup(r.registries, info.Image)
	if err != nil {
		r.logf("    Warning: %v\n", err)
		info.RecommendationNote = fmt.Sprintf("couldn't check for updates: %v", err)
		info.Unverified = true
		return
	}

	r.checkNewerBuild(info)

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
		StayOnMajor:    registry.IsDatabase(info.Image.Name),
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

// markDataUpgrade warns when upgrading this database takes more than a new image.
// It needs the recommendation, so it runs after recommend.
func markDataUpgrade(info *models.ImageInfo) {
	running := info.RunningVersion
	if info.ResolvedRunningVersion != "" {
		running = info.ResolvedRunningVersion
	}
	info.DataUpgradeWarning = registry.DataUpgradeWarning(registry.DataUpgradeInput{
		Name:        info.Image.Name,
		Tag:         info.Image.Tag,
		Running:     running,
		Recommended: info.RecommendedVersion,
		NewerMajor:  info.NewerMajorVersion,
		Floating:    info.UsesLatest,
	})
}

// markTagMismatch records whether the next up -d would change the running version.
// It needs the real running version, so it runs after recommend.
func markTagMismatch(info *models.ImageInfo) {
	info.TagMismatch, info.TagMismatchDowngrade = registry.CheckTagMismatch(info.Image.Tag, info.RunningVersion, info.ResolvedRunningVersion)
}

// checkNewerBuild reports whether the compose tag has moved past the running image.
// A container on a different tag is left out: its digest will differ whether or
// not the tag was rebuilt. A floating tag's running version is whatever its label
// says, so that one still counts.
func (r *recommender) checkNewerBuild(info *models.ImageInfo) {
	if info.Dockerfile != "" {
		r.checkNewerBase(info)
		return
	}

	if len(info.RunningDigests) == 0 || (info.RunningVersion != info.Image.Tag && info.Image.Tag != "latest") {
		return
	}

	build, err := registry.CurrentBuild(r.registries, info.Image)
	if err != nil {
		r.logf("    Couldn't check for a newer build of %s: %v\n", info.Image.Name, err)
		return
	}

	// An unreadable date is a zero time, and the digests still decide.
	runningBuilt, _ := time.Parse(time.RFC3339, info.RunningBuildDate)
	if registry.NewerBuild(build, info.RunningDigests, runningBuilt) {
		info.NewerBuildAvailable = true
		if !build.Pushed.IsZero() {
			info.NewerBuildDate = build.Pushed.UTC().Format(time.RFC3339)
		}
	}
}

// checkNewerBase reports whether a built image's base has been pushed since it was
// built. There's no digest to compare, because a build doesn't keep its base's.
func (r *recommender) checkNewerBase(info *models.ImageInfo) {
	built, err := time.Parse(time.RFC3339, info.RunningBuildDate)
	if err != nil {
		return
	}

	build, err := registry.CurrentBuild(r.registries, info.Image)
	if err != nil {
		r.logf("    Couldn't check for a newer base of %s: %v\n", info.Image.Name, err)
		return
	}

	if build.Pushed.After(built) {
		info.NewerBuildAvailable = true
		info.NewerBuildDate = build.Pushed.UTC().Format(time.RFC3339)
	}
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
	dockerTag, tagAge, majorJump := registry.MatchOverride(versions, rec.Version, info.Image.Tag, running, r.safeDays, r.now())
	if dockerTag == "" {
		r.logf("    Override %s has no matching Docker tag older than %d days, falling back to registry\n", rec.Version, r.safeDays)
		return false
	}

	info.RecommendedVersion = dockerTag
	info.RecommendedAge = tagAge
	if tagAge == "" {
		info.RecommendedAge = rec.Age
	}
	info.MajorVersionJump = majorJump
	r.logf("    Override %s matched Docker tag %s\n", rec.Version, dockerTag)
	return true
}
