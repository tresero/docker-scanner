package registry

import "docker-scanner/pkg/models"

// Registry is the interface for fetching version info from container registries.
// Implement this to add support for new registries.
type Registry interface {
    Name() string
    Supports(registry string) bool
    FetchVersions(image models.Image) ([]models.RegistryVersion, error)
}

// Lookup finds the right registry handler for an image
// and fetches available versions
func Lookup(registries []Registry, image models.Image) ([]models.RegistryVersion, error) {
    for _, r := range registries {
	if r.Supports(image.Registry) {
	    return r.FetchVersions(image)
	}
    }

    // Fall back to generic
    generic := &GenericRegistry{}
    return generic.FetchVersions(image)
}

// DefaultRegistries returns the standard set of registry handlers
func DefaultRegistries() []Registry {
    return []Registry{
	&DockerHubRegistry{},
	&GHCRRegistry{},
	&GenericRegistry{},
    }
}