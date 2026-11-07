package registry

import (
	"docker-scanner/pkg/models"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewerBuild(t *testing.T) {
	built := fixedNow.AddDate(0, -3, 0)
	tests := []struct {
		name    string
		build   TagBuild
		digests []string
		built   time.Time
		want    bool
	}{
		{name: "the running image is what the tag points to", build: TagBuild{Digest: "sha256:new"}, digests: []string{"sha256:new"}},
		{name: "the tag has moved", build: TagBuild{Digest: "sha256:new"}, digests: []string{"sha256:old"}, want: true},
		{name: "one of several running digests matches", build: TagBuild{Digest: "sha256:new"}, digests: []string{"sha256:old", "sha256:new"}},
		{name: "a locally built image has no digest to compare", build: TagBuild{Digest: "sha256:new"}},
		{name: "the registry gave no digest", digests: []string{"sha256:old"}},
		{name: "pushed after the running image was built", build: TagBuild{Digest: "sha256:new", Pushed: fixedNow}, digests: []string{"sha256:old"}, built: built, want: true},
		{name: "pushed before the running image was built is an older copy", build: TagBuild{Digest: "sha256:new", Pushed: built.AddDate(0, -1, 0)}, digests: []string{"sha256:old"}, built: built},
		{name: "no running build date still compares digests", build: TagBuild{Digest: "sha256:new", Pushed: fixedNow}, digests: []string{"sha256:old"}, want: true},
		{name: "no push date still compares digests", build: TagBuild{Digest: "sha256:new"}, digests: []string{"sha256:old"}, built: built, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewerBuild(tt.build, tt.digests, tt.built); got != tt.want {
				t.Errorf("NewerBuild = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHubTagBuild(t *testing.T) {
	tests := []struct {
		name       string
		repo       string
		tag        string
		body       string
		err        error
		wantURL    string
		wantDigest string
		wantPushed time.Time
		wantErr    bool
	}{
		{
			name: "an official image", repo: "library/mariadb", tag: "11.4",
			body:       `{"name":"11.4","digest":"sha256:abc","last_updated":"2026-10-27T01:02:03.123456Z"}`,
			wantURL:    "/v2/repositories/library/mariadb/tags/11.4",
			wantDigest: "sha256:abc", wantPushed: time.Date(2026, 10, 27, 1, 2, 3, 123456000, time.UTC),
		},
		{
			name: "no date is still a digest", repo: "a/b", tag: "apache", body: `{"name":"apache","digest":"sha256:abc"}`,
			wantURL: "/v2/repositories/a/b/tags/apache", wantDigest: "sha256:abc",
		},
		{name: "a failed request is an error", repo: "a/b", tag: "1", err: errors.New("status 429"), wantErr: true},
		{name: "an unreadable response is an error", repo: "a/b", tag: "1", body: "not json", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requested string
			get := func(url string) ([]byte, error) {
				requested = url
				return []byte(tt.body), tt.err
			}
			build, err := hubTagBuild(get, tt.repo, tt.tag)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !strings.Contains(requested, tt.wantURL) {
				t.Errorf("requested %q, want it to contain %q", requested, tt.wantURL)
			}
			if build.Digest != tt.wantDigest {
				t.Errorf("Digest = %q, want %q", build.Digest, tt.wantDigest)
			}
			if !build.Pushed.Equal(tt.wantPushed) {
				t.Errorf("Pushed = %v, want %v", build.Pushed, tt.wantPushed)
			}
		})
	}
}

func TestGenericTagBuild(t *testing.T) {
	image := models.Image{Registry: "quay.io", Name: "keycloak/keycloak", Tag: "26.8.2"}
	digestHeader := http.Header{"Docker-Content-Digest": {"sha256:abc"}}
	tests := []struct {
		name       string
		headers    http.Header
		err        error
		hubBody    string
		wantDigest string
		wantErr    bool
	}{
		{name: "the digest comes from the header", headers: digestHeader, wantDigest: "sha256:abc"},
		{name: "no digest header says nothing", headers: http.Header{}},
		{name: "a failed request is an error", err: &registryStatusError{Status: 404}, wantErr: true},
		{name: "a 401 from another login service is an error", err: &registryStatusError{Status: 401, Challenge: `Bearer service="quay.io"`}, wantErr: true},
		{
			name: "a Docker Hub front asks Docker Hub", err: &registryStatusError{Status: 401, Challenge: dockerHubChallenge},
			hubBody: `{"digest":"sha256:fromhub"}`, wantDigest: "sha256:fromhub",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			get := func(url string) ([]byte, http.Header, error) {
				if strings.Contains(url, "/v2/repositories/") {
					return []byte(tt.hubBody), http.Header{}, nil
				}
				if !strings.HasSuffix(url, "quay.io/v2/keycloak/keycloak/manifests/26.8.2") {
					t.Errorf("unexpected request %q", url)
				}
				return nil, tt.headers, tt.err
			}
			build, err := genericTagBuild(get, image)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if build.Digest != tt.wantDigest {
				t.Errorf("Digest = %q, want %q", build.Digest, tt.wantDigest)
			}
		})
	}
}

// plainRegistry can list versions but can't say what a tag points to.
type plainRegistry struct{}

func (plainRegistry) Name() string         { return "plain" }
func (plainRegistry) Supports(string) bool { return true }
func (plainRegistry) FetchVersions(models.Image) ([]models.RegistryVersion, error) {
	return nil, nil
}

type buildRegistry struct {
	plainRegistry
	build TagBuild
}

func (b buildRegistry) CurrentBuild(models.Image) (TagBuild, error) { return b.build, nil }

func TestCurrentBuild(t *testing.T) {
	want := TagBuild{Digest: "sha256:abc"}
	got, err := CurrentBuild([]Registry{buildRegistry{build: want}}, models.Image{})
	if err != nil || got != want {
		t.Errorf("got %v, %v, want %v", got, err, want)
	}
	got, err = CurrentBuild([]Registry{plainRegistry{}}, models.Image{})
	if err != nil || got != (TagBuild{}) {
		t.Errorf("a registry with no builds gave %v, %v, want nothing", got, err)
	}
}
