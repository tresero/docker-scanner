package parser

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const (
	stateRunning    = `{"Status":"running","Running":true,"ExitCode":0}`
	stateHealthy    = `{"Status":"running","Running":true,"ExitCode":0,"Health":{"Status":"healthy"}}`
	stateStarting   = `{"Status":"running","Running":true,"ExitCode":0,"Health":{"Status":"starting"}}`
	stateUnhealthy  = `{"Status":"running","Running":true,"ExitCode":0,"Health":{"Status":"unhealthy"}}`
	stateRestarting = `{"Status":"restarting","Running":true,"Restarting":true,"ExitCode":1}`
	stateExited137  = `{"Status":"exited","Running":false,"ExitCode":137}`
	stateExited0    = `{"Status":"exited","Running":false,"ExitCode":0}`
	stateCreated    = `{"Status":"created","Running":false,"ExitCode":0}`
	statePaused     = `{"Status":"paused","Running":true,"Paused":true,"ExitCode":0}`
	stateDead       = `{"Status":"dead","Running":false,"Dead":true,"ExitCode":255}`
)

// unrelatedID and unrelatedName are a container from another project that the
// fake always has. Like Docker, the fake's inspect also answers a prefix of its ID.
const (
	unrelatedID   = "a1b2c3d4e5f6"
	unrelatedName = "chhoto"
)

// fakeContainers answers inspect by container name and ps with labelled, the
// container IDs matching the compose labels. noDaemon makes every command fail.
func fakeContainers(t *testing.T, states map[string]string, labelled string, noDaemon bool) *[]string {
	t.Helper()
	original := dockerOutput
	t.Cleanup(func() { dockerOutput = original })
	var calls []string
	dockerOutput = func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		if noDaemon {
			return nil, errors.New("Cannot connect to the Docker daemon")
		}
		switch args[0] {
		case "inspect":
			name := args[len(args)-1]
			state, ok := states[name]
			if !ok && strings.HasPrefix(unrelatedID, name) {
				name, state, ok = unrelatedName, stateHealthy, true
			}
			if !ok {
				return nil, errors.New("Error: No such object")
			}
			if strings.Contains(strings.Join(args, " "), "{{.Name}}") {
				state = "/" + name + " " + state
			}
			return []byte(state + "\n"), nil
		case "ps":
			return []byte(labelled), nil
		}
		return nil, errors.New("unexpected command")
	}
	return &calls
}

func intPtr(i int) *int { return &i }

func TestFindContainer(t *testing.T) {
	const composeFile = "/srv/stacks/web/docker-compose.yml"
	tests := []struct {
		name          string
		containerName string
		states        map[string]string
		labelled      string
		noDaemon      bool
		wantName      string
		wantState     string
		wantHealth    string
		wantExit      *int
	}{
		{name: "running with no health check", states: map[string]string{"web-app-1": stateRunning}, wantName: "web-app-1", wantState: "running"},
		{name: "running and healthy", states: map[string]string{"web-app-1": stateHealthy}, wantName: "web-app-1", wantState: "running", wantHealth: "healthy"},
		{name: "health check still starting counts as running", states: map[string]string{"web-app-1": stateStarting}, wantName: "web-app-1", wantState: "running", wantHealth: "starting"},
		{name: "up but failing its health check", states: map[string]string{"web-app-1": stateUnhealthy}, wantName: "web-app-1", wantState: "unhealthy", wantHealth: "unhealthy"},
		{name: "in a restart loop", states: map[string]string{"web-app-1": stateRestarting}, wantName: "web-app-1", wantState: "restarting"},
		{name: "crashed keeps its exit code", states: map[string]string{"web-app-1": stateExited137}, wantName: "web-app-1", wantState: "exited", wantExit: intPtr(137)},
		{name: "stopped cleanly keeps exit code 0", states: map[string]string{"web-app-1": stateExited0}, wantName: "web-app-1", wantState: "exited", wantExit: intPtr(0)},
		{name: "created but never started", states: map[string]string{"web-app-1": stateCreated}, wantName: "web-app-1", wantState: "created"},
		{name: "paused", states: map[string]string{"web-app-1": statePaused}, wantName: "web-app-1", wantState: "paused"},
		{name: "dead keeps its exit code", states: map[string]string{"web-app-1": stateDead}, wantName: "web-app-1", wantState: "dead", wantExit: intPtr(255)},

		{name: "container_name is used first", containerName: "salsa", states: map[string]string{"salsa": stateRunning, "web-app-1": stateExited137}, wantName: "salsa", wantState: "running"},
		{name: "the project's container wins over one that only shares the service name", states: map[string]string{"web-app-1": stateRunning, "app": stateExited137}, wantName: "web-app-1", wantState: "running"},
		{name: "compose labels find a differently named project", states: map[string]string{"abc123": stateRunning}, labelled: "abc123\n", wantName: "abc123", wantState: "running"},
		{name: "the first of several replicas is used", states: map[string]string{"abc123": stateRunning}, labelled: "abc123\ndef456\n", wantName: "abc123", wantState: "running"},
		{name: "a bare service name is the last resort", states: map[string]string{"app": stateRunning}, wantName: "app", wantState: "running"},

		{name: "no container exists", wantState: StateNotCreated},
		{name: "a missing container_name with nothing labelled", containerName: "salsa", wantState: StateNotCreated},
		{name: "Docker unreachable is unknown, not never-created", noDaemon: true, wantState: ""},
		{name: "an unreadable state falls through", states: map[string]string{"web-app-1": "not json"}, wantState: StateNotCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeContainers(t, tt.states, tt.labelled, tt.noDaemon)
			got := FindContainer(tt.containerName, "web", "app", composeFile)

			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.State != tt.wantState {
				t.Errorf("State = %q, want %q", got.State, tt.wantState)
			}
			if got.Health != tt.wantHealth {
				t.Errorf("Health = %q, want %q", got.Health, tt.wantHealth)
			}
			if fmt.Sprint(deref(got.ExitCode)) != fmt.Sprint(deref(tt.wantExit)) {
				t.Errorf("ExitCode = %v, want %v", deref(got.ExitCode), deref(tt.wantExit))
			}
		})
	}
}

func deref(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

// TestFindContainer_NameMatchesOnlyThatName comes from a real scan: service a
// had no container, and docker inspect a found chhoto, whose ID starts with a.
func TestFindContainer_NameMatchesOnlyThatName(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		service       string
	}{
		{name: "a bare service name that is a prefix of another container's ID", service: "a"},
		{name: "a container_name that is a prefix of another container's ID", containerName: "a1b2", service: "web"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeContainers(t, nil, "", false)
			got := FindContainer(tt.containerName, "app", tt.service, "/srv/stacks/app/compose.yml")
			if got.Name != "" || got.State != StateNotCreated {
				t.Errorf("got %q in state %q, want no container (%q)", got.Name, got.State, StateNotCreated)
			}
		})
	}
}

func TestFindContainer_LabelLookupUsesTheComposeFolder(t *testing.T) {
	calls := fakeContainers(t, nil, "", false)
	FindContainer("", "web", "app", "/srv/stacks/web/docker-compose.yml")

	joined := strings.Join(*calls, "\n")
	for _, want := range []string{"label=com.docker.compose.service=app", "label=com.docker.compose.project.working_dir=/srv/stacks/web"} {
		if !strings.Contains(joined, want) {
			t.Errorf("docker calls %q, want one to contain %q", joined, want)
		}
	}
}
