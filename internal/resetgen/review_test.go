package resetgen

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGeneratorReview(t *testing.T) {
	binary := resetTestBinary

	t.Run("regenerate CRLF output", func(t *testing.T) {
		dir := fixtureModule(t, map[string]string{
			"source.go": "package fixture\n\n// generate:reset\ntype Box struct { Value int }\n",
		})
		testFixtureModule(t, dir)
		runFixtureGenerator(t, dir)
		first := readFixtureFile(t, dir, genFile)
		writeFixtureFile(t, dir, genFile, strings.ReplaceAll(string(first), "\n", "\r\n"))
		testFixtureModule(t, dir)
		runFixtureGenerator(t, dir)
		if got := readFixtureFile(t, dir, genFile); !bytes.Equal(got, first) {
			t.Fatalf("повторная генерация изменила содержимое:\n%s", got)
		}
		testFixtureModule(t, dir)
	})

	for _, body := range []string{"const Original = 42\r\n", "func invalid(\r\n"} {
		t.Run("remove stale CRLF output/"+strings.Fields(body)[1], func(t *testing.T) {
			dir := fixtureModule(t, map[string]string{"source.go": "package fixture\n"})
			testFixtureModule(t, dir)
			writeFixtureFile(t, dir, genFile, genHeader+"\r\n\r\npackage fixture\r\n\r\n"+body)
			runFixtureGenerator(t, dir)
			if _, err := os.Stat(filepath.Join(dir, genFile)); !os.IsNotExist(err) {
				t.Fatalf("устаревший файл CRLF не удалён: %v", err)
			}
			testFixtureModule(t, dir)
		})
	}

	for _, tc := range []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "self pointer cycle",
			source: "type Link *Link\n\n// generate:reset\ntype Box struct { Value Link }\n",
			want:   "циклическ",
		},
		{
			name:   "mutual pointer cycle",
			source: "type Left *Right\ntype Right *Left\n\n// generate:reset\ntype Box struct { Value Left }\n",
			want:   "циклическ",
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
			dir := fixtureModule(t, files)
			testFixtureModule(t, dir)
			before := snapshotFixtureFiles(t, dir)

			command := fixtureCommand{dir: dir, timeout: 5 * time.Second, name: binary}
			output, err, contextErr := executeFixtureCommand(command)
			if contextErr != nil {
				t.Errorf("генератор не завершился за отведённое время: %v", contextErr)
			} else if err == nil {
				t.Errorf("генератор не сообщил об ошибке:\n%s", output)
			} else {
				for _, want := range []string{tc.want, "Box.Value"} {
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
		child string
	}{
		{
			name: "foreign hidden struct",
			child: `type hidden struct { Number int }
type Child struct { Value hidden }
func New() Child { return Child{Value: hidden{Number: 7}} }
`,
		},
		{
			name: "foreign anonymous private field",
			child: `type Child struct { Value struct { private int } }
func New() Child { var child Child; child.Value.private = 7; return child }
`,
		},
		{
			name: "foreign anonymous blank field",
			child: `type Child struct { Value struct { _ int; Number int } }
func New() Child { var child Child; child.Value.Number = 7; return child }
`,
		},
		{
			name: "array of foreign hidden type",
			child: `type hidden int
type Child struct { Value [1]hidden }
func New() Child { return Child{Value: [1]hidden{7}} }
`,
		},
		{
			name: "foreign hidden generic argument",
			child: `type hidden int
type Container[T any] struct { Item T }
type Child struct { Value Container[hidden] }
func New() Child { return Child{Value: Container[hidden]{Item: 7}} }
`,
		},
	} {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			dir := fixtureModule(t, map[string]string{
				"source.go":       "package fixture\n\nimport \"example.com/reset-regression/child\"\n\n// generate:reset\ntype Box child.Child\n",
				"child/source.go": "package child\n\n" + tc.child,
			})
			testFixtureModule(t, dir)
			runFixtureGenerator(t, dir)
			first := readFixtureFile(t, dir, genFile)
			writeFixtureFile(t, dir, "source_test.go", `package fixture
import (
	"example.com/reset-regression/child"
	"reflect"
	"testing"
)
func TestReset(t *testing.T) {
	box := Box(child.New())
	if reflect.DeepEqual(box, Box{}) { t.Fatal("исходные поля уже нулевые") }
	box.Reset()
	if !reflect.DeepEqual(box, Box{}) { t.Fatalf("недоступные составные значения не обнулены: %#v", box) }
}
`)
			testFixtureModule(t, dir)
			assertFixtureGeneratorIdempotent(t, dir)
			if got := readFixtureFile(t, dir, genFile); !bytes.Equal(got, first) {
				t.Fatalf("добавление теста изменило файл:\n%s", got)
			}
		})
	}

	t.Run("accept exported aliases and named types", func(t *testing.T) {
		dir := fixtureModule(t, map[string]string{
			"source.go": "package fixture\n\nimport \"example.com/reset-regression/child\"\n\n// generate:reset\ntype Box child.Child\n",
			"child/source.go": `package child
type hidden struct { private int }
type Public = hidden
type Exported struct { private int }
type Container[T any] struct { private T }
type Child struct {
	Alias Public
	Named Exported
	Array [1]Exported
	Generic Container[Public]
}
func New() Child {
	return Child{Public{1}, Exported{2}, [1]Exported{{3}}, Container[Public]{Public{4}}}
}
`,
		})
		testFixtureModule(t, dir)
		runFixtureGenerator(t, dir)
		first := readFixtureFile(t, dir, genFile)
		writeFixtureFile(t, dir, "source_test.go", `package fixture
import (
	"example.com/reset-regression/child"
	"reflect"
	"testing"
)
func TestReset(t *testing.T) {
	box := Box(child.New())
	box.Reset()
	if !reflect.DeepEqual(box, Box{}) { t.Fatalf("поля не сброшены: %#v", box) }
}
`)
		testFixtureModule(t, dir)
		assertFixtureGeneratorIdempotent(t, dir)
		if got := readFixtureFile(t, dir, genFile); !bytes.Equal(got, first) {
			t.Fatalf("добавление теста изменило файл:\n%s", got)
		}
	})
}
