package registry

import (
	"docker-scanner/internal/config"
	"docker-scanner/pkg/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type GHCRRegistry struct{}

type ghcrTagList struct {
	Tags []string `json:"tags"`
}

type manifestConfig struct {
	Created string `json:"created"`
}

type manifestResponse struct {
	Config   manifestConfig    `json:"config"`
	History  []manifestHistory `json:"history"`
}

type manifestHistory struct {
	V1Compat string `json:"v1Compatibility"`
}

type v1CompatInfo struct {
	Created string `json:"created"`
}

func (r *GHCRRegistry) Name() string {
	return "GitHub Container Registry"
}

func (r *GHCRRegistry) Supports(registry string) bool {
	return registry == "ghcr.io" || registry == "lscr.io"
}

// tagsListURL builds the GHCR tags-list URL with an explicit page size.
// GHCR silently truncates the default response to ~103 tags and does NOT
// return a Link header for pagination, so any image with more tags loses
// recent versions. Requesting n=1000 (the OCI-spec maximum that GHCR
// honors in practice) returns the full tag set in one call.
func tagsListURL(imageName string) string {
	return fmt.Sprintf("https://ghcr.io/v2/%s/tags/list?n=1000", imageName)
}

func (r *GHCRRegistry) FetchVersions(image models.Image) ([]models.RegistryVersion, error) {
	// lscr.io is a redirect to ghcr.io, so always query ghcr.io
	token, err := getGHCRToken(image.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get GHCR token: %w", err)
	}

	body, err := httpGetWithAuth(tagsListURL(image.Name), "Bearer "+token)
	if err != nil {
		return nil, err
	}

	var tagList ghcrTagList
	if err := json.Unmarshal(body, &tagList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	versions := FilterAndSortTags(tagList.Tags)

	// Fetch manifest dates for the filtered tags (at most 10 calls)
	for i := range versions {
		versions[i].ReleasedAt = fetchManifestDate(image.Name, versions[i].Tag, token)
	}

	// Fallback: if LinuxServer image returned poor results, try Docker Hub
	if isLinuxServerImage(image.Name) && len(versions) < 3 {
		dockerHubName := "linuxserver/" + linuxServerAppName(image.Name)
		hubURL := fmt.Sprintf(
			"https://registry.hub.docker.com/v2/repositories/%s/tags/?page_size=100&ordering=last_updated",
			dockerHubName,
		)
		hubBody, err := httpGet(hubURL)
		if err == nil {
			var result dockerHubResponse
			if err := json.Unmarshal(hubBody, &result); err == nil {
				hubVersions := FilterAndSortDockerHubTags(result.Results)
				if len(hubVersions) > len(versions) {
					versions = hubVersions
				}
			}
		}
	}

	return versions, nil
}

// isLinuxServerImage checks if an image is from LinuxServer
func isLinuxServerImage(name string) bool {
	return strings.HasPrefix(name, "linuxserver/")
}

// linuxServerAppName extracts the app name from a LinuxServer image
// e.g., "linuxserver/sonarr" -> "sonarr"
func linuxServerAppName(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return name
}

func getGHCRToken(imageName string) (string, error) {
	url := fmt.Sprintf("https://ghcr.io/token?scope=repository:%s:pull", imageName)

	body, err := httpGet(url)
	if err != nil {
		return "", err
	}

	var tokenResp struct {
		Token string `json:"token"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}

	if tokenResp.Token == "" {
		return "", fmt.Errorf("empty token received")
	}

	return strings.TrimSpace(tokenResp.Token), nil
}

// httpGetWithAuth performs a GET with an Authorization header
func httpGetWithAuth(url, auth string) ([]byte, error) {
	client := &http.Client{
		Timeout: time.Duration(config.HTTPTimeoutSeconds) * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// fetchManifestDate gets the creation date of a specific tag
// by querying the manifest endpoint
func fetchManifestDate(imageName, tag, token string) time.Time {
	url := fmt.Sprintf("https://ghcr.io/v2/%s/manifests/%s", imageName, tag)

	client := &http.Client{
		Timeout: time.Duration(config.HTTPTimeoutSeconds) * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return time.Time{}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	// Try v1 manifest first — it contains creation dates
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v1+json")

	resp, err := client.Do(req)
	if err != nil {
		return time.Time{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return time.Time{}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return time.Time{}
	}

	var manifest manifestResponse
	if err := json.Unmarshal(body, &manifest); err != nil {
		return time.Time{}
	}

	// Try to get date from history (v1 manifest)
	if len(manifest.History) > 0 {
		var compat v1CompatInfo
		if err := json.Unmarshal([]byte(manifest.History[0].V1Compat), &compat); err == nil {
			return parseTimestamp(compat.Created)
		}
	}

	return time.Time{}
}