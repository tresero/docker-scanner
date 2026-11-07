package report

import (
	"docker-scanner/pkg/models"
	"encoding/json"
	"strings"
	"testing"
)

func exitCode(i int) *int { return &i }

func containerImage(state, health, running string, exit *int) models.ImageInfo {
	return models.ImageInfo{
		Image:              models.Image{Registry: "docker.io", Name: "app", Tag: "1.2.3", Service: "app", Project: "web"},
		RunningVersion:     running,
		RecommendedVersion: "1.2.3",
		ContainerState:     state,
		Health:             health,
		ExitCode:           exit,
	}
}

func TestContainerStates(t *testing.T) {
	tests := []struct {
		name        string
		info        models.ImageInfo
		wantLabel   string
		wantProblem bool
	}{
		{"running", containerImage("running", "", "1.2.3", nil), "", false},
		{"running and healthy", containerImage("running", "healthy", "1.2.3", nil), "", false},
		{"health check starting", containerImage("running", "starting", "1.2.3", nil), "", false},
		{"unhealthy", containerImage("unhealthy", "unhealthy", "1.2.3", nil), "unhealthy", true},
		{"restarting", containerImage("restarting", "", "1.2.3", nil), "restarting", true},
		{"crashed", containerImage("exited", "", "", exitCode(137)), "exited (137)", true},
		{"stopped cleanly", containerImage("exited", "", "", exitCode(0)), "exited (0)", false},
		{"dead", containerImage("dead", "", "", exitCode(255)), "dead", true},
		{"paused", containerImage("paused", "", "1.2.3", nil), "paused", false},
		{"created", containerImage("created", "", "", nil), "created", false},
		{"never created", containerImage("not created", "", "", nil), "not created", false},
		{"Docker couldn't be asked", containerImage("", "", "", nil), "not running", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runningLabel(tt.info); got != tt.wantLabel {
				t.Errorf("runningLabel = %q, want %q", got, tt.wantLabel)
			}
			if got := containerProblem(tt.info); got != tt.wantProblem {
				t.Errorf("containerProblem = %v, want %v", got, tt.wantProblem)
			}
		})
	}
}

// TestContainerStates_EveryFormat checks what readers actually see.
func TestContainerStates_EveryFormat(t *testing.T) {
	results := []models.ImageInfo{
		containerImage("exited", "", "", exitCode(137)),
		containerImage("unhealthy", "unhealthy", "1.2.3", nil),
		containerImage("not created", "", "", nil),
		containerImage("running", "", "1.2.3", nil),
	}
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
			out := f.generate(results)
			for _, want := range []string{"exited (137)", "unhealthy", "not created", "🚨", "Container problems"} {
				if !strings.Contains(out, want) {
					t.Errorf("%s report is missing %q", f.name, want)
				}
			}
		})
	}
}

func TestContainerProblems_NoSummaryLineWhenThereAreNone(t *testing.T) {
	results := []models.ImageInfo{containerImage("running", "", "1.2.3", nil), containerImage("not created", "", "", nil)}
	for name, out := range map[string]string{"text": Generate(results), "markdown": GenerateMarkdown(results)} {
		if strings.Contains(out, "Container problems") {
			t.Errorf("%s report has a problems line with no problems", name)
		}
	}
}

func TestContainerStates_DontChangeVersionStatus(t *testing.T) {
	info := containerImage("exited", "", "", exitCode(137))
	if got := ClassifyImage(info); got != StatusGood {
		t.Errorf("ClassifyImage = %v, want StatusGood", got)
	}
}

func TestContainerStates_JSONIsAdditive(t *testing.T) {
	results := []models.ImageInfo{
		containerImage("exited", "", "", exitCode(0)),
		containerImage("exited", "", "", exitCode(137)),
		containerImage("unhealthy", "unhealthy", "1.2.3", nil),
		containerImage("", "", "", nil),
	}
	var parsed struct {
		ContainerProblemCount int              `json:"container_problem_count"`
		Images                []map[string]any `json:"images"`
	}
	if err := json.Unmarshal([]byte(GenerateJSON(results)), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.ContainerProblemCount != 2 {
		t.Errorf("container_problem_count = %d, want 2", parsed.ContainerProblemCount)
	}

	clean := parsed.Images[0]
	if clean["container_state"] != "exited" {
		t.Errorf("container_state = %v, want exited", clean["container_state"])
	}
	if code, ok := clean["exit_code"]; !ok || code != float64(0) {
		t.Errorf("exit_code = %v (present %v), want an explicit 0", code, ok)
	}
	if parsed.Images[2]["health"] != "unhealthy" {
		t.Errorf("health = %v, want unhealthy", parsed.Images[2]["health"])
	}
	for _, field := range []string{"container_state", "exit_code", "health"} {
		if _, ok := parsed.Images[3][field]; ok {
			t.Errorf("%s is present with no state known, want it omitted", field)
		}
	}
}
