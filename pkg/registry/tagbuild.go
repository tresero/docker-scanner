package registry

import (
	"docker-scanner/pkg/models"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// TagBuild describes the image a tag points to right now. Pushed is zero if the
// registry doesn't say.
type TagBuild struct {
	Digest string
	Pushed time.Time
}

// TagBuilder is implemented by registries that can say what a tag points to.
type TagBuilder interface {
	CurrentBuild(image models.Image) (TagBuild, error)
}

// CurrentBuild asks the image's registry what its tag points to. A registry that
// can't say returns an empty TagBuild.
func CurrentBuild(registries []Registry, image models.Image) (TagBuild, error) {
	for _, r := range registries {
		if r.Supports(image.Registry) {
			if builder, ok := r.(TagBuilder); ok {
				return builder.CurrentBuild(image)
			}
			return TagBuild{}, nil
		}
	}
	return TagBuild{}, nil
}

// NewerBuild reports whether a tag has moved past the running image. Registries
// and Docker both record the digest of the whole multi-arch image, so the digests
// compare directly. With nothing to compare it says no: this should only be
// claimed when it's known. A push older than the running image's build means the
// tag now points to an older copy, which isn't an upgrade.
func NewerBuild(build TagBuild, runningDigests []string, runningBuilt time.Time) bool {
	if build.Digest == "" || len(runningDigests) == 0 {
		return false
	}
	for _, digest := range runningDigests {
		if digest == build.Digest {
			return false
		}
	}
	if !build.Pushed.IsZero() && !runningBuilt.IsZero() && build.Pushed.Before(runningBuilt) {
		return false
	}
	return true
}

func (r *DockerHubRegistry) CurrentBuild(image models.Image) (TagBuild, error) {
	return hubTagBuild(httpGet, hubRepo(image.Name), image.Tag)
}

func (r *GHCRRegistry) CurrentBuild(image models.Image) (TagBuild, error) {
	if image.Registry == "lscr.io" {
		return hubTagBuild(httpGet, image.Name, image.Tag)
	}
	token, err := getGHCRToken(image.Name)
	if err != nil {
		return TagBuild{}, fmt.Errorf("failed to get GHCR token: %w", err)
	}
	pushed, digest := fetchManifestDetails(image.Name, image.Tag, token)
	return TagBuild{Digest: digest, Pushed: pushed}, nil
}

func (r *GenericRegistry) CurrentBuild(image models.Image) (TagBuild, error) {
	return genericTagBuild(httpGetManifest, image)
}

func hubTagBuild(get func(url string) ([]byte, error), repo, tag string) (TagBuild, error) {
	body, err := get(fmt.Sprintf("https://registry.hub.docker.com/v2/repositories/%s/tags/%s", repo, url.PathEscape(tag)))
	if err != nil {
		return TagBuild{}, err
	}
	var result DockerHubTag
	if err := json.Unmarshal(body, &result); err != nil {
		return TagBuild{}, fmt.Errorf("failed to parse response: %w", err)
	}
	return TagBuild{Digest: result.Digest, Pushed: parseTimestamp(result.LastUpdated)}, nil
}

// genericTagBuild reads the digest from a manifest request. As with listing tags,
// a Docker Hub front answers through Docker Hub.
func genericTagBuild(get func(url string) ([]byte, http.Header, error), image models.Image) (TagBuild, error) {
	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", image.Registry, image.Name, url.PathEscape(image.Tag))
	_, headers, err := get(manifestURL)
	if err != nil {
		if !servedByDockerHub(err) {
			return TagBuild{}, err
		}
		hubGet := func(url string) ([]byte, error) {
			body, _, err := get(url)
			return body, err
		}
		return hubTagBuild(hubGet, hubRepo(image.Name), image.Tag)
	}
	return TagBuild{Digest: headers.Get("Docker-Content-Digest")}, nil
}
