package resetgen

import (
	"bytes"
	"testing"
)

func TestGeneratorTestNames(t *testing.T) {

	const source = `package fixture

// generate:reset
type Box struct { Array [1]int }
`
	const resetTest = `
func TestReset(t *testing.T) {
	box := Box{Array: [1]int{42}}
	box.Reset()
	if box.Array != [1]int{} { t.Fatalf("массив не обнулён: %#v", box) }
}
`
	for _, tc := range []struct {
		name string
		decl string
	}{
		{"function", "func resetZero() {}"},
		{"type", "type resetZero struct{}"},
		{"variable", "var resetZero int"},
		{"constant", "const resetZero = 42"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixtureModule(t, map[string]string{
				"source.go": source,
				"source_test.go": "package fixture\n\nimport \"testing\"\n\n" +
					tc.decl + "\nvar resetZero2 int\n" + resetTest,
			})
			runFixtureGenerator(t, dir)
			testFixtureModule(t, dir)
			assertFixtureGeneratorIdempotent(t, dir)
		})
	}

	t.Run("aliased imports", func(t *testing.T) {
		dir := fixtureModule(t, map[string]string{
			"source.go": `package fixture

type Counter int
func (c *Counter) Reset() { *c = 0 }

// generate:reset
type Box[T interface{ Reset() }] struct {
	Value T
	Array [1]int
}
`,
			"source_test.go": `package fixture

import (
	resetValue "fmt"
	resetReflect "reflect"
	resetZero "strings"
	"testing"
)

func TestReset(t *testing.T) {
	counter := Counter(42)
	box := Box[*Counter]{Value: &counter, Array: [1]int{42}}
	box.Reset()
	if counter != 0 || !resetReflect.DeepEqual(box.Array, [1]int{}) {
		t.Fatal(resetZero.TrimSpace(resetValue.Sprintf("не обнулено: %#v", box)))
	}
}
`,
		})
		runFixtureGenerator(t, dir)
		testFixtureModule(t, dir)
	})

	t.Run("default import name differs from directory", func(t *testing.T) {
		dir := fixtureModule(t, map[string]string{
			"source.go":           source,
			"different/source.go": "package resetZero\n\nconst Value = 42\n",
			"source_test.go": `package fixture

import (
	"example.com/reset-regression/different"
	"testing"
)

var _ = resetZero.Value
` + resetTest,
		})
		runFixtureGenerator(t, dir)
		testFixtureModule(t, dir)
	})

	for _, tc := range []struct {
		name       string
		tests      string
		dependency string
	}{
		{
			name:  "inactive function",
			tests: "import \"testing\"\n\nfunc resetZero() {}\n" + resetTest,
		},
		{
			name: "inactive default import",
			tests: "import (\"testing\"; \"example.com/reset-regression/different\")\n" +
				"var _ = resetZero.Value\n" + resetTest,
			dependency: "package resetZero\n\nconst Value = 42\n",
		},
		{
			name: "inactive default import with excluded dependency",
			tests: "import (\"testing\"; \"example.com/reset-regression/different\")\n" +
				"var _ = resetZero.Value\n" + resetTest,
			dependency: "//go:build custom\n\npackage resetZero\n\nconst Value = 42\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{
				"source.go":      source,
				"source_test.go": "//go:build custom\n\npackage fixture\n\n" + tc.tests,
			}
			if tc.dependency != "" {
				files["different/source.go"] = tc.dependency
			}
			dir := fixtureModule(t, files)
			runFixtureGenerator(t, dir)
			testFixtureModule(t, dir)
			testFixtureModule(t, dir, "-tags=custom")
			assertFixtureGeneratorIdempotent(t, dir)
		})
	}

	for _, tc := range []struct {
		name   string
		source string
	}{
		{"external test package", "package fixture_test\n\nfunc resetZero() {}\n"},
		{"unrelated declarations", "package fixture\n\nimport \"testing\"\n" + resetTest},
		{"method with matching name", "package fixture\n\ntype Extra struct{}\nfunc (Extra) resetZero() {}\n"},
		{"inactive external test package", "//go:build custom\n\npackage fixture_test\n\nfunc resetZero() {}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixtureModule(t, map[string]string{"source.go": source})
			runFixtureGenerator(t, dir)
			first := readFixtureFile(t, dir, genFile)
			writeFixtureFile(t, dir, "source_test.go", tc.source)
			assertFixtureGeneratorIdempotent(t, dir)
			testFixtureModule(t, dir)
			if second := readFixtureFile(t, dir, genFile); !bytes.Equal(first, second) {
				t.Fatalf("тесты без конфликтов изменили генерацию:\n%s", second)
			}
		})
	}
}
