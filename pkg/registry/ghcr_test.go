package registry

import (
	"strings"
	"testing"
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