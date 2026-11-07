package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"testing"
)

// fakeHub serves Docker Hub tag responses by name filter and page, like the
// real API: each filter has its own list, split into pages with a next link.
type fakeHub struct {
	t       *testing.T
	tags    map[string][]DockerHubTag // by name filter
	failFor map[string]error          // by name filter
	calls   []string                  // "filter#page" for each request
}

func (f *fakeHub) get(rawURL string) ([]byte, error) {
	f.t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		f.t.Fatalf("bad URL %q: %v", rawURL, err)
	}
	query := parsed.Query()
	filter := query.Get("name")
	if query.Get("ordering") != "last_updated" {
		f.t.Errorf("URL %q isn't ordered by last_updated", rawURL)
	}

	page := 1
	if p := query.Get("page"); p != "" {
		if _, err := fmt.Sscanf(p, "%d", &page); err != nil {
			f.t.Fatalf("bad page in %q", rawURL)
		}
	}
	f.calls = append(f.calls, fmt.Sprintf("%s#%d", filter, page))

	if err := f.failFor[filter]; err != nil {
		return nil, err
	}

	const size = 100
	all := f.tags[filter]
	start := min((page-1)*size, len(all))
	end := min(start+size, len(all))

	response := struct {
		Next    string         `json:"next"`
		Results []DockerHubTag `json:"results"`
	}{Results: all[start:end]}
	if end < len(all) {
		query.Set("page", fmt.Sprint(page+1))
		parsed.RawQuery = query.Encode()
		response.Next = parsed.String()
	}
	return json.Marshal(response)
}

// builds returns the per-architecture flood real images push for one version.
func builds(version string, pushed int) []DockerHubTag {
	var tags []DockerHubTag
	for _, suffix := range []string{"-626a58e-distroless", "-626a58e", "-distroless", "", "-626a58e-arm64", "-arm64", "-amd64", "-distroless-arm64", "-distroless-amd64"} {
		tags = append(tags, hubTag(version+suffix, daysAgo(pushed)))
	}
	return tags
}

// runnersDotFiltered is what name=. returns for n8nio/runners: maintenance builds
// of the old major fill the first page, so 3.x is only on later pages.
func runnersDotFiltered() []DockerHubTag {
	var tags []DockerHubTag
	for i := 0; i < 12; i++ {
		tags = append(tags, builds(fmt.Sprintf("2.47.%d", 12-i), 0)...)
	}
	return append(tags, builds("3.1.0", 4)...)
}

func runnersMajorFiltered() []DockerHubTag {
	return append(builds("3.1.0", 4), builds("3.0.1", 10)...)
}

// TestHubLookup_FindsVersionsBehindAFlood comes from a real scan: nightly and
// maintenance builds hid 3.1.0 from a 3.0.1 pin, which then showed as current.
func TestHubLookup_FindsVersionsBehindAFlood(t *testing.T) {
	hub := &fakeHub{t: t, tags: map[string][]DockerHubTag{
		"":   {hubTag("nightly", daysAgo(0)), hubTag("nightly-amd64", daysAgo(0))},
		".":  runnersDotFiltered(),
		"3.": runnersMajorFiltered(),
	}}

	tags, err := fetchHubTags(hub.get, "n8nio/runners", "3.0.1")
	if err != nil {
		t.Fatal(err)
	}
	got := Pick(PickInput{
		Versions:       FilterAndSortDockerHubTags(tags),
		CurrentTag:     "3.0.1",
		RunningVersion: "3.0.1",
		SafeDays:       3,
		Now:            fixedNow,
	})
	if got.Version != "3.1.0" {
		t.Errorf("Version = %q, want 3.1.0 (note %q)", got.Version, got.Note)
	}
}

