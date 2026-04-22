package scanner

import (
	"docker-scanner/internal/config"
	"docker-scanner/pkg/models"
	"os"
	"path/filepath"
	"strings"
)

// Scan recursively walks rootDir and finds all compose files,
// grouping them by project directory
func Scan(rootDir string) ([]models.Project, error) {
	projectMap := make(map[string]*models.Project)

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}

		if info.IsDir() {
			if isIgnoredDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if !isComposeFile(info.Name()) {
			return nil
		}

		projectDir := filepath.Dir(path)
		projectName := filepath.Base(projectDir)

		if project, exists := projectMap[projectDir]; exists {
			project.ComposeFiles = append(project.ComposeFiles, path)
		} else {
			projectMap[projectDir] = &models.Project{
				Name:         projectName,
				Path:         projectDir,
				ComposeFiles: []string{path},
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	projects := make([]models.Project, 0, len(projectMap))
	for _, p := range projectMap {
		projects = append(projects, *p)
	}

	return projects, nil
}

func isComposeFile(name string) bool {
	lower := strings.ToLower(name)
	for _, valid := range config.ComposeFileNames {
		if lower == valid {
			return true
		}
	}
	return false
}

func isIgnoredDir(name string) bool {
	for _, ignored := range config.IgnoredDirs {
		if name == ignored {
			return true
		}
	}
	return false
}