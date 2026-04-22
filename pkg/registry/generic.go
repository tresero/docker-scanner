package registry

import (
	"docker-scanner/pkg/models"
	"encoding/json"
	"fmt"
)

// GenericRegistry handles any OCI-compatible registry
// as a fallback when no specific handler matches
type GenericRegistry struct{}

type genericTagList struct {
	Tags []string `json:"tags"`
}

func (r *GenericRegistry) Name() string {
	return "Generic OCI Registry"
}

func (r *GenericRegistry) Supports(registry string) bool {
	return true
}

func (r *GenericRegistry) FetchVersions(image models.Image) ([]models.RegistryVersion, error) {
	url := fmt.Sprintf("https://%s/v2/%s/tags/list", image.Registry, image.Name)

	body, err := httpGet(url)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", image.Registry, err)
	}

	var tagList genericTagList
	if err := json.Unmarshal(body, &tagList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return FilterAndSortTags(tagList.Tags), nil
}