func TestPinNameFilter(t *testing.T) {
	tests := []struct {
		tag  string
		want string
	}{
		{"3.0.1", "3."},
		{"v2.7.0", "2."},
		{"11.4", "11."},
		{"8-alpine", "8."},
		{"16", "16."},
		{"7.10.7-alpine", "7."},
		{"0.21.1", "0."},
		{"apache", "-apache"},
		{"fpm-alpine", "-fpm-alpine"},
		{"latest", ""},
		{"stable", ""},
		{"a1b2c3d", ""},
		{"20240501", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			if got := pinNameFilter(tt.tag); got != tt.want {
				t.Errorf("pinNameFilter(%q) = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}

// TestFetchHubTags pins down which requests are made, since each one counts
// against Docker Hub's rate limit.
func TestFetchHubTags(t *testing.T) {
	version := func(i int) DockerHubTag { return hubTag(fmt.Sprintf("1.%d.0", i), daysAgo(10)) }
	many := func(from, count int) []DockerHubTag {
		var tags []DockerHubTag
		for i := from; i < from+count; i++ {
			tags = append(tags, version(i))
		}
		return tags
	}

	tests := []struct {
		name      string
		pin       string
		tags      map[string][]DockerHubTag
		wantCalls []string
		wantTags  int
	}{
		{
			name:      "a short list is complete, so one request is enough",
			pin:       "1.0.0",
			tags:      map[string][]DockerHubTag{".": many(0, 3)},
			wantCalls: []string{".#1"},
			wantTags:  3,
		},
		{
			name:      "no tags at all",
			pin:       "1.0.0",
			tags:      map[string][]DockerHubTag{},
			wantCalls: []string{".#1"},
			wantTags:  0,
		},
		{
			name:      "a floating pin has nothing to filter on",
			pin:       "latest",
			tags:      map[string][]DockerHubTag{".": many(0, 150)},
			wantCalls: []string{".#1"},
			wantTags:  100,
		},
		{
			name:      "tags found by both requests are kept once",
			pin:       "1.0.0",
			tags:      map[string][]DockerHubTag{".": many(0, 150), "1.": many(0, 50)},
			wantCalls: []string{".#1", "1.#1"},
			wantTags:  100,
		},
		{
			name:      "the pin's filter follows later pages",
			pin:       "1.0.0",
			tags:      map[string][]DockerHubTag{".": many(0, 150), "1.": many(0, 250)},
			wantCalls: []string{".#1", "1.#1", "1.#2", "1.#3"},
			wantTags:  250,
		},
		{
			name:      "the pin's filter stops after 3 pages",
			pin:       "1.0.0",
			tags:      map[string][]DockerHubTag{".": many(0, 150), "1.": many(0, 900)},
			wantCalls: []string{".#1", "1.#1", "1.#2", "1.#3"},
			wantTags:  300,
		},
		{
			name:      "a word pin filters on its variant",
			pin:       "apache",
			tags:      map[string][]DockerHubTag{".": many(0, 150), "-apache": {hubTag("7.1.3-apache", daysAgo(10))}},
			wantCalls: []string{".#1", "-apache#1"},
			wantTags:  101,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := &fakeHub{t: t, tags: tt.tags}
			tags, err := fetchHubTags(hub.get, "org/app", tt.pin)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(hub.calls) != fmt.Sprint(tt.wantCalls) {
				t.Errorf("requests = %v, want %v", hub.calls, tt.wantCalls)
			}
			if len(tags) != tt.wantTags {
				t.Errorf("got %d tags, want %d", len(tags), tt.wantTags)
			}
		})
	}
}

func TestFetchHubTags_Failures(t *testing.T) {
	limited := errors.New("registry returned status 429")

	t.Run("the first request failing is an error", func(t *testing.T) {
		hub := &fakeHub{t: t, failFor: map[string]error{".": limited}}
		if _, err := fetchHubTags(hub.get, "org/app", "1.0.0"); !errors.Is(err, limited) {
			t.Errorf("err = %v, want %v", err, limited)
		}
	})

	t.Run("the pin's filter failing keeps what was read", func(t *testing.T) {
		var dotted []DockerHubTag
		for i := 0; i < 150; i++ {
			dotted = append(dotted, hubTag(fmt.Sprintf("1.%d.0", i), daysAgo(10)))
		}
		hub := &fakeHub{t: t, tags: map[string][]DockerHubTag{".": dotted}, failFor: map[string]error{"1.": limited}}
		tags, err := fetchHubTags(hub.get, "org/app", "1.0.0")
		if err != nil {
			t.Fatalf("err = %v, want none", err)
		}
		if len(tags) != 100 {
			t.Errorf("got %d tags, want 100", len(tags))
		}
	})

	t.Run("an unparseable response is an error", func(t *testing.T) {
		get := func(string) ([]byte, error) { return []byte("not json"), nil }
		if _, err := fetchHubTags(get, "org/app", "1.0.0"); err == nil {
			t.Error("err = nil, want a parse error")
		}
	})
}
