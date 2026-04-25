package scanner

import (
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