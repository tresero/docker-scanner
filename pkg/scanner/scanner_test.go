package scanner

import (
    "fmt"
    "os"
    "path/filepath"
    "testing"
)

func createProject(t *testing.T, root, name, filename string) {
    t.Helper()
    dir := filepath.Join(root, name)
    if err := os.MkdirAll(dir, 0755); err != nil {
	t.Fatal(err)
    }
    content := "services:\n  app:\n    image: nginx:latest\n"
    if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
	t.Fatal(err)
    }
}

func TestScan_FindsComposeFiles(t *testing.T) {
    root := t.TempDir()
    createProject(t, root, "project1", "docker-compose.yml")
    createProject(t, root, "project2", "compose.yml")

    projects, err := Scan(root)
    if err != nil {
	t.Fatal(err)
    }

    if len(projects) != 2 {
	t.Errorf("expected 2 projects, got %d", len(projects))
    }
}

func TestScan_FindsYAMLVariants(t *testing.T) {
    root := t.TempDir()
    createProject(t, root, "p1", "docker-compose.yml")
    createProject(t, root, "p2", "docker-compose.yaml")
    createProject(t, root, "p3", "compose.yml")
    createProject(t, root, "p4", "compose.yaml")

    projects, err := Scan(root)
    if err != nil {
	t.Fatal(err)
    }

    if len(projects) != 4 {
	t.Errorf("expected 4 projects, got %d", len(projects))
    }
}

func TestScan_SkipsNodeModules(t *testing.T) {
    root := t.TempDir()
    createProject(t, root, "myapp", "docker-compose.yml")
    createProject(t, root, "myapp/node_modules/knex/scripts", "docker-compose.yml")

    projects, err := Scan(root)
    if err != nil {
	t.Fatal(err)
    }

    if len(projects) != 1 {
	t.Errorf("expected 1 project (skip node_modules), got %d", len(projects))
    }
}

func TestScan_SkipsGitDir(t *testing.T) {
    root := t.TempDir()
    createProject(t, root, "myapp", "docker-compose.yml")
    createProject(t, root, "myapp/.git/hooks", "docker-compose.yml")

    projects, err := Scan(root)
    if err != nil {
	t.Fatal(err)
    }

    if len(projects) != 1 {
	t.Errorf("expected 1 project (skip .git), got %d", len(projects))
    }
}

func TestScan_SkipsVendor(t *testing.T) {
    root := t.TempDir()
    createProject(t, root, "myapp", "compose.yml")
    createProject(t, root, "myapp/vendor/somelib", "docker-compose.yml")

    projects, err := Scan(root)
    if err != nil {
	t.Fatal(err)
    }

    if len(projects) != 1 {
	t.Errorf("expected 1 project (skip vendor), got %d", len(projects))
    }
}

func TestScan_EmptyDirectory(t *testing.T) {
    root := t.TempDir()

    projects, err := Scan(root)
    if err != nil {
	t.Fatal(err)
    }

    if len(projects) != 0 {
	t.Errorf("expected 0 projects, got %d", len(projects))
    }
}

func TestScan_NestedProjects(t *testing.T) {
    root := t.TempDir()
    createProject(t, root, "apps/frontend", "docker-compose.yml")
    createProject(t, root, "apps/backend", "compose.yml")

    projects, err := Scan(root)
    if err != nil {
	t.Fatal(err)
    }

    if len(projects) != 2 {
	t.Errorf("expected 2 nested projects, got %d", len(projects))
    }
}

func TestScan_IgnoresNonComposeFiles(t *testing.T) {
    root := t.TempDir()
    dir := filepath.Join(root, "myapp")
    if err := os.MkdirAll(dir, 0755); err != nil {
	t.Fatal(err)
    }
    if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("key: value"), 0644); err != nil {
	t.Fatal(err)
    }
    if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  app:\n    image: nginx\n"), 0644); err != nil {
	t.Fatal(err)
    }

    projects, err := Scan(root)
    if err != nil {
	t.Fatal(err)
    }

    if len(projects) != 1 {
	t.Errorf("expected 1 project, got %d", len(projects))
    }

    if len(projects[0].ComposeFiles) != 1 {
	t.Errorf("expected 1 compose file, got %d", len(projects[0].ComposeFiles))
    }
}

func TestScan_Dockerfiles(t *testing.T) {
	write := func(t *testing.T, root string, files ...string) {
		t.Helper()
		for _, file := range files {
			path := filepath.Join(root, file)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("FROM alpine:3.20\n"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	tests := []struct {
		name            string
		files           []string
		wantDockerfiles map[string]int
		wantCompose     map[string]int
	}{
		{name: "a Dockerfile with no compose file is a project", files: []string{"tool/Dockerfile"}, wantDockerfiles: map[string]int{"tool": 1}},
		{name: "a Containerfile counts", files: []string{"tool/Containerfile"}, wantDockerfiles: map[string]int{"tool": 1}},
		{name: "a compose folder's Dockerfile is left to compose", files: []string{"app/Dockerfile", "app/compose.yml"}, wantCompose: map[string]int{"app": 1}},
		{name: "other files aren't Dockerfiles", files: []string{"tool/Dockerfile.md", "tool/notes.txt"}},
		{name: "ignored folders stay ignored", files: []string{"node_modules/x/Dockerfile"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, tt.files...)
			projects, err := Scan(root)
			if err != nil {
				t.Fatal(err)
			}
			gotDockerfiles, gotCompose := map[string]int{}, map[string]int{}
			for _, p := range projects {
				if len(p.Dockerfiles) > 0 {
					gotDockerfiles[p.Name] = len(p.Dockerfiles)
				}
				if len(p.ComposeFiles) > 0 {
					gotCompose[p.Name] = len(p.ComposeFiles)
				}
			}
			if fmt.Sprint(gotDockerfiles) != fmt.Sprint(mapOrEmpty(tt.wantDockerfiles)) {
				t.Errorf("Dockerfiles = %v, want %v", gotDockerfiles, tt.wantDockerfiles)
			}
			if fmt.Sprint(gotCompose) != fmt.Sprint(mapOrEmpty(tt.wantCompose)) {
				t.Errorf("ComposeFiles = %v, want %v", gotCompose, tt.wantCompose)
			}
		})
	}
}

func mapOrEmpty(m map[string]int) map[string]int {
	if m == nil {
		return map[string]int{}
	}
	return m
}
