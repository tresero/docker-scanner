package parser

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// StateNotCreated means Docker answered and no container exists for the service.
const StateNotCreated = "not created"

// ContainerStatus describes the container behind a compose service. State is
// empty if Docker couldn't be asked.
type ContainerStatus struct {
	Name     string
	State    string
	Health   string
	ExitCode *int
}

// dockerState is the part of docker inspect's .State that matters here.
type dockerState struct {
	Status     string
	Running    bool
	Restarting bool
	ExitCode   int
	Health     *struct{ Status string }
}

// FindContainer finds the container for a compose service. Names are tried from
// most to least specific: container_name, or the name Compose generates, then the
// Compose labels for this service and folder, which also find a project whose
// name isn't its folder's. A bare service name comes last because it could belong
// to another project.
//
// A failed label query means Docker can't be asked, so the state stays unknown
// rather than calling every service never-created.
func FindContainer(containerName, project, service, composeFile string) ContainerStatus {
	first := containerName
	if first == "" {
		first = project + "-" + service + "-1"
	}
	if status, ok := inspectContainer(first); ok {
		return status
	}

	labelled, daemonAnswered := containerByLabels(service, composeFile)
	if labelled != "" {
		if status, ok := inspectContainer(labelled); ok {
			return status
		}
	}

	if containerName == "" {
		if status, ok := inspectContainer(service); ok {
			return status
		}
	}

	if !daemonAnswered {
		return ContainerStatus{}
	}
	return ContainerStatus{State: StateNotCreated}
}

func inspectContainer(name string) (ContainerStatus, bool) {
	out, err := dockerOutput("inspect", "--format", "{{json .State}}", name)
	if err != nil {
		return ContainerStatus{}, false
	}
	var state dockerState
	if err := json.Unmarshal(out, &state); err != nil {
		return ContainerStatus{}, false
	}

	status := ContainerStatus{Name: name, State: state.Status}
	if state.Health != nil {
		status.Health = state.Health.Status
	}
	switch {
	case state.Restarting:
		status.State = "restarting"
	case state.Running && status.Health == "unhealthy" && state.Status == "running":
		status.State = "unhealthy"
	case state.Status == "exited" || state.Status == "dead":
		exitCode := state.ExitCode
		status.ExitCode = &exitCode
	}
	return status, true
}

// containerByLabels returns the first container Compose labelled with this service
// and folder, and whether Docker answered at all.
func containerByLabels(service, composeFile string) (string, bool) {
	dir := filepath.Dir(composeFile)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	out, err := dockerOutput("ps", "-a", "-q",
		"--filter", "label=com.docker.compose.service="+service,
		"--filter", "label=com.docker.compose.project.working_dir="+dir)
	if err != nil {
		return "", false
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return "", true
	}
	return ids[0], true
}
