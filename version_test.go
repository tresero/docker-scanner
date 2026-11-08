package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildScanner compiles the real binary, so the test covers the flag, the
// release stamp and the exit before scanning together.
func buildScanner(t *testing.T, ldflags string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "docker-scanner")
	args := []string{"build", "-o", bin}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	out, err := exec.Command("go", append(args, ".")...).CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

func TestVersionFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}

	stamped := buildScanner(t, "-X main.version=v9.9.9-test")
	unstamped := buildScanner(t, "")

	tests := []struct {
		name string
		bin  string
		args []string
		want string
	}{
		{name: "a release build prints its version", bin: stamped, args: []string{"-version"}, want: "docker-scanner v9.9.9-test\n"},
		{name: "a build with no version says dev", bin: unstamped, args: []string{"-version"}, want: "docker-scanner dev\n"},
		{name: "it exits before scanning", bin: stamped, args: []string{"-version", "-dir", "/no/such/dir"}, want: "docker-scanner v9.9.9-test\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(tt.bin, tt.args...)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("exit: %v, stderr: %s", err, stderr.String())
			}
			if string(out) != tt.want {
				t.Errorf("stdout = %q, want %q", out, tt.want)
			}
			if stderr.Len() > 0 {
				t.Errorf("stderr = %q, want nothing", stderr.String())
			}
		})
	}
}
