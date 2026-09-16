package resetgen

import (
	"maps"
	"runtime"
	"strings"
	"testing"
)

func TestGeneratorZeroConstraintBuildTags(t *testing.T) {
	binary := resetTestBinary

	for _, tc := range []struct {
		name   string
		source string
	}{
		{"direct", "// generate:reset\ntype Holder[T Constraint] struct { Value T }\n"},
		{"alias", "type Alias = Constraint\n// generate:reset\ntype Holder[T Alias] struct { Value T }\n"},
		{"embedded", "type Alias = interface { Constraint }\ntype Nested interface { any; Alias }\n// generate:reset\ntype Holder[T Nested] struct { Value T }\n"},
		{"pointer", "// generate:reset\ntype Holder[T Constraint] struct { Value *T }\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, build := range []struct {
				name  string
				flags string
				file  string
			}{
				{"default", "", "constraint_default.go"},
				{"custom", "-tags=custom", "constraint_custom.go"},
			} {
				t.Run(build.name, func(t *testing.T) {
					t.Setenv("GOFLAGS", build.flags)
					dir := fixtureModule(t, map[string]string{
						"source.go": "package fixture\n" + tc.source + `
type Item struct { Value int }
func (item *Item) Reset() { item.Value = 0 }
var _ Holder[*Item]
`,
						"constraint_default.go": "//go:build !custom\n\npackage fixture\ntype Constraint interface {}\n",
						"constraint_custom.go":  "//go:build custom\n\npackage fixture\ntype Constraint interface { Reset() }\n",
						genFile:                 genHeader + "\n\npackage fixture\nconst Original = 42\n",
						"existing/source.go":    "package existing\n// generate:reset\ntype Existing struct { Value int }\n",
						"existing/" + genFile:   genHeader + "\n\npackage existing\nfunc (e *Existing) Reset() { e.Value = 42 }\n",
						"obsolete/source.go":    "package obsolete\n",
						"obsolete/" + genFile:   genHeader + "\n\npackage obsolete\nconst Original = 42\n",
						"new/source.go":         "package fresh\n// generate:reset\ntype Fresh struct { Value int }\n",
					})
					testFixtureModule(t, dir)
					before := snapshotFixtureFiles(t, dir)
					output, err, _ := executeFixtureCommand(fixtureCommand{dir: dir, name: binary})
					if err == nil {
						t.Errorf("генератор не сообщил об ограничении сборки:\n%s", output)
					} else {
						for _, want := range []string{"Holder.Value", build.file, "ограничения сборки"} {
							if !strings.Contains(string(output), want) {
								t.Errorf("ошибка не содержит %q:\n%s", want, output)
							}
						}
					}
					assertFixtureFilesUnchanged(t, dir, before)
				})
			}
		})
	}
}

