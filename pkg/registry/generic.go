package registry

import (
	"docker-scanner/pkg/models"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// GenericRegistry handles any OCI-compatible registry
// as a fallback when no specific handler matches
type GenericRegistry struct{}

func (r *GenericRegistry) Name() string {
	return "Generic OCI Registry"
}

func (r *GenericRegistry) Supports(registry string) bool {
	return true
}

func (r *GenericRegistry) FetchVersions(image models.Image) ([]models.RegistryVersion, error) {
	return fetchGenericVersions(httpGetFull, image)
}

func fetchGenericVersions(get tagPageGetter, image models.Image) ([]models.RegistryVersion, error) {
	url := fmt.Sprintf("https://%s/v2/%s/tags/list", image.Registry, image.Name)

	tags, err := fetchAllTags(get, url)
	if err != nil {
		if !servedByDockerHub(err) {
			return nil, fmt.Errorf("%s: %w", image.Registry, err)
		}
		hubGet := func(url string) ([]byte, error) {
			body, _, err := get(url)
			return body, err
		}
		hubTags, hubErr := fetchHubTags(hubGet, hubRepo(image.Name), image.Tag)
		if hubErr != nil {
			return nil, fmt.Errorf("%s: %w", image.Registry, hubErr)
		}
		return FilterAndSortDockerHubTags(hubTags), nil
	}

	return FilterAndSortTags(tags), nil
}

// servedByDockerHub reports whether a registry is a front for Docker Hub: it
// answers 401 and asks for a Docker Hub token, as docker.n8n.io does. Docker Hub's
// API then gives the same repository's tags, with dates that tag lists don't have.
func servedByDockerHub(err error) bool {
	var status *registryStatusError
	return errors.As(err, &status) && status.Status == http.StatusUnauthorized &&
		strings.Contains(status.Challenge, `service="registry.docker.io"`)
}
