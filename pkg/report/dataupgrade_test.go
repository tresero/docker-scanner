package report

import (
	"docker-scanner/pkg/models"
	"encoding/json"
	"strings"
	"testing"
)

const testDataWarning = "postgres: a new major version won't start on the old data. Back up, then migrate with pg_upgrade or a dump and restore."

func dataUpgradeImage(warning string) models.ImageInfo {
	return models.ImageInfo{
		Image:              models.Image{Registry: "docker.io", Name: "postgres", Tag: "16-alpine", Service: "db", Project: "web"},
		ContainerState:     "running",
		RunningVersion:     "16-alpine",
		RecommendedVersion: "16.14-alpine",
		NewerMajorVersion:  "18.4-alpine",
		DataUpgradeWarning: warning,
	}
}

func TestDataUpgrade_EveryFormat(t *testing.T) {
	results := []models.ImageInfo{dataUpgradeImage(testDataWarning)}
	formats := []struct {
		name     string
		generate func([]models.ImageInfo) string
	}{
		{"text", Generate},
		{"markdown", GenerateMarkdown},
		{"html", GenerateHTML},
	}
	for _, f := range formats {
		t.Run(f.name, func(t *testing.T) {
			out := strings.ReplaceAll(f.generate(results), "&#39;", "'")
			for _, want := range []string{"🗄️", testDataWarning} {
				if !strings.Contains(out, want) {
					t.Errorf("%s report is missing %q", f.name, want)
				}
			}
		})
	}
}

func TestDataUpgrade_AbsentWithNoWarning(t *testing.T) {
	results := []models.ImageInfo{dataUpgradeImage("")}
	for name, out := range map[string]string{"text": Generate(results), "markdown": GenerateMarkdown(results)} {
		if strings.Contains(out, "🗄️") {
			t.Errorf("%s report shows a warning with none set", name)
		}
	}
}

// TestHTMLReport_ExplainsUpgradesToANewReader: the email has to make sense to
// someone who has never run this server.
func TestHTMLReport_ExplainsUpgradesToANewReader(t *testing.T) {
	out := strings.ReplaceAll(GenerateHTML([]models.ImageInfo{dataUpgradeImage("")}), "&#39;", "'")
	legend := extractSection(t, out, "Legend", "Images")
	for _, want := range []string{"🗄️", "Docker only replaces the program", "Back up", "release notes"} {
		if !strings.Contains(legend, want) {
			t.Errorf("legend is missing %q", want)
		}
	}
}

func TestDataUpgrade_DoesntChangeVersionStatus(t *testing.T) {
	if ClassifyImage(dataUpgradeImage(testDataWarning)) != ClassifyImage(dataUpgradeImage("")) {
		t.Error("a data warning changed the version status")
	}
}

func TestDataUpgrade_JSONIsAdditive(t *testing.T) {
	var parsed struct {
		Images []map[string]any `json:"images"`
	}
	results := []models.ImageInfo{dataUpgradeImage(testDataWarning), dataUpgradeImage("")}
	if err := json.Unmarshal([]byte(GenerateJSON(results)), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Images[0]["data_upgrade_warning"] != testDataWarning {
		t.Errorf("data_upgrade_warning = %v", parsed.Images[0]["data_upgrade_warning"])
	}
	if _, ok := parsed.Images[1]["data_upgrade_warning"]; ok {
		t.Error("data_upgrade_warning is present with no warning, want it omitted")
	}
}
