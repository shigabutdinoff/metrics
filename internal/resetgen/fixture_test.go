package resetgen

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	fixtureModuleFile             = "module example.com/reset-regression\n\ngo 1.25.8\n"
	fixtureModuleWithExternalFile = fixtureModuleFile + "\nrequire example.com/reset-external v0.0.0\n\nreplace example.com/reset-external => ./external\n"
	externalModuleFile            = "module example.com/reset-external\n\ngo 1.25.8\n"
)

type fixtureCommand struct {
	dir     string
	timeout time.Duration
	env     []string
	name    string
	args    []string
}

func fixtureModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writeFixtureFile(t, dir, "go.mod", fixtureModuleFile)
	for name, content := range files {
		writeFixtureFile(t, dir, name, content)
	}
	return dir
}

func writeFixtureFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFixtureFile(t *testing.T, dir, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func executeFixtureCommand(command fixtureCommand) ([]byte, error, error) {
	ctx := context.Background()
	cancel := func() {}
	if command.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, command.timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(ctx, command.name, command.args...)
	cmd.Dir = command.dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	cmd.Env = append(cmd.Env, command.env...)
	output, err := cmd.CombinedOutput()
	return output, err, ctx.Err()
}

func requireFixtureCommand(t *testing.T, command fixtureCommand) {
	t.Helper()
	output, err, contextErr := executeFixtureCommand(command)
	if err != nil {
		t.Fatalf("%s %s: %v (%v)\n%s", command.name, strings.Join(command.args, " "), err, contextErr, output)
	}
}

func runFixtureGenerator(t *testing.T, dir string, extraEnv ...string) {
	t.Helper()
	requireFixtureCommand(t, fixtureCommand{dir: dir, env: extraEnv, name: resetTestBinary})
}

func testFixtureModule(t *testing.T, dir string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"test"}, args...)
	commandArgs = append(commandArgs, "./...")
	requireFixtureCommand(t, fixtureCommand{dir: dir, name: "go", args: commandArgs})
}

func testFixtureModuleRace(t *testing.T, dir string) {
	t.Helper()
	requireFixtureCommand(t, fixtureCommand{
		dir:     dir,
		timeout: time.Minute,
		name:    "go",
		args:    []string{"test", "-race", "-timeout=20s", "./..."},
	})
}

func snapshotFixtureFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		name, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[name] = string(readFixtureFile(t, dir, name))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func assertFixtureFilesUnchanged(t *testing.T, dir string, before map[string]string) {
	t.Helper()
	after := snapshotFixtureFiles(t, dir)
	for name, content := range before {
		if got, ok := after[name]; !ok || got != content {
			t.Errorf("ошибка генерации изменила или удалила %s", name)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			t.Errorf("ошибка генерации создала %s", name)
		}
	}
}

func assertFixtureGeneratorIdempotent(t *testing.T, dir string) {
	t.Helper()
	before := snapshotFixtureFiles(t, dir)
	runFixtureGenerator(t, dir)
	assertFixtureFilesUnchanged(t, dir, before)
}
