package registry

import (
	"docker-scanner/pkg/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"docker-scanner/internal/config"
)

type DockerHubRegistry struct{}

// DockerHubTag represents a single tag from Docker Hub API
type DockerHubTag struct {
	Name        string `json:"name"`
	LastUpdated string `json:"last_updated"`
}

type dockerHubResponse struct {
	Next    string         `json:"next"`
	Results []DockerHubTag `json:"results"`
}

func (r *DockerHubRegistry) Name() string {
	return "Docker Hub"
}

func (r *DockerHubRegistry) Supports(registry string) bool {
	return registry == "docker.io" || registry == ""
}

func (r *DockerHubRegistry) FetchVersions(image models.Image) ([]models.RegistryVersion, error) {
	tags, err := fetchHubTags(httpGet, hubRepo(image.Name), image.Tag)
	if err != nil {
		return nil, err
	}

	return FilterAndSortDockerHubTags(tags), nil
}

// hubRepo returns an image's Docker Hub repository; official images have no
// namespace in compose files but live under library/.
func hubRepo(name string) string {
	if !strings.Contains(name, "/") {
		return "library/" + name
	}
	return name
}

// hubPinFilterPages limits how many pages are read for a pin's own name filter.
const hubPinFilterPages = 3

// fetchHubTags returns the Docker Hub tags worth considering for a pin, newest
// pushes first.
//
// The first request only asks for tags with a dot, because a tag without one
// can't be recommended and images can have thousands of nightlies. If that isn't
// the whole list, a second query uses the pin's own filter: on n8nio/runners,
// builds of an older major fill the first page and hide 3.1.0 from a 3.0.1 pin.
func fetchHubTags(get func(url string) ([]byte, error), repo, currentTag string) ([]DockerHubTag, error) {
	tags, next, err := fetchHubPage(get, hubTagsURL(repo, "."))
	if err != nil {
		return nil, err
	}

	filter := pinNameFilter(currentTag)
	if next == "" || filter == "" {
		return tags, nil
	}

	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		seen[tag.Name] = true
	}

	next = hubTagsURL(repo, filter)
	for page := 0; page < hubPinFilterPages && next != ""; page++ {
		var results []DockerHubTag
		results, next, err = fetchHubPage(get, next)
		if err != nil {
			// The first request succeeded, so keep what it found.
			break
		}
		for _, tag := range results {
			if !seen[tag.Name] {
				seen[tag.Name] = true
				tags = append(tags, tag)
			}
		}
	}
	return tags, nil
}

func hubTagsURL(repo, nameFilter string) string {
	return fmt.Sprintf(
		"https://registry.hub.docker.com/v2/repositories/%s/tags/?page_size=100&ordering=last_updated&name=%s",
		repo, url.QueryEscape(nameFilter),
	)
}

func fetchHubPage(get func(url string) ([]byte, error), pageURL string) ([]DockerHubTag, string, error) {
	body, err := get(pageURL)
	if err != nil {
		return nil, "", err
	}

	var result dockerHubResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", fmt.Errorf("failed to parse response: %w", err)
	}
	return result.Results, result.Next, nil
}

// pinNameFilter returns the Docker Hub name filter that finds a pin's relatives:
// its major for a version, or its variant for a word tag like apache. Tags with
// neither, such as latest or a git hash, have nothing to filter on.
func pinNameFilter(tag string) string {
	if major, _, _ := SemverParts(tag); major >= 0 {
		return fmt.Sprintf("%d.", major)
	}
	if variant := tagVariant(tag); variant != "" {
		return "-" + variant
	}
	return ""
}

// registryStatusError is a non-200 registry response. Challenge holds the
// Www-Authenticate header, which says who a 401 expects a token from.
type registryStatusError struct {
	Status    int
	Challenge string
}

func (e *registryStatusError) Error() string {
	return fmt.Sprintf("registry returned status %d", e.Status)
}

// httpGet is a shared HTTP helper
func httpGet(url string) ([]byte, error) {
	client := &http.Client{
		Timeout: time.Duration(config.HTTPTimeoutSeconds) * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to query registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &registryStatusError{Status: resp.StatusCode, Challenge: resp.Header.Get("Www-Authenticate")}
	}

	return io.ReadAll(resp.Body)
}