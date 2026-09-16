package resetgen

import (
	"strings"
	"testing"
)

func TestGeneratorInactiveProductionNames(t *testing.T) {
	for _, tc := range []struct {
		name        string
		declaration string
		dependency  string
		alternate   string
	}{
		{name: "function", declaration: "func resetZero() {}"},
		{name: "type", declaration: "type resetZero struct{}"},
		{name: "variable", declaration: "var resetZero int"},
		{name: "constant", declaration: "const resetZero = 42"},
		{name: "aliased import", declaration: "import resetZero \"strings\"\nvar _ = resetZero.TrimSpace"},
		{
			name:        "default import name differs from directory",
			declaration: "import \"example.com/reset-regression/different\"\nvar _ = resetZero.Value",
			dependency:  "package resetZero\nconst Value = 42\n",
		},
		{
			name:        "default import with excluded dependency",
			declaration: "import \"example.com/reset-regression/different\"\nvar _ = resetZero.Value",
			dependency:  "//go:build custom\n\npackage resetZero\nconst Value = 42\n",
		},
		{
			name:        "default import name changes with build tags",
			declaration: "import \"example.com/reset-regression/different\"\nvar _ = resetZero.Value",
			dependency:  "//go:build custom\n\npackage resetZero\nconst Value = 42\n",
			alternate:   "//go:build !custom\n\npackage ordinary\nconst Value = 42\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{
				"source.go":      "package fixture\n// generate:reset\ntype Box struct { Array [1]int }\n",
				"conditional.go": "//go:build custom\n\npackage fixture\n" + tc.declaration + "\nvar resetZero2 int\n",
				"source_test.go": `package fixture
import "testing"
func TestReset(t *testing.T) {
	box := Box{Array: [1]int{42}}
	box.Reset()
	if box.Array != [1]int{} { t.Fatalf("массив не обнулён: %#v", box) }
}
`,
			}
			if tc.dependency != "" {
				files["different/source.go"] = tc.dependency
			}
			if tc.alternate != "" {
				files["different/ordinary.go"] = tc.alternate
				files["active.go"] = "package fixture\nimport active \"example.com/reset-regression/different\"\nvar _ = active.Value\n"
			}
			dir := fixtureModule(t, files)
			runFixtureGenerator(t, dir)
			testFixtureModule(t, dir)
			testFixtureModule(t, dir, "-tags=custom")
			assertFixtureGeneratorIdempotent(t, dir)
		})
	}
}

func TestGeneratorTestBuiltinConflicts(t *testing.T) {
	binary := resetTestBinary
	for _, tc := range []struct {
		name        string
		declaration string
		field       string
		want        []string
	}{
		{"clear map", "func clear(map[string]int) {}", "Value map[string]int", []string{"Box.Value", "clear", "затен"}},
		{"clear pointer to map", "var clear = 42", "Value *map[string]int", []string{"Box.Value", "clear", "затен"}},
		{"nil empty struct", "var nil = 42", "", []string{"Box", "nil", "затен"}},
		{"false bool", "const false = true", "Value bool", []string{"Box.Value", "false", "затен"}},
		{"false named bool", "const false = true", "Value Flag", []string{"Box.Value", "false", "затен"}},
		{"false pointer to bool", "const false = true", "Value *bool", []string{"Box.Value", "false", "затен"}},
	} {
		for _, variant := range []struct {
			name   string
			path   string
			header string
		}{
			{"test", "source_test.go", ""},
			{"inactive test", "source_test.go", "//go:build custom\n\n"},
			{"inactive production", "conditional.go", "//go:build custom\n\n"},
		} {
			t.Run(tc.name+"/"+variant.name, func(t *testing.T) {
				dir := fixtureModule(t, map[string]string{
					"source.go":           "package fixture\ntype Flag bool\n// generate:reset\ntype Box struct { " + tc.field + " }\n",
					variant.path:          variant.header + "package fixture\n" + tc.declaration + "\n",
					genFile:               genHeader + "\n\npackage fixture\nconst Original = 42\n",
					"new/source.go":       "package fresh\n// generate:reset\ntype Fresh struct { Value int }\n",
					"obsolete/source.go":  "package obsolete\n",
					"obsolete/" + genFile: genHeader + "\n\npackage obsolete\nconst Original = 42\n",
				})
				assertGeneratorConflict(t, binary, dir, tc.want)
			})
		}
	}
}

