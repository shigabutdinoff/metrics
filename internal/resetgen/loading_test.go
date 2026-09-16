package resetgen

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratorLoading(t *testing.T) {
	binary := resetTestBinary

	for _, obsolete := range []bool{false, true} {
		name := "regenerate after removing dependency"
		if obsolete {
			name = "remove obsolete output after removing dependency"
		}
		t.Run(name, func(t *testing.T) {
			dir := fixtureModule(t, map[string]string{
				"source.go": `package fixture

import "example.com/reset-regression/foreign"

// generate:reset
type Box struct { Value foreign.Value }
`,
				"foreign/source.go": "package foreign\n\ntype Value struct { Number int }\n",
				genFile: genHeader + `

package fixture

import "example.com/reset-regression/foreign"

func (b *Box) Reset() { b.Value = foreign.Value{} }
`,
			})
			testFixtureModule(t, dir)
			source := "package fixture\n\n// generate:reset\ntype Box struct { Value int }\n"
			if obsolete {
				source = "package fixture\n"
			}
			writeFixtureFile(t, dir, "source.go", source)
			if err := os.RemoveAll(filepath.Join(dir, "foreign")); err != nil {
				t.Fatal(err)
			}

			runFixtureGenerator(t, dir)
			if obsolete {
				if _, err := os.Stat(filepath.Join(dir, genFile)); !os.IsNotExist(err) {
					t.Fatalf("устаревший файл не удалён: %v", err)
				}
			} else {
				generated := readFixtureFile(t, dir, genFile)
				if bytes.Contains(generated, []byte("foreign")) {
					t.Fatalf("остался импорт удалённой зависимости:\n%s", generated)
				}
				writeFixtureFile(t, dir, "source_test.go", `package fixture

import "testing"

func TestReset(t *testing.T) {
	box := Box{Value: 42}
	box.Reset()
	if box.Value != 0 { t.Fatalf("поле не обнулено: %#v", box) }
}
`)
			}
			testFixtureModule(t, dir)
			before := snapshotFixtureFiles(t, dir)
			runFixtureGenerator(t, dir)
			assertFixtureFilesUnchanged(t, dir, before)
		})
	}

	for _, obsolete := range []bool{false, true} {
		name := "regenerate after renaming package"
		if obsolete {
			name = "remove obsolete output after renaming package"
		}
		t.Run(name, func(t *testing.T) {
			dir := fixtureModule(t, map[string]string{
				"source.go": "package old\n\n// generate:reset\ntype Box struct { Value int }\n",
			})
			runFixtureGenerator(t, dir)
			testFixtureModule(t, dir)
			source := "package renamed\n\n// generate:reset\ntype Box struct { Value int }\n"
			if obsolete {
				source = "package renamed\n\ntype Box struct { Value int }\n"
			}
			writeFixtureFile(t, dir, "source.go", source)

			runFixtureGenerator(t, dir)
			if obsolete {
				if _, err := os.Stat(filepath.Join(dir, genFile)); !os.IsNotExist(err) {
					t.Fatalf("устаревший файл не удалён после переименования пакета: %v", err)
				}
			} else {
				writeFixtureFile(t, dir, "source_test.go", `package renamed

import "testing"

func TestReset(t *testing.T) {
	box := Box{Value: 42}
	box.Reset()
	if box.Value != 0 { t.Fatalf("поле не обнулено: %#v", box) }
}
`)
			}
			testFixtureModule(t, dir)
			before := snapshotFixtureFiles(t, dir)
			runFixtureGenerator(t, dir)
			assertFixtureFilesUnchanged(t, dir, before)
		})
	}

	for _, unowned := range []bool{false, true} {
		name := "missing import in source keeps outputs"
		if unowned {
			name = "missing import in unowned output keeps outputs"
		}
		t.Run(name, func(t *testing.T) {
			files := map[string]string{
				"source.go":           "package fixture\n",
				"existing/source.go":  "package existing\n\n// generate:reset\ntype Box struct { Value int; Other string }\n",
				"existing/" + genFile: genHeader + "\n\npackage existing\n\nfunc (b *Box) Reset() { b.Value = 0 }\n",
				"obsolete/source.go":  "package obsolete\n",
				"obsolete/" + genFile: genHeader + "\n\npackage obsolete\n\nconst Original = 42\n",
				"new/source.go":       "package fresh\n\n// generate:reset\ntype Box struct { Value int }\n",
			}
			broken := "source.go"
			if unowned {
				broken = genFile
			}
			files[broken] = "package fixture\n\nimport _ \"example.com/reset-regression/missing\"\n"
			dir := fixtureModule(t, files)
			before := snapshotFixtureFiles(t, dir)

			output, err, _ := executeFixtureCommand(fixtureCommand{dir: dir, name: binary})
			if err == nil {
				t.Fatalf("генератор не сообщил об отсутствующем импорте:\n%s", output)
			}
			if !strings.Contains(string(output), "example.com/reset-regression/missing") {
				t.Errorf("ошибка не содержит путь отсутствующего импорта:\n%s", output)
			}
			assertFixtureFilesUnchanged(t, dir, before)
		})
	}
}
