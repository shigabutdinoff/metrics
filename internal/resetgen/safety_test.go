package resetgen

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGeneratorSafety(t *testing.T) {
	binary := resetTestBinary

	const marked = "package fixture\n\n// generate:reset\ntype Sample struct { Value int }\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name: "unowned output",
			files: map[string]string{
				"source.go": marked,
				genFile:     "package fixture\n\nfunc answer() int { return 42 }\n",
			},
			want: []string{"принадлежит", genFile},
		},
		{
			name: "marker inside unowned output",
			files: map[string]string{
				genFile: marked,
			},
			want: []string{"принадлежит", genFile},
		},
		{
			name: "go build constraint",
			files: map[string]string{
				"source.go": "//go:build " + runtime.GOOS + "\n\n" + marked,
			},
			want: []string{"ограничения сборки", "source.go"},
		},
		{
			name: "legacy build constraint",
			files: map[string]string{
				"source.go": "// +build " + runtime.GOARCH + "\n\n" + marked,
			},
			want: []string{"ограничения сборки", "source.go"},
		},
		{
			name: "operating system suffix",
			files: map[string]string{
				"model_" + runtime.GOOS + ".go": marked,
			},
			want: []string{"ограничения сборки", "model_" + runtime.GOOS + ".go"},
		},
		{
			name: "architecture suffix",
			files: map[string]string{
				"model_" + runtime.GOARCH + ".go": marked,
			},
			want: []string{"ограничения сборки", "model_" + runtime.GOARCH + ".go"},
		},
		{
			name: "operating system and architecture suffix",
			files: map[string]string{
				"model_" + runtime.GOOS + "_" + runtime.GOARCH + ".go": marked,
			},
			want: []string{"ограничения сборки", "model_" + runtime.GOOS + "_" + runtime.GOARCH + ".go"},
		},
		{
			name: "foreign unexported fields",
			files: map[string]string{
				"source.go": "package fixture\n\nimport \"time\"\n\n// generate:reset\ntype Outer time.Time\n",
			},
			want: []string{"недоступно", "Outer"},
		},
		{
			name: "clear variable",
			files: map[string]string{
				"source.go": "package fixture\n\nvar clear = 123\n\n// generate:reset\ntype Sample struct { Map map[string]int }\n",
			},
			want: []string{"clear", "затен", "Sample.Map"},
		},
		{
			name: "compatible clear function",
			files: map[string]string{
				"source.go": "package fixture\n\nfunc clear(m map[string]int) {}\n\n// generate:reset\ntype Sample struct { Map map[string]int }\n",
			},
			want: []string{"clear", "затен", "Sample.Map"},
		},
		{
			name: "clear with pointer to map",
			files: map[string]string{
				"source.go": "package fixture\n\nconst clear = 123\n\n// generate:reset\ntype Sample struct { Map *map[string]int }\n",
			},
			want: []string{"clear", "затен", "Sample.Map"},
		},
		{
			name: "shadowed nil with empty struct",
			files: map[string]string{
				"source.go": "package fixture\n\nvar nil = 42\n\n// generate:reset\ntype Box struct {}\n",
			},
			want: []string{"nil", "затен", "Box"},
		},
		{
			name: "shadowed false with bool",
			files: map[string]string{
				"source.go": "package fixture\n\nconst false = true\n\n// generate:reset\ntype Box struct { Value bool }\n",
			},
			want: []string{"false", "затен", "Box.Value"},
		},
		{
			name: "shadowed false with named bool",
			files: map[string]string{
				"source.go": "package fixture\n\nconst false = true\ntype Flag bool\n\n// generate:reset\ntype Box struct { Value Flag }\n",
			},
			want: []string{"false", "затен", "Box.Value"},
		},
		{
			name: "shadowed false with pointer to bool",
			files: map[string]string{
				"source.go": "package fixture\n\nconst false = true\n\n// generate:reset\ntype Box struct { Value *bool }\n",
			},
			want: []string{"false", "затен", "Box.Value"},
		},
		{
			name: "Reset field",
			files: map[string]string{
				"source.go": "package fixture\n\n// generate:reset\ntype Box struct { Reset bool }\n",
			},
			want: []string{"Box.Reset"},
		},
		{
			name: "embedded Reset field",
			files: map[string]string{
				"source.go": "package fixture\n\ntype Reset bool\n\n// generate:reset\ntype Box struct { Reset }\n",
			},
			want: []string{"Box.Reset"},
		},
		{
			name: "Reset value receiver",
			files: map[string]string{
				"source.go": "package fixture\n\n// generate:reset\ntype Box struct { Value int }\nfunc (Box) Reset() {}\n",
			},
			want: []string{"Box.Reset"},
		},
		{
			name: "Reset pointer receiver",
			files: map[string]string{
				"source.go": "package fixture\n\n// generate:reset\ntype Box struct { Value int }\nfunc (*Box) Reset() {}\n",
			},
			want: []string{"Box.Reset"},
		},
		{
			name: "Reset with parameters",
			files: map[string]string{
				"source.go": "package fixture\n\n// generate:reset\ntype Box struct { Value int }\nfunc (Box) Reset(int) {}\n",
			},
			want: []string{"Box.Reset"},
		},
		{
			name: "ResetWithVisited field",
			files: map[string]string{
				"source.go": "package fixture\n\n// generate:reset\ntype Box struct { ResetWithVisited int }\n",
			},
			want: []string{"Box.ResetWithVisited"},
		},
		{
			name: "ResetWithVisited method",
			files: map[string]string{
				"source.go": "package fixture\n\n// generate:reset\ntype Box struct { Value int }\nfunc (*Box) ResetWithVisited() {}\n",
			},
			want: []string{"Box.ResetWithVisited"},
		},
		{
			name: "Reset with result",
			files: map[string]string{
				"source.go": "package fixture\n\n// generate:reset\ntype Box struct { Value int }\nfunc (*Box) Reset() int { return 42 }\n",
			},
			want: []string{"Box.Reset"},
		},
	} {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			files := map[string]string{
				"root.go":             "package fixture\n",
				genFile:               genHeader + "\n\npackage fixture\n\nconst original = 42\n",
				"user/" + genFile:     "package user\n\nconst Answer = 42\n",
				"existing/source.go":  "package existing\n\n// generate:reset\ntype Existing struct { Value int }\n",
				"existing/" + genFile: genHeader + "\n\npackage existing\n\nfunc (e *Existing) Reset() { e.Value = 42 }\n",
				"obsolete/source.go":  "package obsolete\n",
				"obsolete/" + genFile: genHeader + "\n\npackage obsolete\n\nconst Original = 42\n",
				"new/source.go":       "package fresh\n\n// generate:reset\ntype Fresh struct { Value int }\n",
			}
			for name, content := range tc.files {
				files[name] = content
			}
			dir := fixtureModule(t, files)
			testFixtureModule(t, dir)
			before := snapshotFixtureFiles(t, dir)

			output, err, _ := executeFixtureCommand(fixtureCommand{dir: dir, name: binary})
			if err == nil {
				t.Errorf("генератор не сообщил об ошибке:\n%s", output)
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

	for _, tc := range []struct {
		name  string
		files map[string]string
		tests string
	}{
		{
			name: "unconstrained filenames",
			files: map[string]string{
				"linux.go":             strings.ReplaceAll(marked, "Sample", "Linux"),
				"model_data.go":        strings.ReplaceAll(marked, "Sample", "Data"),
				"model.extra_linux.go": strings.ReplaceAll(marked, "Sample", "Extra"),
			},
			tests: `package fixture
import "testing"
func TestReset(t *testing.T) {
	l, d, e := Linux{7}, Data{8}, Extra{9}
	l.Reset()
	d.Reset()
	e.Reset()
	if l.Value != 0 || d.Value != 0 || e.Value != 0 { t.Fatal("структуры не сброшены") }
}
`,
		},
		{
			name: "accessible derived fields",
			files: map[string]string{
				"source.go": `package fixture
import "example.com/reset-regression/foreign"
type local struct { hidden int; Items []int; Pointer *int }
// generate:reset
type Local local
// generate:reset
type Public foreign.Public
`,
				"foreign/source.go": "package foreign\n\ntype Public struct { Value int; Items []int; _ int }\n",
			},
			tests: `package fixture
import "testing"
func TestReset(t *testing.T) {
	n := 7
	items := []int{1, 2}
	l := Local{hidden: 8, Items: items, Pointer: &n}
	p := Public{Value: 9, Items: items}
	l.Reset()
	p.Reset()
	if l.hidden != 0 || l.Pointer != &n || n != 0 || p.Value != 0 { t.Fatal("поля не сброшены") }
	if len(l.Items) != 0 || len(p.Items) != 0 || cap(l.Items) != cap(items) || cap(p.Items) != cap(items) {
		t.Fatal("слайсы не очищены с сохранением capacity")
	}
	if &l.Items[:1][0] != &items[0] || &p.Items[:1][0] != &items[0] { t.Fatal("слайсы заменены") }
}
`,
		},
		{
			name: "shadowed clear without builtin use",
			files: map[string]string{
				"source.go": `package fixture
var clear = 123
type Entries map[string]int
func (m Entries) Reset() { for key := range m { delete(m, key) } }
// generate:reset
type Scalar struct { Value int }
// generate:reset
type Box struct { Map Entries; Pointer *Entries }
`,
			},
			tests: `package fixture
import "testing"
func TestReset(t *testing.T) {
	s := Scalar{7}
	entries := Entries{"first": 1}
	pointed := Entries{"second": 2}
	b := Box{Map: entries, Pointer: &pointed}
	s.Reset()
	b.Reset()
	if s.Value != 0 || len(entries) != 0 || len(pointed) != 0 || b.Pointer != &pointed {
		t.Fatal("Reset не сбросил поля через пользовательский метод")
	}
	entries["new"] = 3
	if b.Map["new"] != 3 { t.Fatal("мапа заменена") }
}
`,
		},
		{
			name: "shadowed false without builtin use",
			files: map[string]string{
				"source.go": `package fixture
const false = true
type Flag bool
func (f *Flag) Reset() { *f = 0 != 0 }
// generate:reset
type Box struct {
	Value int
	Array [2]bool
	Nested struct { Value bool }
	Flag Flag
	Pointer *Flag
}
`,
			},
			tests: `package fixture
import "testing"
func TestReset(t *testing.T) {
	flag := Flag(true)
	b := Box{Value: 7, Array: [2]bool{true, true}, Flag: true, Pointer: &flag}
	b.Nested.Value = true
	b.Reset()
	if b.Value != 0 || b.Array[0] || b.Array[1] || b.Nested.Value || bool(b.Flag) || bool(flag) {
		t.Fatal("Reset не сбросил поля без предопределённого false")
	}
	if b.Pointer != &flag { t.Fatal("Reset заменил указатель") }
}
`,
		},
		{
			name: "promoted Reset field",
			files: map[string]string{
				"source.go": `package fixture
type Inner struct { Reset bool }
// generate:reset
type Box struct { Inner; Value int }
`,
			},
			tests: `package fixture
import "testing"
func TestReset(t *testing.T) {
	b := Box{Inner: Inner{Reset: true}, Value: 7}
	b.Reset()
	if b.Inner.Reset || b.Value != 0 { t.Fatal("Reset не сбросил встроенное поле") }
	var empty *Box
	empty.Reset()
}
`,
		},
		{
			name: "promoted Reset method",
			files: map[string]string{
				"source.go": `package fixture
type Inner struct { Value int; Items []int }
func (i *Inner) Reset() { i.Value = 0; i.Items = i.Items[:0] }
// generate:reset
type Box struct { Inner; Value int }
`,
			},
			tests: `package fixture
import "testing"
func TestReset(t *testing.T) {
	items := []int{1, 2}
	b := Box{Inner: Inner{Value: 7, Items: items}, Value: 8}
	b.Reset()
	if b.Inner.Value != 0 || b.Value != 0 || len(b.Items) != 0 {
		t.Fatal("Reset не сбросил поля через встроенный метод")
	}
	if cap(b.Items) != cap(items) || &b.Items[:1][0] != &items[0] { t.Fatal("Reset заменил слайс") }
	var empty *Box
	empty.Reset()
}
`,
		},
		{
			name: "map NaN identity and nil",
			files: map[string]string{
				"source.go": `package fixture
var delete = 123
// generate:reset
type Box struct {
	Map map[float64]int
	Pointer *map[float64]int
	Nil map[float64]int
	NilPointer *map[float64]int
	NilInner *map[float64]int
}
`,
			},
			tests: `package fixture
import (
	"math"
	"testing"
)
func TestReset(t *testing.T) {
	entries := map[float64]int{math.NaN(): 1}
	pointed := map[float64]int{math.NaN(): 2}
	var empty map[float64]int
	b := Box{Map: entries, Pointer: &pointed, NilInner: &empty}
	b.Reset()
	if len(entries) != 0 || len(pointed) != 0 || len(b.Map) != 0 || len(*b.Pointer) != 0 {
		t.Fatal("мапы с NaN не очищены")
	}
	if b.Map == nil || b.Pointer != &pointed || pointed == nil { t.Fatal("мапы или указатель заменены") }
	entries[1] = 3
	pointed[2] = 4
	if b.Map[1] != 3 || (*b.Pointer)[2] != 4 { t.Fatal("мапы потеряли aliases") }
	if b.Nil != nil || b.NilPointer != nil || b.NilInner != &empty || empty != nil {
		t.Fatal("Reset изменил nil-мапы или указатели")
	}
}
`,
		},
	} {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			dir := fixtureModule(t, tc.files)
			testFixtureModule(t, dir)
			runFixtureGenerator(t, dir)
			writeFixtureFile(t, dir, "source_test.go", tc.tests)
			testFixtureModule(t, dir)
			assertFixtureGeneratorIdempotent(t, dir)
		})
	}
}

func TestBuildConstraintCgo(t *testing.T) {
	dir := fixtureModule(t, map[string]string{
		"source.go": "package fixture\n",
		"native.go": "package fixture\n\nimport \"C\"\n",
		"tagged.go": "//go:build " + runtime.GOOS + "\n\npackage fixture\n",
	})
	if err := checkBuildConstraints(filepath.Join(dir, "native.go")); err == nil ||
		!strings.Contains(err.Error(), "ограничения сборки") || !strings.Contains(err.Error(), "cgo") {
		t.Fatalf("не распознано ограничение cgo: %v", err)
	}
	if err := checkBuildConstraints(filepath.Join(dir, "source.go")); err != nil {
		t.Fatalf("ограничения соседних файлов затронули обычный файл: %v", err)
	}
}
