package parser

import (
	"docker-scanner/pkg/models"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type ComposeFile struct {
	Services map[string]Service `yaml:"services"`
}

type Service struct {
	Image         string      `yaml:"image"`
	ContainerName string      `yaml:"container_name"`
	Environment   interface{} `yaml:"environment"`
}

// versionPattern matches tags that look like a pinned version.
// Allows: 1, v1.2.3, 1.2.3-rc1, 1.2.3+build.5, 16-alpine, 2024.05.01, a1b2c3d
// Rejects: latest, main, apache, bookworm, alpine, fpm
var versionPattern = regexp.MustCompile(`^(?:v?\d+(?:[.\-+]\w+)*|[0-9a-f]{7,})$`)

func looksLikeVersion(tag string) bool {
	if tag == "" {
		return false
	}
	return versionPattern.MatchString(tag)
}

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
		image.ContainerName = service.ContainerName

		info := models.ImageInfo{
			Image:      image,
			File:       filePath,
			UsesLatest: !looksLikeVersion(image.Tag),
		}

		results = append(results, info)
	}

	return results, nil
}

func parseImageRef(ref string) models.Image {
	image := models.Image{
		Registry: "docker.io",
		Tag:      "latest",
	}

	tagSplit := strings.SplitN(ref, ":", 2)
	namePart := tagSplit[0]
	if len(tagSplit) > 1 {
		image.Tag = tagSplit[1]
	}

	parts := strings.Split(namePart, "/")
	if len(parts) >= 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		image.Registry = parts[0]
		image.Name = strings.Join(parts[1:], "/")
	} else {
		image.Name = namePart
	}

	return image
}

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

func extractEnvVars(env interface{}) []string {
	if env == nil {
		return nil
	}

	var entries []string

	switch v := env.(type) {
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				entries = append(entries, s)
			}
		}
	case map[string]interface{}:
		for key, val := range v {
			entries = append(entries, fmt.Sprintf("%s=%v", key, val))
		}
	}

	return entries
}

func GetComposeDir(filePath string) string {
	return filepath.Dir(filePath)
}

func GetRunningVersion(containerName string) string {
	out, err := exec.Command("docker", "inspect", "--format",
		"{{.State.Running}}", containerName).Output()
	if err != nil {
		return ""
	}

	running := strings.TrimSpace(string(out))
	if running != "true" {
		return ""
	}

	out, err = exec.Command("docker", "inspect", "--format",
		"{{.Config.Image}}", containerName).Output()
	if err != nil {
		return "unknown"
	}

	image := strings.TrimSpace(string(out))

	if strings.HasPrefix(image, "sha256:") {
		out, err = exec.Command("docker", "inspect", "--format",
			"{{index .Image}}", containerName).Output()
		if err != nil {
			return "unknown"
		}
		imageID := strings.TrimSpace(string(out))

		out, err = exec.Command("docker", "image", "inspect", "--format",
			"{{index .RepoTags 0}}", imageID).Output()
		if err != nil {
			return "unknown"
		}
		image = strings.TrimSpace(string(out))
	}

	parts := strings.SplitN(image, ":", 2)
	if len(parts) > 1 && parts[1] != "latest" {
		return parts[1]
	}

	out, err = exec.Command("docker", "inspect", "--format",
		"{{index .Config.Labels \"org.opencontainers.image.version\"}}", containerName).Output()
	if err != nil {
		return "unknown"
	}

	version := strings.TrimSpace(string(out))
	if version == "" || version == "<no value>" {
		return "unknown"
	}

	refName, _ := exec.Command("docker", "inspect", "--format",
		"{{index .Config.Labels \"org.opencontainers.image.ref.name\"}}", containerName).Output()
	ref := strings.TrimSpace(string(refName))
	if ref == "ubuntu" || ref == "debian" || ref == "alpine" {
		return "unknown"
	}

	return version
}