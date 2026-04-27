package parser

import (
	"docker-scanner/pkg/models"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComposeFile represents the structure of a docker-compose.yml
type ComposeFile struct {
	Services map[string]Service `yaml:"services"`
}

// Service represents a single service in a compose file
type Service struct {
	Image       string      `yaml:"image"`
	Environment interface{} `yaml:"environment"`
}

// Parse reads a compose file and extracts image references
func Parse(project models.Project) ([]models.ImageInfo, error) {
	var results []models.ImageInfo

	for _, file := range project.ComposeFiles {
		images, err := parseFile(file, project.Name)
		if err != nil {
			return nil, fmt.Errorf("error parsing %s: %w", file, err)
		}
		results = append(results, images...)
	}

	return results, nil
}

func parseFile(filePath string, projectName string) ([]models.ImageInfo, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var compose ComposeFile
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", filePath, err)
	}

	var results []models.ImageInfo

	for serviceName, service := range compose.Services {
		if service.Image == "" {
			continue
		}

		image := parseImageRef(service.Image)
		image.Service = serviceName
		image.Project = projectName

		info := models.ImageInfo{
			Image:      image,
			File:       filePath,
			UsesLatest: image.Tag == "latest",
		}

		results = append(results, info)
	}

	return results, nil
}

// parseImageRef breaks an image string into registry, name, and tag
func parseImageRef(ref string) models.Image {
	image := models.Image{
		Registry: "docker.io",
		Tag:      "latest",
	}

	// Split tag
	tagSplit := strings.SplitN(ref, ":", 2)
	namePart := tagSplit[0]
	if len(tagSplit) > 1 {
		image.Tag = tagSplit[1]
	}

	// Split registry from name
	parts := strings.Split(namePart, "/")
	if len(parts) >= 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		image.Registry = parts[0]
		image.Name = strings.Join(parts[1:], "/")
	} else {
		image.Name = namePart
		// Docker Hub official images have no slash (e.g., "postgres")
		// Docker Hub user images have one slash (e.g., "user/repo")
	}

	return image
}

// GetRawEnvironment returns the raw environment entries from a compose file
// for use by security checks. Supports both list and map formats.
func GetRawEnvironment(filePath string) (map[string][]string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var compose ComposeFile
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, err
	}

	result := make(map[string][]string)

	for serviceName, service := range compose.Services {
		envVars := extractEnvVars(service.Environment)
		if len(envVars) > 0 {
			result[serviceName] = envVars
		}
	}

	return result, nil
}

// extractEnvVars handles both list and map environment formats
func extractEnvVars(env interface{}) []string {
	if env == nil {
		return nil
	}

	var entries []string

	switch v := env.(type) {
	case []interface{}:
		// List format: - KEY=value or - KEY=${VAR}
		for _, item := range v {
			if s, ok := item.(string); ok {
				entries = append(entries, s)
			}
		}
	case map[string]interface{}:
		// Map format: KEY: value
		for key, val := range v {
			entries = append(entries, fmt.Sprintf("%s=%v", key, val))
		}
	}

	return entries
}

// GetComposeDir returns the directory containing the compose file
func GetComposeDir(filePath string) string {
	return filepath.Dir(filePath)
}

// GetRunningVersion queries Docker for the actual version of a running container
func GetRunningVersion(containerName string) string {
	out, err := exec.Command("docker", "inspect", "--format",
		"{{index .Config.Labels \"org.opencontainers.image.version\"}}", containerName).Output()
	if err != nil {
		return ""
	}

	version := strings.TrimSpace(string(out))
	if version != "" && version != "<no value>" {
		return version
	}

	// Fallback: get the image tag
	out, err = exec.Command("docker", "inspect", "--format", "{{.Config.Image}}", containerName).Output()
	if err != nil {
		return ""
	}

	image := strings.TrimSpace(string(out))
	parts := strings.SplitN(image, ":", 2)
	if len(parts) > 1 && parts[1] != "latest" {
		return parts[1]
	}

	return ""
}