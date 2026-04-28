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

// manifestList is the multi-arch manifest list / OCI image index that GHCR
// returns by default for any tag. Each entry points to a per-platform
// manifest by digest.
type manifestList struct {
	Manifests []manifestListEntry `json:"manifests"`
}

type manifestListEntry struct {
	Digest   string               `json:"digest"`
	Platform manifestListPlatform `json:"platform"`
}

type manifestListPlatform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

// platformManifest is a single-arch v2 manifest. Its config.digest points to
// the image config blob, which is where the build timestamp lives.
type platformManifest struct {
	Config struct {
		Digest string `json:"digest"`
	} `json:"config"`
}

// imageConfig is the JSON blob fetched from /v2/{name}/blobs/{digest}.
// The top-level Created field is the image build timestamp.
type imageConfig struct {
	Created string `json:"created"`
}

// manifestAcceptHeader lists every manifest media type we know how to
// parse. GHCR strictly enforces Accept headers — if the format the
// registry has stored isn't listed, it returns 404 MANIFEST_UNKNOWN.
// The four types cover both the older Docker formats and the newer OCI
// formats; both are in active use across different images.
const manifestAcceptHeader = "application/vnd.oci.image.index.v1+json," +
	"application/vnd.docker.distribution.manifest.list.v2+json," +
	"application/vnd.oci.image.manifest.v1+json," +
	"application/vnd.docker.distribution.manifest.v2+json"

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

	body, err := httpGetWithAuth(tagsListURL(image.Name), "Bearer "+token, "")
	if err != nil {
		return nil, err
	}

	var tagList ghcrTagList
	if err := json.Unmarshal(body, &tagList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	versions := FilterAndSortTags(tagList.Tags)

	// Fetch manifest dates for the filtered tags (at most 10 calls).
	// Each tag costs 3 HTTP requests to resolve the date through the
	// manifest-list -> platform-manifest -> config-blob chain.
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

// pickAmd64ManifestDigest parses a manifest list and returns the digest of
// the amd64/linux platform manifest. Returns an error if the body isn't a
// manifest list, has no amd64 entry, or fails to parse.
func pickAmd64ManifestDigest(body []byte) (string, error) {
	var list manifestList
	if err := json.Unmarshal(body, &list); err != nil {
		return "", fmt.Errorf("parse manifest list: %w", err)
	}
	if len(list.Manifests) == 0 {
		return "", fmt.Errorf("not a manifest list (no manifests array)")
	}
	for _, m := range list.Manifests {
		if m.Platform.Architecture == "amd64" && m.Platform.OS == "linux" {
			return m.Digest, nil
		}
	}
	return "", fmt.Errorf("no amd64/linux manifest found")
}

// extractConfigDigest parses a single-arch manifest and returns the digest
// of its config blob.
func extractConfigDigest(body []byte) (string, error) {
	var pm platformManifest
	if err := json.Unmarshal(body, &pm); err != nil {
		return "", fmt.Errorf("parse platform manifest: %w", err)
	}
	if pm.Config.Digest == "" {
		return "", fmt.Errorf("no config digest in manifest")
	}
	return pm.Config.Digest, nil
}

// extractCreatedTimestamp parses an image config blob and returns the
// top-level Created field. Note: there are also per-layer created entries
// inside history[] — those are not what we want.
func extractCreatedTimestamp(body []byte) (time.Time, error) {
	var cfg imageConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return time.Time{}, fmt.Errorf("parse image config: %w", err)
	}
	if cfg.Created == "" {
		return time.Time{}, fmt.Errorf("no created timestamp in config")
	}
	t := parseTimestamp(cfg.Created)
	if t.IsZero() {
		return time.Time{}, fmt.Errorf("unrecognized timestamp format: %s", cfg.Created)
	}
	return t, nil
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

// httpGetWithAuth performs a GET with an Authorization header. If accept
// is non-empty, it is set as the Accept header — required for GHCR
// manifest endpoints which strictly enforce content negotiation.
func httpGetWithAuth(url, auth, accept string) ([]byte, error) {
	client := &http.Client{
		Timeout: time.Duration(config.HTTPTimeoutSeconds) * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

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

// fetchManifestDate resolves the build timestamp for a tag through GHCR's
// 3-step chain: manifest-list -> platform manifest -> config blob.
//
// Returns time.Time{} (zero) on any failure — including the "image is
// single-arch and not a list" case. The caller (PickSafeVersion) treats
// zero values as "unknown" and falls back to position-based recommendation.
func fetchManifestDate(imageName, tag, token string) time.Time {
	// Step 1: manifest list
	listURL := fmt.Sprintf("https://ghcr.io/v2/%s/manifests/%s", imageName, tag)
	listBody, err := httpGetWithAuth(listURL, "Bearer "+token, manifestAcceptHeader)
	if err != nil {
		return time.Time{}
	}

	platformDigest, err := pickAmd64ManifestDigest(listBody)
	if err != nil {
		return time.Time{}
	}

	// Step 2: platform manifest
	platformURL := fmt.Sprintf("https://ghcr.io/v2/%s/manifests/%s", imageName, platformDigest)
	platformBody, err := httpGetWithAuth(platformURL, "Bearer "+token, manifestAcceptHeader)
	if err != nil {
		return time.Time{}
	}

	configDigest, err := extractConfigDigest(platformBody)
	if err != nil {
		return time.Time{}
	}

	// Step 3: config blob. httpGetWithAuth follows redirects by default
	// (Go's http.Client default), which is what we want — GHCR returns a
	// 307 redirect to a CDN URL where the blob actually lives.
	blobURL := fmt.Sprintf("https://ghcr.io/v2/%s/blobs/%s", imageName, configDigest)
	blobBody, err := httpGetWithAuth(blobURL, "Bearer "+token, "")
	if err != nil {
		return time.Time{}
	}

	created, err := extractCreatedTimestamp(blobBody)
	if err != nil {
		return time.Time{}
	}
	return created
}