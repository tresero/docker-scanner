package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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

// fakeGHCR answers tag list requests the way GHCR does: 1000 tags a page, a
// Link header on every page but the last, and last= starting after a tag.
type fakeGHCR struct {
	tags          []string
	failOnCall    int
	corruptOnCall int
	calls         int
}

func (f *fakeGHCR) get(rawURL string) ([]byte, http.Header, error) {
	f.calls++
	if f.calls == f.failOnCall {
		return nil, nil, errors.New("registry returned status 429")
	}
	if f.calls == f.corruptOnCall {
		return []byte("not json"), http.Header{}, nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, err
	}
	start := 0
	if last := parsed.Query().Get("last"); last != "" {
		for i, tag := range f.tags {
			if tag == last {
				start = i + 1
				break
			}
		}
	}
	end := min(start+1000, len(f.tags))

	headers := http.Header{}
	if end < len(f.tags) {
		headers.Set("Link", fmt.Sprintf(`</v2/x/tags/list?last=%s&n=1000>; rel="next"`, f.tags[end-1]))
	}
	body, err := json.Marshal(ghcrTagList{Tags: f.tags[start:end]})
	return body, headers, err
}

// noiseTags returns n git-hash tags, which is most of what a busy image pushes.
func noiseTags(prefix string, n int) []string {
	tags := make([]string, n)
	for i := range tags {
		tags[i] = fmt.Sprintf("git-%s%05x", prefix, i)
	}
	return tags
}

// openWebUITags has the shape of ghcr.io/open-webui/open-webui: tags come in push
// order, with versions spread far apart between thousands of git-hash tags, and
// the newest version close to the end.
func openWebUITags() []string {
	var tags []string
	add := func(t ...string) { tags = append(tags, t...) }
	add("main", "v0.1.118", "0.1", "0.1.118")
	add(noiseTags("a", 1200)...)
	add("latest", "v0.2.5", "0.2.5")
	add(noiseTags("b", 300)...)
	add("v0.5.7", "0.5.7", "0.5.7-cuda", "v0.5.7-cuda", "git-b72150c-ollama")
	add(noiseTags("c", 5000)...)
	add("v0.12.4", "0.12.4", "v0.12.4-cuda", "0.12.4-cuda")
	add(noiseTags("d", 700)...)
	add("v0.13.0", "0.13.0", "v0.13.0-cuda", "0.13.0-cuda")
	add(noiseTags("e", 300)...)
	add("v0.13.1", "0.13.1")
	return tags
}

func TestFetchAllGHCRTags(t *testing.T) {
	last := openWebUITags()
	cases := []struct {
		name          string
		tags          []string
		failOnCall    int
		corruptOnCall int
		wantCount     int
		wantCalls     int
		wantErr       string
	}{
		{name: "an empty list", wantCount: 0, wantCalls: 1},
		{name: "a short list takes one request", tags: noiseTags("a", 215), wantCount: 215, wantCalls: 1},
		{name: "exactly one full page takes one request", tags: noiseTags("a", 1000), wantCount: 1000, wantCalls: 1},
		{name: "one tag past a full page reads the second page", tags: noiseTags("a", 1001), wantCount: 1001, wantCalls: 2},
		{name: "every page of a long list is read", tags: last, wantCount: len(last), wantCalls: 8},
		{name: "the last allowed page is still read", tags: noiseTags("a", maxGHCRPages*1000), wantCount: maxGHCRPages * 1000, wantCalls: maxGHCRPages},
		{name: "a list past the page limit is an error", tags: noiseTags("a", maxGHCRPages*1000+1), wantCalls: maxGHCRPages, wantErr: "couldn't read"},
		{name: "a failed first page is an error", tags: last, failOnCall: 1, wantCalls: 1, wantErr: "429"},
		{name: "a failed middle page is an error, not a partial list", tags: last, failOnCall: 4, wantCalls: 4, wantErr: "429"},
		{name: "an unreadable middle page is an error, not a partial list", tags: last, corruptOnCall: 4, wantCalls: 4, wantErr: "parse"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ghcr := &fakeGHCR{tags: tc.tags, failOnCall: tc.failOnCall, corruptOnCall: tc.corruptOnCall}
			got, err := fetchAllGHCRTags(ghcr.get, "open-webui/open-webui")

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
				}
				if got != nil {
					t.Errorf("got %d tags with the error, want none", len(got))
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tc.wantCount {
				t.Errorf("got %d tags, want %d", len(got), tc.wantCount)
			}
			if ghcr.calls != tc.wantCalls {
				t.Errorf("made %d requests, want %d", ghcr.calls, tc.wantCalls)
			}
		})
	}
}

// TestGHCRLongTagList runs the pins from issue #18 from tag list to recommendation.
func TestGHCRLongTagList(t *testing.T) {
	released := map[string]time.Time{
		"0.13.1": fixedNow.AddDate(0, 0, -1), // too new for safe-days 3
		"0.13.0": fixedNow.AddDate(0, 0, -4),
		"0.12.4": fixedNow.AddDate(0, 0, -30),
		"0.5.7":  fixedNow.AddDate(-1, -6, 0),
		"0.2.5":  fixedNow.AddDate(-2, 0, 0),
	}
	cases := []struct {
		name      string
		pin       string
		running   string
		want      string
		wantNote  string
		confirmed bool
	}{
		{name: "a main pin gets the newest safe version", pin: "main", want: "0.13.0"},
		{name: "a latest pin gets the newest safe version", pin: "latest", want: "0.13.0"},
		{name: "an old pin isn't stuck near its own place in the list", pin: "v0.5.7", want: "v0.13.0"},
		{name: "a pin without v keeps its spelling", pin: "0.5.7", want: "0.13.0"},
		{name: "a variant pin keeps its variant", pin: "v0.5.7-cuda", want: "v0.13.0-cuda"},
		{name: "a pin on the newest safe version is confirmed", pin: "v0.13.0", running: "v0.13.0", want: "v0.13.0", wantNote: "newest is", confirmed: true},
		{name: "a pin on a version too new to recommend is confirmed", pin: "v0.13.1", running: "v0.13.1", wantNote: "already on the newest", confirmed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ghcr := &fakeGHCR{tags: openWebUITags()}
			tags, err := fetchAllGHCRTags(ghcr.get, "open-webui/open-webui")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			versions := FilterAndSortTags(tags)
			fillReleaseDates(versions, tc.pin, func(tag string) (time.Time, string) {
				number := strings.TrimPrefix(strings.TrimSuffix(tag, "-cuda"), "v")
				return released[number], ""
			})

			pick := Pick(PickInput{Versions: versions, CurrentTag: tc.pin, RunningVersion: tc.running, SafeDays: 3, Now: fixedNow})

			if pick.Version != tc.want {
				t.Errorf("Version = %q, want %q (note: %q)", pick.Version, tc.want, pick.Note)
			}
			if tc.wantNote != "" && !strings.Contains(pick.Note, tc.wantNote) {
				t.Errorf("Note = %q, want it to mention %q", pick.Note, tc.wantNote)
			}
			if tc.confirmed && pick.Unverified {
				t.Errorf("Unverified = true, want the scan to confirm this pin (note: %q)", pick.Note)
			}
		})
	}
}
