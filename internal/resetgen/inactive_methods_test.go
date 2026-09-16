package resetgen

import (
	"maps"
	"runtime"
	"strings"
	"testing"
)

func TestGeneratorInactiveResetMethods(t *testing.T) {
	binary := resetTestBinary
	otherOS := "linux"
	if runtime.GOOS == otherOS {
		otherOS = "darwin"
	}
	otherArch := "amd64"
	if runtime.GOARCH == otherArch {
		otherArch = "arm64"
	}
	const source = "package fixture\ntype Counter int\n// generate:reset\ntype Box struct { Count Counter }\n"
	const conditional = "//go:build custom\n\npackage fixture\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name:  "value receiver",
			files: map[string]string{"method.go": conditional + "func (Counter) Reset() {}\n"},
			want:  []string{"Box.Count", "method.go", "ограничения сборки"},
		},
		{
			name:  "pointer receiver",
			files: map[string]string{"method.go": conditional + "func (*Counter) Reset() {}\n"},
			want:  []string{"Box.Count", "method.go", "ограничения сборки"},
		},
		{
			name:  "alternate platform",
			files: map[string]string{"method_" + otherOS + ".go": "package fixture\nfunc (*Counter) Reset() {}\n"},
			want:  []string{"Box.Count", "method_" + otherOS + ".go", "ограничения сборки"},
		},
		{
			name:  "alternate architecture",
			files: map[string]string{"method_" + otherArch + ".go": "package fixture\nfunc (*Counter) Reset() {}\n"},
			want:  []string{"Box.Count", "method_" + otherArch + ".go", "ограничения сборки"},
		},
		{
			name: "generic receiver",
			files: map[string]string{
				"source.go": "package fixture\ntype Counter[A, B any] struct { First A; Second B }\n// generate:reset\ntype Box struct { Count *Counter[int, string] }\n",
				"method.go": conditional + "func (*Counter[A, B]) Reset() {}\n",
			},
			want: []string{"Box.Count", "method.go", "ограничения сборки"},
		},
		{
			name: "local receiver aliases",
			files: map[string]string{
				"aliases.go": "package fixture\ntype First = Counter\ntype Second = (First)\n",
				"method.go":  conditional + "func (*(Second)) Reset() {}\n",
			},
			want: []string{"Box.Count", "method.go", "ограничения сборки"},
		},
		{
			name: "pointer receiver alias",
			files: map[string]string{
				"method.go": conditional + "type Pointer = *Counter\nfunc (Pointer) Reset() {}\n",
			},
			want: []string{"Box.Count", "method.go", "ограничения сборки"},
		},
		{
			name: "external generated method",
			files: map[string]string{
				"go.mod":              "module example.com/reset-regression\n\ngo 1.25.8\n\nrequire example.com/reset-external v0.0.0\nreplace example.com/reset-external => ./external\n",
				"source.go":           "package fixture\nimport \"example.com/reset-external\"\n// generate:reset\ntype Box struct { Count *external.Counter }\n",
				"external/go.mod":     externalModuleFile,
				"external/source.go":  "package external\ntype Counter int\n",
				"external/" + genFile: genHeader + "\n//go:build custom\n\npackage external\nfunc (*Counter) Reset() {}\n",
			},
			want: []string{"Box.Count", "external", genFile, "ограничения сборки"},
		},
		{
			name:  "marked Reset with parameters",
			files: map[string]string{"method.go": conditional + "func (*Box) Reset(int) {}\n"},
			want:  []string{"Box.Reset", "method.go", "метод уже объявлен"},
		},
		{
			name:  "marked service method with result",
			files: map[string]string{"method.go": conditional + "func (*Box) ResetWithVisited() int { return 1 }\n"},
			want:  []string{"Box.ResetWithVisited", "method.go", "метод уже объявлен"},
		},
		{
			name:  "marked alias receiver",
			files: map[string]string{"method.go": conditional + "type Alias = Box\nfunc (*Alias) Reset() {}\n"},
			want:  []string{"Box.Reset", "method.go", "метод уже объявлен"},
		},
	} {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			files := map[string]string{
				"source.go":           source,
				genFile:               genHeader + "\n\npackage fixture\nconst Original = 42\n",
				"existing/source.go":  "package existing\n// generate:reset\ntype Existing struct { Value int }\n",
				"existing/" + genFile: genHeader + "\n\npackage existing\nfunc (e *Existing) Reset() { e.Value = 42 }\n",
				"obsolete/source.go":  "package obsolete\n",
				"obsolete/" + genFile: genHeader + "\n\npackage obsolete\nconst Original = 42\n",
				"new/source.go":       "package fresh\n// generate:reset\ntype Fresh struct { Value int }\n",
			}
			maps.Copy(files, tc.files)
			dir := fixtureModule(t, files)
			testFixtureModule(t, dir)
			testFixtureModule(t, dir, "-tags=custom")
			requireFixtureCommand(t, fixtureCommand{
				dir:  dir,
				name: "env",
				args: []string{"GOOS=" + otherOS, "GOARCH=" + otherArch, "CGO_ENABLED=0", "go", "build", "./..."},
			})
			before := snapshotFixtureFiles(t, dir)
			output, err, _ := executeFixtureCommand(fixtureCommand{dir: dir, name: binary})
			if err == nil {
				t.Errorf("генератор не обнаружил условный метод:\n%s", output)
			} else {
				for _, want := range tc.want {
					if !strings.Contains(string(output), want) {
						t.Errorf("ошибка не содержит %q:\n%s", want, output)
					}
				}
			}
			assertFixtureFilesUnchanged(t, dir, before)
		})
	}

	t.Run("accept unrelated methods and test receivers", func(t *testing.T) {
		dir := fixtureModule(t, map[string]string{
			"source.go": source,
			"method.go": conditional + "type Other int\nfunc (*Other) Reset() {}\nfunc (*Counter) Reset(int) {}\n",
			"source_test.go": `package fixture
import "testing"
type Alias = Counter
func (*Alias) Reset() { panic("тестовый Reset не должен вызываться") }
func TestReset(t *testing.T) {
	box := Box{Count: 42}
	box.Reset()
	if box.Count != 0 { t.Fatalf("поле не обнулено: %#v", box) }
}
`,
		})
		runFixtureGenerator(t, dir)
		testFixtureModule(t, dir)
		requireFixtureCommand(t, fixtureCommand{dir: dir, name: "go", args: []string{"build", "-tags=custom", "./..."}})
		assertFixtureGeneratorIdempotent(t, dir)
	})
}