func TestGeneratorTestMethodConflicts(t *testing.T) {
	binary := resetTestBinary
	for _, tc := range []struct {
		name        string
		declaration string
		method      string
	}{
		{"value receiver", "func (Box) Reset() {}", "Reset"},
		{"pointer receiver", "func (*Box) Reset() {}", "Reset"},
		{"Reset with parameters", "func (*Box) Reset(int) {}", "Reset"},
		{"service method with result", "func (*Box) ResetWithVisited() int { return 1 }", "ResetWithVisited"},
		{"alias chain", "type First = Box\ntype Second = (First)\nfunc (*(Second)) Reset() {}", "Reset"},
		{"pointer alias", "type Pointer = *Box\nfunc (Pointer) Reset() {}", "Reset"},
		{"generic receiver", "func (*Generic[A, B]) Reset() {}", "Reset"},
	} {
		for _, header := range []string{"", "//go:build custom\n\n"} {
			name := tc.name
			if header != "" {
				name += "/inactive"
			}
			t.Run(name, func(t *testing.T) {
				dir := fixtureModule(t, map[string]string{
					"source.go":           "package fixture\n// generate:reset\ntype Box struct {}\n// generate:reset\ntype Generic[A, B any] struct { First A; Second B }\n",
					"source_test.go":      header + "package fixture\n" + tc.declaration + "\n",
					genFile:               genHeader + "\n\npackage fixture\nconst Original = 42\n",
					"new/source.go":       "package fresh\n// generate:reset\ntype Fresh struct { Value int }\n",
					"obsolete/source.go":  "package obsolete\n",
					"obsolete/" + genFile: genHeader + "\n\npackage obsolete\nconst Original = 42\n",
				})
				receiver := "Box"
				if tc.name == "generic receiver" {
					receiver = "Generic"
				}
				assertGeneratorConflict(t, binary, dir, []string{receiver + "." + tc.method, "source_test.go", "метод уже объявлен"})
			})
		}
	}
}

func TestGeneratorTestBuiltinImports(t *testing.T) {
	dir := fixtureModule(t, map[string]string{
		"source.go": "package fixture\n// generate:reset\ntype Box struct { Value bool; Map map[string]int }\n",
		"source_test.go": `package fixture
import (
	clear "strings"
	nil "fmt"
	false "reflect"
	"testing"
)
func TestReset(t *testing.T) {
	box := Box{Value: true, Map: map[string]int{"value": 42}}
	box.Reset()
	if box.Value || !false.DeepEqual(box.Map, map[string]int{}) {
		t.Fatal(clear.TrimSpace(nil.Sprintf("поля не сброшены: %#v", box)))
	}
}
`,
		"external_test.go": "package fixture_test\nvar nil = 42\nconst false = true\nfunc clear(map[string]int) {}\ntype Box struct{}\nfunc (*Box) Reset() {}\n",
	})
	runFixtureGenerator(t, dir)
	testFixtureModule(t, dir)
}

func TestGeneratorUnusedTestBuiltins(t *testing.T) {
	dir := fixtureModule(t, map[string]string{
		"source.go": "package fixture\n// generate:reset\ntype Box struct { Value int; Array [1]bool }\n",
		"source_test.go": `package fixture
import "testing"
const false = true
func clear(map[string]int) { panic("тестовый clear не должен вызываться") }
func TestReset(t *testing.T) {
	box := Box{Value: 42, Array: [1]bool{true}}
	box.Reset()
	if box.Value != 0 || box.Array[0] { t.Fatalf("поля не сброшены: %#v", box) }
}
`,
	})
	runFixtureGenerator(t, dir)
	testFixtureModule(t, dir)
}

func TestGeneratorInactiveCgoNames(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	dir := fixtureModule(t, map[string]string{
		"source.go": "package fixture\n// generate:reset\ntype Box struct { Value int }\n",
		"native.go": "package fixture\nimport \"C\"\n",
		"source_test.go": `package fixture
import "testing"
func TestReset(t *testing.T) {
	box := Box{Value: 42}
	box.Reset()
	if box.Value != 0 { t.Fatalf("поле не обнулено: %#v", box) }
}
`,
	})
	runFixtureGenerator(t, dir)
	testFixtureModule(t, dir)
}

func assertGeneratorConflict(t *testing.T, binary, dir string, want []string) {
	t.Helper()
	testFixtureModule(t, dir)
	testFixtureModule(t, dir, "-tags=custom")
	before := snapshotFixtureFiles(t, dir)
	output, err, _ := executeFixtureCommand(fixtureCommand{dir: dir, name: binary})
	if err == nil {
		t.Errorf("генератор не обнаружил конфликт:\n%s", output)
	} else {
		for _, fragment := range want {
			if !strings.Contains(string(output), fragment) {
				t.Errorf("ошибка не содержит %q:\n%s", fragment, output)
			}
		}
	}
	assertFixtureFilesUnchanged(t, dir, before)
}
