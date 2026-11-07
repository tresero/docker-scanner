package registry

import (
    "docker-scanner/pkg/models"
    "strings"
)

// Registry is the interface for fetching version info from container registries.
// Implement this to add support for new registries.
type Registry interface {
    Name() string
    Supports(registry string) bool
    FetchVersions(image models.Image) ([]models.RegistryVersion, error)
}

// Lookup finds the right registry handler for an image
// and fetches available versions
func Lookup(registries []Registry, image models.Image) ([]models.RegistryVersion, error) {
    for _, r := range registries {
	if r.Supports(image.Registry) {
	    return r.FetchVersions(image)
	}
    }

    // Fall back to generic
    generic := &GenericRegistry{}
    return generic.FetchVersions(image)
}

// DefaultRegistries returns the standard set of registry handlers
func DefaultRegistries() []Registry {
    return []Registry{
	&DockerHubRegistry{},
	&GHCRRegistry{},
	&GenericRegistry{},
    }
}

// upstreamCandidates returns a list of candidate Docker tag strings derived
// from an upstream version (e.g., a GitHub release tag). Most specific first.
func upstreamCandidates(upstreamVersion string) []string {
    clean := strings.TrimPrefix(upstreamVersion, "v")
    // Strip build metadata after "+" (e.g., "13.0.1+security-01" → "13.0.1")
    base := clean
    if idx := strings.IndexByte(base, '+'); idx >= 0 {
	base = base[:idx]
    }

    candidates := []string{upstreamVersion, clean, "v" + clean}
    if base != clean {
	candidates = append(candidates, base)
    }
    return candidates
}

// MatchDockerTag finds a Docker registry tag that corresponds to an upstream
// version string (e.g., from a GitHub release). It tries exact match, without
// "v" prefix, stripped build metadata, and substring containment.
// Returns "" if no match is found.
func MatchDockerTag(versions []models.RegistryVersion, upstreamVersion string) string {
    candidates := upstreamCandidates(upstreamVersion)

    // Pass 1: exact match
    for _, v := range versions {
	for _, c := range candidates {
	    if v.Tag == c {
		return v.Tag
	    }
	}
    }

    // Pass 2: Docker tag contains the version (e.g., "4.0.17.2952-ls309" contains "4.0.17.2952")
    for _, v := range versions {
	for _, c := range candidates {
	    if strings.Contains(v.Tag, c) {
		return v.Tag
	    }
	}
    }

    return ""
}

