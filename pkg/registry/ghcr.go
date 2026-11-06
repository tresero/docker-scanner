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
func tagsListURL(imageName string) string {
	return fmt.Sprintf("https://ghcr.io/v2/%s/tags/list?n=1000", imageName)
}

func (r *GHCRRegistry) FetchVersions(image models.Image) ([]models.RegistryVersion, error) {
	// lscr.io images are mirrored on Docker Hub. Docker Hub returns tags
	// newest-first, which gives us current versions reliably regardless
	// of total tag count. GHCR returns oldest-first and many lscr.io
	// images have thousands of tags, making pagination impractical.
	if image.Registry == "lscr.io" {
		return fetchDockerHubMirror(image.Name)
	}

	// lscr.io is a redirect to ghcr.io, so always query ghcr.io
	token, err := getGHCRToken(image.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get GHCR token: %w", err)
	}

	allTags, err := fetchAllGHCRTags(image.Name, image.Tag, token)
	if err != nil {
		return nil, err
	}

	versions := FilterAndSortTags(allTags)

	fillReleaseDates(versions, image.Tag, func(tag string) time.Time {
		return fetchManifestDate(image.Name, tag, token)
	})

	return versions, nil
}

// fillReleaseDates sets ReleasedAt using fetch, which is costly: each GHCR tag
// takes 3 HTTP requests (manifest-list -> platform-manifest -> config-blob).
// Only the newest config.VersionFetchLimit tags that fit the pin are dated, so
// images with thousands of tags stay cheap. Pick treats the rest as undated.
func fillReleaseDates(versions []models.RegistryVersion, currentTag string, fetch func(tag string) time.Time) {
	fetched := 0
	for i := range versions {
		if fetched == config.VersionFetchLimit {
			return
		}
		if !fitsPin(currentTag, versions[i].Tag) {
			continue
		}
		versions[i].ReleasedAt = fetch(versions[i].Tag)
		fetched++
	}
}

// fetchDockerHubMirror gets tags from Docker Hub for lscr.io images.
// Docker Hub returns tags ordered by last_updated (newest first), so we
// always get current versions regardless of total tag count.
func fetchDockerHubMirror(imageName string) ([]models.RegistryVersion, error) {
	// lscr.io images use the format "linuxserver/<app>" on both registries
	dockerHubName := imageName
	hubURL := fmt.Sprintf(
		"https://registry.hub.docker.com/v2/repositories/%s/tags/?page_size=100&ordering=last_updated",
		dockerHubName,
	)
	hubBody, err := httpGet(hubURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query Docker Hub for %s: %w", dockerHubName, err)
	}

	var result dockerHubResponse
	if err := json.Unmarshal(hubBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Docker Hub response: %w", err)
	}

	return FilterAndSortDockerHubTags(result.Results), nil
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


// fetchAllGHCRTags retrieves tags from GHCR using a two-pass strategy.
// GHCR returns tags in lexicographic order and caps each page at n=1000.
// Images with many tags (like open-webui with 10k+ tags) would require
// many pages to reach recent versions.
//
// Strategy:
//  1. Fetch the first page (oldest tags)
//  2. If the page is full (1000 tags) and currentTag is set, do a second
//     request starting from the current tag to capture newer versions
//  3. Follow a few pages of pagination from there
func fetchAllGHCRTags(imageName, currentTag, token string) ([]string, error) {
	const maxPages = 3
	auth := "Bearer " + token

	// First page: standard request
	body, headers, err := httpGetWithAuthFull(tagsListURL(imageName), auth, "")
	if err != nil {
		return nil, err
	}

	var tagList ghcrTagList
	if err := json.Unmarshal(body, &tagList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	allTags := tagList.Tags

	// If the first page is full, there are more tags. Jump ahead to the
	// user's current tag to get nearby (newer) versions.
	if len(tagList.Tags) >= 1000 && currentTag != "" && currentTag != "latest" {
		jumpURL := fmt.Sprintf("https://ghcr.io/v2/%s/tags/list?n=1000&last=%s", imageName, currentTag)
		for page := 0; page < maxPages; page++ {
			body, headers, err = httpGetWithAuthFull(jumpURL, auth, "")
			if err != nil {
				break
			}
			var page2 ghcrTagList
			if err := json.Unmarshal(body, &page2); err != nil {
				break
			}
			allTags = append(allTags, page2.Tags...)

			nextURL := parseLinkNext(headers.Get("Link"))
			if nextURL == "" {
				break
			}
			jumpURL = nextURL
		}
	} else if len(tagList.Tags) >= 1000 {
		// No current tag to jump from — follow pagination normally
		url := parseLinkNext(headers.Get("Link"))
		for page := 0; page < maxPages && url != ""; page++ {
			body, headers, err = httpGetWithAuthFull(url, auth, "")
			if err != nil {
				break
			}
			var pageN ghcrTagList
			if err := json.Unmarshal(body, &pageN); err != nil {
				break
			}
			allTags = append(allTags, pageN.Tags...)
			url = parseLinkNext(headers.Get("Link"))
		}
	}

	return allTags, nil
}

// parseLinkNext extracts the "next" URL from an OCI Link header.
// Format: </v2/name/tags/list?last=tag&n=1000>; rel="next"
func parseLinkNext(header string) string {
	if header == "" {
		return ""
	}
	// Link header format: <URL>; rel="next"
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start < 0 || end < 0 || end <= start {
			continue
		}
		path := part[start+1 : end]
		// GHCR returns relative paths; make them absolute
		if strings.HasPrefix(path, "/") {
			return "https://ghcr.io" + path
		}
		return path
	}
	return ""
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
	body, _, err := httpGetWithAuthFull(url, auth, accept)
	return body, err
}

// httpGetWithAuthFull is like httpGetWithAuth but also returns response headers.
func httpGetWithAuthFull(url, auth, accept string) ([]byte, http.Header, error) {
	client := &http.Client{
		Timeout: time.Duration(config.HTTPTimeoutSeconds) * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", auth)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	return body, resp.Header, err
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