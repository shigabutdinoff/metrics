package resetgen

import (
	"bytes"
	"go/build"
	"go/token"
	"go/types"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestConstraintSourcePaths(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "external", "source.go")
	for _, tc := range []struct {
		name     string
		position string
		files    []string
		compiled []string
		want     string
		wantErr  string
	}{
		{name: "absolute", position: source, want: source},
		{name: "GOROOT", position: "$GOROOT/src/time/time.go", want: filepath.Join(build.Default.GOROOT, "src", "time", "time.go")},
		{name: "trimmed", position: "example.com/external/source.go", files: []string{source}, want: source},
		{name: "versioned", position: "example.com/external@v1.2.3/source.go", files: []string{source}, want: source},
		{name: "compiled file", position: "example.com/external/source.go", compiled: []string{source}, want: source},
		{name: "duplicate metadata", position: "example.com/external/source.go", files: []string{source}, compiled: []string{source}, want: source},
		{name: "unknown file", position: "example.com/external/missing.go", files: []string{source}, wantErr: "не найден файл объявления зависимости"},
		{name: "another package only", position: "example.com/external/source.go", wantErr: "не найден файл объявления зависимости"},
		{name: "ambiguous", position: "example.com/external/source.go", files: []string{source, filepath.Join(dir, "generated", "source.go")}, wantErr: "неоднозначный файл объявления зависимости"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file := fset.AddFile(tc.position, -1, 1)
			pkg := types.NewPackage("example.com/external", "external")
			obj := types.NewTypeName(file.Pos(0), pkg, "Item", nil)
			checker := newConstraintChecker(fset, nil, []*packages.Package{
				{PkgPath: pkg.Path(), GoFiles: tc.files, CompiledGoFiles: tc.compiled},
				{PkgPath: "example.com/another", GoFiles: []string{filepath.Join(dir, "another", "source.go")}},
			})
			got, err := checker.sources.path(obj)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), tc.position) {
					t.Fatalf("path() = %q, %v; ожидалась ошибка %q для %q", got, err, tc.wantErr, tc.position)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("path() = %q, %v; ожидался %q", got, err, tc.want)
			}
		})
	}
}

func TestConstraintLoadedPackagePaths(t *testing.T) {
	fset := token.NewFileSet()
	file := fset.AddFile("bytes/buffer.go", -1, 1)
	pkg := types.NewPackage("bytes", "bytes")
	obj := types.NewTypeName(file.Pos(0), pkg, "Buffer", nil)
	checker := newConstraintChecker(fset, nil, nil)
	if _, err := checker.sources.loadPackage(pkg); err != nil {
		t.Fatal(err)
	}
	if err := checker.checkObject(obj); err != nil {
		t.Fatalf("не найден файл загруженного пакета: %v", err)
	}
}

func TestGeneratorTrimpath(t *testing.T) {
	binary := resetTestBinary

	const externalReset = genHeader + "\n\npackage external\n\nfunc (item *Item) Reset() { item.Value = 77; Calls++ }\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		tests string
	}{
		{
			name: "standard library",
			files: map[string]string{
				"source.go": `package fixture
import (
	"bytes"
	"time"
)
// generate:reset
type Box struct { Buffer bytes.Buffer; Time time.Time; Pointer *time.Time }
`,
			},
			tests: `package fixture
import (
	"bytes"
	"testing"
	"time"
)
func TestReset(t *testing.T) {
	pointed := time.Unix(123, 0)
	box := Box{Buffer: *bytes.NewBufferString("buffer"), Time: pointed, Pointer: &pointed}
	box.Reset()
	if box.Buffer.Len() != 0 || !box.Time.IsZero() || !pointed.IsZero() || box.Pointer != &pointed {
		t.Fatalf("поля стандартных типов не сброшены: %#v", box)
	}
}
`,
		},
		{
			name: "external generated Reset and type chain",
			files: map[string]string{
				"go.mod": fixtureModuleWithExternalFile,
				"source.go": `package fixture
import "example.com/reset-external"
// generate:reset
type Box struct { Item external.Item; Pointer *external.Item; Items external.Items }
`,
				"external/go.mod":     externalModuleFile,
				"external/source.go":  "package external\n\ntype Item struct { Value int }\n\ntype Items BaseItems\ntype BaseItems []int\n\nvar Calls int\n",
				"external/" + genFile: externalReset,
			},
			tests: `package fixture
import (
	"example.com/reset-external"
	"testing"
)
func TestReset(t *testing.T) {
	pointed := external.Item{Value: 3}
	box := Box{Item: external.Item{Value: 2}, Pointer: &pointed, Items: make(external.Items, 2, 4)}
	box.Reset()
	if external.Calls != 2 || box.Item.Value != 77 || pointed.Value != 77 || box.Pointer != &pointed || len(box.Items) != 0 || cap(box.Items) != 4 {
		t.Fatalf("методы и типы зависимости обработаны неверно: box=%#v, calls=%d", box, external.Calls)
	}
}
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixtureModule(t, tc.files)
			runFixtureGenerator(t, dir, "GOFLAGS=-trimpath")
			first := readFixtureFile(t, dir, genFile)
			runFixtureGenerator(t, dir, "GOFLAGS=")
			if !bytes.Equal(first, readFixtureFile(t, dir, genFile)) {
				t.Error("trimpath изменил результат генерации")
			}
			writeFixtureFile(t, dir, "source_test.go", tc.tests)
			requireFixtureCommand(t, fixtureCommand{dir: dir, env: []string{"GOFLAGS=-trimpath"}, name: "go", args: []string{"test", "./..."}})
			if want, ok := tc.files["external/"+genFile]; ok {
				if got := string(readFixtureFile(t, dir, "external/"+genFile)); got != want {
					t.Error("изменён generated Reset внешней зависимости")
				}
			}
		})
	}

	for _, tc := range []struct {
		name   string
		source string
		file   string
		tagged string
	}{
		{
			name:   "platform type",
			source: "package external\n",
			file:   "item_" + runtime.GOOS + ".go",
			tagged: "package external\n\ntype Item struct { Value int }\n",
		},
		{
			name:   "tagged Reset method",
			source: "package external\n\ntype Item struct { Value int }\n",
			file:   genFile,
			tagged: "//go:build " + runtime.GOARCH + "\n\n" + externalReset,
		},
	} {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			dir := fixtureModule(t, map[string]string{
				"go.mod":              fixtureModuleWithExternalFile,
				"source.go":           "package fixture\n\nimport \"example.com/reset-external\"\n\n// generate:reset\ntype Box struct { Item external.Item }\n",
				"external/go.mod":     externalModuleFile,
				"external/source.go":  tc.source + "\nvar Calls int\n",
				"external/" + tc.file: tc.tagged,
			})
			before := snapshotFixtureFiles(t, dir)
			command := fixtureCommand{dir: dir, env: []string{"GOFLAGS=-trimpath"}, name: binary}
			output, err, _ := executeFixtureCommand(command)
			if err == nil {
				t.Fatal("генератор не сообщил об ограничении сборки")
			}
			for _, want := range []string{"ограничения сборки", filepath.Join(dir, "external", tc.file)} {
				if !strings.Contains(string(output), want) {
					t.Errorf("ошибка не содержит %q:\n%s", want, output)
				}
			}
			assertFixtureFilesUnchanged(t, dir, before)
		})
	}
}
