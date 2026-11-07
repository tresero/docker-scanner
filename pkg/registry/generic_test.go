package registry

import (
	"docker-scanner/pkg/models"
	"errors"
	"strings"
	"testing"
)

const dockerHubChallenge = `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:n8nio/n8n:pull"`

// TestGenericRegistry_DockerHubProxy comes from a real scan: docker.n8n.io answers
// 401 with a Docker Hub login challenge because it serves Docker Hub's repository.
func TestGenericRegistry_DockerHubProxy(t *testing.T) {
	hub := &fakeHub{t: t, tags: map[string][]DockerHubTag{
		".": {hubTag("3.1.0", daysAgo(4)), hubTag("3.1.0-arm64", daysAgo(4)), hubTag("3.0.1", daysAgo(10))},
	}}
	var requested []string
	get := func(url string) ([]byte, error) {
		requested = append(requested, url)
		if strings.HasPrefix(url, "https://docker.n8n.io/") {
			return nil, &registryStatusError{Status: 401, Challenge: dockerHubChallenge}
		}
		if !strings.Contains(url, "/repositories/n8nio/n8n/tags/") {
			t.Errorf("unexpected request %q", url)
		}
		return hub.get(url)
	}

	image := models.Image{Registry: "docker.n8n.io", Name: "n8nio/n8n", Tag: "3.0.1"}
	versions, err := fetchGenericVersions(get, image)
	if err != nil {
		t.Fatal(err)
	}
	got := Pick(PickInput{Versions: versions, CurrentTag: image.Tag, RunningVersion: "3.0.1", SafeDays: 3, Now: fixedNow})
	if got.Version != "3.1.0" {
		t.Errorf("Version = %q, want 3.1.0 (note %q)", got.Version, got.Note)
	}
	if got.Unverified {
		t.Error("Unverified = true, want false")
	}
}

func TestGenericRegistry_OfficialImageThroughAProxy(t *testing.T) {
	hub := &fakeHub{t: t, tags: map[string][]DockerHubTag{".": {hubTag("8.1.0", daysAgo(10))}}}
	get := func(url string) ([]byte, error) {
		if strings.HasPrefix(url, "https://mirror.example.com/") {
			return nil, &registryStatusError{Status: 401, Challenge: dockerHubChallenge}
		}
		if !strings.Contains(url, "/repositories/library/redis/tags/") {
			t.Errorf("unexpected request %q", url)
		}
		return hub.get(url)
	}
	versions, err := fetchGenericVersions(get, models.Image{Registry: "mirror.example.com", Name: "redis", Tag: "8.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Tag != "8.1.0" {
		t.Errorf("versions = %v", versions)
	}
}

// TestGenericRegistry_Failures keeps every other failure an error, so the report
// says it couldn't check instead of guessing.
func TestGenericRegistry_Failures(t *testing.T) {
	image := models.Image{Registry: "registry.example.com", Name: "org/app", Tag: "1.0.0"}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"a login challenge from another service", &registryStatusError{Status: 401, Challenge: `Bearer realm="https://registry.example.com/token",service="registry.example.com"`}, "status 401"},
		{"401 with no challenge", &registryStatusError{Status: 401}, "status 401"},
		{"403 naming Docker Hub", &registryStatusError{Status: 403, Challenge: dockerHubChallenge}, "status 403"},
		{"rate limited", &registryStatusError{Status: 429}, "status 429"},
		{"not found", &registryStatusError{Status: 404}, "status 404"},
		{"network failure", errors.New("connection refused"), "connection refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			get := func(string) ([]byte, error) {
				calls++
				return nil, tt.err
			}
			_, err := fetchGenericVersions(get, image)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), image.Registry) {
				t.Errorf("err = %v, want it to mention %q and the registry", err, tt.want)
			}
			if calls != 1 {
				t.Errorf("made %d requests, want 1", calls)
			}
		})
	}

	t.Run("Docker Hub failing after a proxy challenge is an error", func(t *testing.T) {
		get := func(url string) ([]byte, error) {
			if strings.Contains(url, "registry.hub.docker.com") {
				return nil, &registryStatusError{Status: 429}
			}
			return nil, &registryStatusError{Status: 401, Challenge: dockerHubChallenge}
		}
		_, err := fetchGenericVersions(get, image)
		if err == nil || !strings.Contains(err.Error(), "status 429") {
			t.Errorf("err = %v, want the Docker Hub failure", err)
		}
	})

	t.Run("a readable registry still parses its tag list", func(t *testing.T) {
		get := func(string) ([]byte, error) { return []byte(`{"tags":["1.2.0","latest"]}`), nil }
		versions, err := fetchGenericVersions(get, image)
		if err != nil || len(versions) != 1 || versions[0].Tag != "1.2.0" {
			t.Errorf("versions = %v, err = %v", versions, err)
		}
	})

	t.Run("an unparseable tag list is an error", func(t *testing.T) {
		get := func(string) ([]byte, error) { return []byte("not json"), nil }
		if _, err := fetchGenericVersions(get, image); err == nil {
			t.Error("err = nil, want a parse error")
		}
	})
}
