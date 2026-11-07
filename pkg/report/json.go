package report

import (
	"docker-scanner/pkg/models"
	"encoding/json"
	"time"
)

type jsonReport struct {
	Generated      string             `json:"generated"`
	ProjectCount   int                `json:"project_count"`
	ImageCount     int                `json:"image_count"`
	LatestCount    int                `json:"latest_count"`
	UpgradeCount   int                `json:"upgrade_count"`
	DowngradeCount int                `json:"downgrade_count"`
	SecurityCount  int                `json:"security_count"`
	ContainerProblemCount int         `json:"container_problem_count"`
	NewerBuildCount int               `json:"newer_build_count"`
	Images         []models.ImageInfo `json:"images"`
}

// GenerateJSON produces a JSON report from scan results
func GenerateJSON(results []models.ImageInfo) string {
	securityCount := 0
	projects := make(map[string]bool)
	for _, r := range results {
		projects[r.Image.Project] = true
		securityCount += len(r.SecurityIssues)
	}

	floating, upgrades, downgrades, _ := countByStatus(results)

	data := jsonReport{
		Generated:      time.Now().Format(time.RFC3339),
		ProjectCount:   len(projects),
		ImageCount:     len(results),
		LatestCount:    floating,
		UpgradeCount:   upgrades,
		DowngradeCount: downgrades,
		SecurityCount:  securityCount,
		ContainerProblemCount: countContainerProblems(results),
		NewerBuildCount: countNewerBuilds(results),
		Images:         results,
	}

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return `{"error":"` + err.Error() + `"}`
	}
	return string(out) + "\n"
}
