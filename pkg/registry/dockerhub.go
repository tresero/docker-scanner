package registry

import (
	"docker-scanner/pkg/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Results []DockerHubTag `json:"results"`
}

func (r *DockerHubRegistry) Name() string {
	return "Docker Hub"
}

func (r *DockerHubRegistry) Supports(registry string) bool {
	return registry == "docker.io" || registry == ""
}

func (r *DockerHubRegistry) FetchVersions(image models.Image) ([]models.RegistryVersion, error) {
	name := image.Name
	if !strings.Contains(name, "/") {
		name = "library/" + name
	}

	url := fmt.Sprintf(
		"https://registry.hub.docker.com/v2/repositories/%s/tags/?page_size=100&ordering=last_updated",
		name,
	)

	body, err := httpGet(url)
	if err != nil {
		return nil, err
	}

	var result dockerHubResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return FilterAndSortDockerHubTags(result.Results), nil
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
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}