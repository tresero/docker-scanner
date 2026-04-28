package registry

import (
	"strings"
	"testing"
	"time"
)

func TestTagsListURL_RequestsLargePageSize(t *testing.T) {
	url := tagsListURL("muchobien/pocketbase")

	if !strings.Contains(url, "ghcr.io/v2/muchobien/pocketbase/tags/list") {
		t.Errorf("URL missing expected path: %s", url)
	}
	if !strings.Contains(url, "n=1000") {
		t.Errorf("URL must request large page size to avoid GHCR's silent 103-tag cap, got: %s", url)
	}
}

func TestParseManifestList_PicksAmd64(t *testing.T) {
	body := []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
		"manifests": [
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"digest": "sha256:amd64digest",
				"platform": {"architecture": "amd64", "os": "linux"}
			},
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"digest": "sha256:arm64digest",
				"platform": {"architecture": "arm64", "os": "linux"}
			}
		]
	}`)

	digest, err := pickAmd64ManifestDigest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digest != "sha256:amd64digest" {
		t.Errorf("expected amd64 digest, got %q", digest)
	}
}

func TestParseManifestList_NoAmd64ReturnsError(t *testing.T) {
	body := []byte(`{
		"schemaVersion": 2,
		"manifests": [
			{"digest": "sha256:arm64only", "platform": {"architecture": "arm64", "os": "linux"}}
		]
	}`)

	_, err := pickAmd64ManifestDigest(body)
	if err == nil {
		t.Error("expected error when no amd64 manifest is present")
	}
}

func TestParseManifestList_NotAList(t *testing.T) {
	// A single-arch manifest doesn't have a manifests[] field.
	// Per design: skip it, let the date fallback handle it.
	body := []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": {"digest": "sha256:configdigest"}
	}`)

	_, err := pickAmd64ManifestDigest(body)
	if err == nil {
		t.Error("expected error when body is a single-arch manifest, not a list")
	}
}

func TestParsePlatformManifest_ExtractsConfigDigest(t *testing.T) {
	body := []byte(`{
		"schemaVersion": 2,
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": {
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"digest": "sha256:configdigest123",
			"size": 2286
		},
		"layers": [
			{"digest": "sha256:layerdigest"}
		]
	}`)

	digest, err := extractConfigDigest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digest != "sha256:configdigest123" {
		t.Errorf("expected config digest, got %q", digest)
	}
}

func TestParseImageConfig_ExtractsCreatedTimestamp(t *testing.T) {
	// Real shape from GHCR — the config blob has a top-level created
	// plus per-layer created entries in history[]. We want the top-level one.
	body := []byte(`{
		"created": "2026-04-27T08:29:15.949857998Z",
		"architecture": "amd64",
		"os": "linux",
		"history": [
			{"created": "2026-04-15T20:01:40.139676757Z"},
			{"created": "2026-04-19T07:22:03.500308455Z"}
		]
	}`)

	created, err := extractCreatedTimestamp(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected, _ := time.Parse(time.RFC3339Nano, "2026-04-27T08:29:15.949857998Z")
	if !created.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, created)
	}
}

func TestParseImageConfig_MissingCreated(t *testing.T) {
	body := []byte(`{
		"architecture": "amd64",
		"os": "linux"
	}`)

	_, err := extractCreatedTimestamp(body)
	if err == nil {
		t.Error("expected error when created field is missing")
	}
}
func TestManifestAcceptHeader_IncludesAllFormats(t *testing.T) {
	// GHCR strictly enforces Accept headers — if a media type isn't listed,
	// it returns 404 MANIFEST_UNKNOWN. We need all four because images
	// vary: older Docker manifest formats and newer OCI index formats are
	// both in active use.
	required := []string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}

	for _, mt := range required {
		if !strings.Contains(manifestAcceptHeader, mt) {
			t.Errorf("manifestAcceptHeader missing required media type: %s", mt)
		}
	}
}