func TestGeneratorConstraintDependencies(t *testing.T) {
	binary := resetTestBinary

	otherOS := "linux"
	if runtime.GOOS == otherOS {
		otherOS = "darwin"
	}
	platformFile := "items_" + runtime.GOOS + ".go"
	alternateFile := "items_" + otherOS + ".go"
	platformTypes := func(current, alternate string) map[string]string {
		return map[string]string{
			platformFile:  "package fixture\n\n" + current + "\n",
			alternateFile: "package fixture\n\n" + alternate + "\n",
		}
	}

	for _, tc := range []struct {
		name   string
		source string
		files  map[string]string
		want   []string
	}{
		{
			name:   "platform slice and array",
			source: "// generate:reset\ntype Box struct { Items Items }\n",
			files:  platformTypes("type Items []int", "type Items [4]int"),
			want:   []string{"Box.Items", platformFile},
		},
		{
			name:   "local alias with build tags",
			source: "type Items = PlatformItems\n\n// generate:reset\ntype Box struct { Items Items }\n",
			files: map[string]string{
				"items.go": "//go:build " + runtime.GOARCH + "\n\npackage fixture\n\ntype PlatformItems []int\n",
				"other.go": "//go:build !" + runtime.GOARCH + "\n\npackage fixture\n\ntype PlatformItems [4]int\n",
			},
			want: []string{"Box.Items", "items.go"},
		},
		{
			name:   "defined type chain",
			source: "type Items PlatformItems\n\n// generate:reset\ntype Box struct { Items Items }\n",
			files:  platformTypes("type PlatformItems []int", "type PlatformItems [4]int"),
			want:   []string{"Box.Items", platformFile},
		},
		{
			name:   "generic alias argument",
			source: "type Identity[T any] = *T\ntype Items Identity[PlatformItems]\n\n// generate:reset\ntype Box struct { Items Items }\n",
			files:  platformTypes("type PlatformItems []int", "type PlatformItems int"),
			want:   []string{"Box.Items", platformFile},
		},
		{
			name:   "nested generic alias argument",
			source: "type Identity[T any] = *T\ntype Nested[T any] = Identity[T]\ntype Items (Nested[Identity[PlatformItems]])\n\n// generate:reset\ntype Box struct { Items Items }\n",
			files:  platformTypes("type PlatformItems []int", "type PlatformItems int"),
			want:   []string{"Box.Items", platformFile},
		},
		{
			name:   "multiple generic alias arguments",
			source: "type Second[A, B any] = *B\ntype Items Second[int, PlatformItems]\n\n// generate:reset\ntype Box struct { Items Items }\n",
			files:  platformTypes("type PlatformItems []int", "type PlatformItems int"),
			want:   []string{"Box.Items", platformFile},
		},
		{
			name:   "external generic alias argument",
			source: "import renamed \"example.com/reset-external/child\"\n\n// generate:reset\ntype Box struct { Items renamed.Items }\n",
			files: map[string]string{
				"go.mod":                         fixtureModuleWithExternalFile,
				"external/go.mod":                externalModuleFile,
				"external/child/source.go":       "package child\n\nimport renamed \"example.com/reset-external/leaf\"\n\ntype Items renamed.Identity[renamed.PlatformItems]\n",
				"external/leaf/source.go":        "package leaf\n\ntype Identity[T any] = *T\n",
				"external/leaf/" + platformFile:  "package leaf\n\ntype PlatformItems []int\n",
				"external/leaf/" + alternateFile: "package leaf\n\ntype PlatformItems int\n",
			},
			want: []string{"Box.Items", platformFile},
		},
		{
			name:   "platform Reset constraint alias",
			source: "type CommonResetter interface { Reset() }\n\n// generate:reset\ntype Box[T Constraint] struct { Value T }\n",
			files:  platformTypes("type Constraint = CommonResetter", "type Constraint = any"),
			want:   []string{"Box.Value", platformFile},
		},
		{
			name:   "embedded platform Reset constraint alias",
			source: "type CommonResetter interface { Reset() }\n\n// generate:reset\ntype Box[T interface { Constraint }] struct { Value T }\n",
			files:  platformTypes("type Constraint = CommonResetter", "type Constraint = any"),
			want:   []string{"Box.Value", platformFile},
		},
		{
			name:   "inactive platform Reset constraint alias",
			source: "type CommonResetter interface { Reset() }\n\n// generate:reset\ntype Box[T Constraint] struct { Value T }\n",
			files:  platformTypes("type Constraint = any", "type Constraint = CommonResetter"),
			want:   []string{"Box.Value", platformFile},
		},
		{
			name:   "embedded inactive platform Reset constraint alias",
			source: "type CommonResetter interface { Reset() }\n\n// generate:reset\ntype Box[T interface { Constraint }] struct { Value T }\n",
			files:  platformTypes("type Constraint = any", "type Constraint = CommonResetter"),
			want:   []string{"Box.Value", platformFile},
		},
		{
			name:   "concrete term beside platform Reset constraint alias",
			source: "type CommonResetter interface { Reset() }\ntype Concrete struct{}\nfunc (*Concrete) Reset() {}\n\n// generate:reset\ntype Box[T interface { *Concrete; Constraint }] struct { Value T }\n",
			files:  platformTypes("type Constraint = CommonResetter", "type Constraint = any"),
			want:   []string{"Box.Value", platformFile},
		},
		{
			name:   "imported chain with renamed import",
			source: "import renamed \"example.com/reset-external/child\"\n\ntype Items renamed.Items\n\n// generate:reset\ntype Box struct { Items Items }\n",
			files: map[string]string{
				"go.mod":                         fixtureModuleWithExternalFile,
				"external/go.mod":                externalModuleFile,
				"external/child/source.go":       "package child\n\nimport renamed \"example.com/reset-external/leaf\"\n\ntype Items renamed.PlatformItems\n",
				"external/leaf/" + platformFile:  "package leaf\n\ntype PlatformItems []int\n",
				"external/leaf/" + alternateFile: "package leaf\n\ntype PlatformItems [4]int\n",
			},
			want: []string{"Box.Items", platformFile},
		},
		{
			name:   "imported chain with hidden builtin name",
			source: "import \"example.com/reset-external/child\"\n\n// generate:reset\ntype Box struct { Items child.Items }\n",
			files: map[string]string{
				"go.mod":                          fixtureModuleWithExternalFile,
				"external/go.mod":                 externalModuleFile,
				"external/child/source.go":        "package child\n\ntype Items string\n",
				"external/child/" + platformFile:  "package child\n\ntype string []int\n",
				"external/child/" + alternateFile: "package child\n\ntype string [4]int\n",
			},
			want: []string{"Box.Items", platformFile},
		},
		{
			name:   "imported chain with dot import",
			source: "import . \"example.com/reset-regression/child\"\n\ntype Local Items\n\n// generate:reset\ntype Box struct { Items Local }\n",
			files: map[string]string{
				"child/source.go":        "package child\n\ntype Items = PlatformItems\n",
				"child/" + platformFile:  "package child\n\ntype PlatformItems []int\n",
				"child/" + alternateFile: "package child\n\ntype PlatformItems [4]int\n",
			},
			want: []string{"Box.Items", platformFile},
		},
		{
			name:   "named pointer",
			source: "type Items *PlatformItems\n\n// generate:reset\ntype Box struct { Items Items }\n",
			files:  platformTypes("type PlatformItems []int", "type PlatformItems [4]int"),
			want:   []string{"Box.Items", platformFile},
		},
		{
			name:   "generic field",
			source: "// generate:reset\ntype Box struct { Items Items[int] }\n",
			files:  platformTypes("type Items[T any] []T", "type Items[T any] [4]T"),
			want:   []string{"Box.Items", platformFile},
		},
		{
			name:   "generic derived struct",
			source: "// generate:reset\ntype Box[T any] Base[T]\n",
			files:  platformTypes("type Base[T any] struct { Items []T }", "type Base[T any] struct { Items [4]T }"),
			want:   []string{"Box", platformFile},
		},
		{
			name:   "empty derived struct",
			source: "type Middle Base\n\n// generate:reset\ntype Box Middle\n",
			files:  platformTypes("type Base struct {}", "type Base struct { Items []int }"),
			want:   []string{"Box", platformFile},
		},
		{
			name:   "platform Reset method",
			source: "type Items []int\n\n// generate:reset\ntype Box struct { Items Items }\n",
			files: map[string]string{
				platformFile: "package fixture\n\nfunc (items *Items) Reset() { *items = (*items)[:0] }\n",
			},
			want: []string{"Box.Items", platformFile},
		},
	} {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			files := map[string]string{
				"source.go":           "package fixture\n\n" + tc.source,
				genFile:               genHeader + "\n\npackage fixture\n\nconst Original = 42\n",
				"existing/source.go":  "package existing\n\n// generate:reset\ntype Existing struct { Value int }\n",
				"existing/" + genFile: genHeader + "\n\npackage existing\n\nfunc (e *Existing) Reset() { e.Value = 42 }\n",
				"obsolete/source.go":  "package obsolete\n",
				"obsolete/" + genFile: genHeader + "\n\npackage obsolete\n\nconst Original = 42\n",
				"new/source.go":       "package fresh\n\n// generate:reset\ntype Fresh struct { Value int }\n",
			}
			maps.Copy(files, tc.files)
			dir := fixtureModule(t, files)
			testFixtureModule(t, dir)
			requireFixtureCommand(t, fixtureCommand{
				dir:  dir,
				name: "env",
				args: []string{"GOOS=" + otherOS, "GOARCH=amd64", "CGO_ENABLED=0", "go", "build", "./..."},
			})
			before := snapshotFixtureFiles(t, dir)

			output, err, _ := executeFixtureCommand(fixtureCommand{dir: dir, name: binary})
			if err == nil {
				t.Errorf("генератор не сообщил об ограничении сборки:\n%s", output)
			} else {
				for _, want := range append(tc.want, "ограничения сборки") {
					if !strings.Contains(string(output), want) {
						t.Errorf("ошибка не содержит %q:\n%s", want, output)
					}
				}
			}
			assertFixtureFilesUnchanged(t, dir, before)
		})
	}

	t.Run("accept generic aliases with stable slice shape", func(t *testing.T) {
		files := platformTypes("type PlatformItems []int", "type PlatformItems int")
		files["source.go"] = `package fixture
type Fixed[T any] = []int
type Slice[T any] = []T
type FixedItems Fixed[PlatformItems]
type SliceItems Slice[PlatformItems]
// generate:reset
type Box struct { Fixed FixedItems; Slice SliceItems }
`
		dir := fixtureModule(t, files)
		runFixtureGenerator(t, dir)
		writeFixtureFile(t, dir, "source_test.go", `package fixture
import "testing"
func TestReset(t *testing.T) {
	fixed := make(FixedItems, 2, 4)
	slice := make(SliceItems, 2, 4)
	box := Box{Fixed: fixed, Slice: slice}
	box.Reset()
	if len(box.Fixed) != 0 || len(box.Slice) != 0 ||
		cap(box.Fixed) != cap(fixed) || cap(box.Slice) != cap(slice) ||
		&box.Fixed[:1][0] != &fixed[0] || &box.Slice[:1][0] != &slice[0] {
		t.Fatal("слайсы потеряли исходный массив или не сброшены")
	}
}
`)
		testFixtureModule(t, dir)
		requireFixtureCommand(t, fixtureCommand{
			dir:  dir,
			name: "env",
			args: []string{"GOOS=" + otherOS, "GOARCH=amd64", "CGO_ENABLED=0", "go", "build", "./..."},
		})
		assertFixtureGeneratorIdempotent(t, dir)
	})

	t.Run("accept stable Reset beside platform constraint", func(t *testing.T) {
		files := platformTypes("type Constraint interface { Reset() }", "type Constraint interface {}")
		files["source.go"] = `package fixture
type CommonResetter interface { Reset() }
// generate:reset
type Explicit[T interface { Reset(); Constraint }] struct { Value T }
// generate:reset
type Embedded[T interface { Constraint; CommonResetter }] struct { Value T }
`
		dir := fixtureModule(t, files)
		runFixtureGenerator(t, dir)
		writeFixtureFile(t, dir, "source_test.go", `package fixture
import (
	"bytes"
	"testing"
)
func TestReset(t *testing.T) {
	first := bytes.NewBufferString("first")
	second := bytes.NewBufferString("second")
	explicit := Explicit[*bytes.Buffer]{Value: first}
	embedded := Embedded[*bytes.Buffer]{Value: second}
	explicit.Reset()
	embedded.Reset()
	if explicit.Value != first || embedded.Value != second || first.Len() != 0 || second.Len() != 0 {
		t.Fatal("метод ограничения не вызван или указатель заменён")
	}
}
`)
		testFixtureModule(t, dir)
		requireFixtureCommand(t, fixtureCommand{
			dir:  dir,
			name: "env",
			args: []string{"GOOS=" + otherOS, "GOARCH=amd64", "CGO_ENABLED=0", "go", "build", "./..."},
		})
		assertFixtureGeneratorIdempotent(t, dir)
	})

	t.Run("accept ordinary types beside platform file", func(t *testing.T) {
		dir := fixtureModule(t, map[string]string{
			"source.go": `package fixture
type Items []int
type Alias = Items
type Plain interface { any }
type Stable = interface { Plain }
type Value[T any] = *T
type Generic[T any] struct { Value Value[T]; Items []T }
type Base struct { Items Alias }
// generate:reset
type Derived Base
// generate:reset
type Box[T Stable] Generic[T]
`,
			"unrelated_" + runtime.GOOS + ".go": "package fixture\n\ntype T []int\n",
		})
		testFixtureModule(t, dir)
		runFixtureGenerator(t, dir)
		writeFixtureFile(t, dir, "source_test.go", `package fixture
import "testing"
func TestReset(t *testing.T) {
	items := make(Items, 2, 4)
	items[0] = 7
	derived := Derived{Items: items}
	value := 9
	box := Box[int]{Value: &value, Items: items}
	derived.Reset()
	box.Reset()
	if box.Value != &value || value != 0 || len(box.Items) != 0 || len(derived.Items) != 0 {
		t.Fatal("обычные типы не сброшены")
	}
	if cap(box.Items) != cap(items) || cap(derived.Items) != cap(items) ||
		&box.Items[:1][0] != &items[0] || &derived.Items[:1][0] != &items[0] {
		t.Fatal("слайсы потеряли исходный массив")
	}
}
`)
		testFixtureModule(t, dir)
		assertFixtureGeneratorIdempotent(t, dir)
	})

	t.Run("accept unrelated type errors in dependency", func(t *testing.T) {
		const externalSource = "package external\n\ntype Flag bool\n"
		dir := fixtureModule(t, map[string]string{
			"go.mod":             fixtureModuleWithExternalFile,
			"source.go":          "package fixture\n\nimport \"example.com/reset-external\"\n\n// generate:reset\ntype Box struct { Flag external.Flag }\n",
			"external/go.mod":    externalModuleFile,
			"external/source.go": externalSource + "\nvar Broken = Missing\n",
		})
		runFixtureGenerator(t, dir)
		writeFixtureFile(t, dir, "external/source.go", externalSource)
		writeFixtureFile(t, dir, "source_test.go", `package fixture
import "testing"
func TestReset(t *testing.T) {
	box := Box{Flag: true}
	box.Reset()
	if box.Flag { t.Fatal("булево поле не сброшено") }
}
`)
		testFixtureModule(t, dir)
		assertFixtureGeneratorIdempotent(t, dir)
	})

	t.Run("accept generated Reset from external module", func(t *testing.T) {
		const externalReset = genHeader + `

package external

func (item *Item) Reset() { item.Value = 77; Calls++ }
`
		dir := fixtureModule(t, map[string]string{
			"go.mod": fixtureModuleWithExternalFile,
			"source.go": `package fixture
import "example.com/reset-external"
// generate:reset
type Box struct { Item external.Item; Pointer *external.Item }
`,
			"external/go.mod":     externalModuleFile,
			"external/source.go":  "package external\n\n// generate:reset\ntype Item struct { Value int }\n\nvar Calls int\n",
			"external/" + genFile: externalReset,
		})
		testFixtureModule(t, dir)
		runFixtureGenerator(t, dir)
		writeFixtureFile(t, dir, "source_test.go", `package fixture
import (
	"example.com/reset-external"
	"testing"
)
func TestReset(t *testing.T) {
	external.Calls = 0
	pointed := external.Item{Value: 3}
	box := Box{Item: external.Item{Value: 2}, Pointer: &pointed}
	box.Reset()
	if external.Calls != 2 || box.Item.Value != 77 || pointed.Value != 77 || box.Pointer != &pointed {
		t.Fatalf("метод зависимости не вызван: box=%#v, pointer=%#v, calls=%d", box, pointed, external.Calls)
	}
}
`)
		testFixtureModule(t, dir)
		assertFixtureGeneratorIdempotent(t, dir)
		if got := string(readFixtureFile(t, dir, "external/"+genFile)); got != externalReset {
			t.Errorf("изменён сгенерированный файл зависимости:\n%s", got)
		}
	})
}
