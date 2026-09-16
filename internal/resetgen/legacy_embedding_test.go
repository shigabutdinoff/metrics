package resetgen

import (
	"bytes"
	"regexp"
	"testing"
)

func TestGeneratorLegacyEmbedding(t *testing.T) {
	binary := resetTestBinary
	seed := fixtureModule(t, map[string]string{
		"source.go": "package fixture\n",
		"external/source.go": `package external
type Probe struct { Calls int }
func (p *Probe) Reset() { p.Calls++ }
// generate:reset
type Node struct { Number int; Probe Probe; Next *Node }
// generate:reset
type Holder[T interface { Reset() }] struct { Value T }
`,
	})
	runFixtureGenerator(t, seed)
	external := readFixtureFile(t, seed, "external/"+genFile)
	owner := regexp.MustCompile(`, _ \.\.\.\*[^)]+`)
	if len(owner.FindAll(external, -1)) != 2 {
		t.Fatalf("не найдены параметры владельцев сгенерированного протокола:\n%s", external)
	}
	external = owner.ReplaceAll(external, nil)
	helper := bytes.Index(external, []byte("func resetValue("))
	if helper < 0 {
		t.Fatalf("не найден helper сгенерированного протокола:\n%s", external)
	}
	external = append(external[:helper], legacyResetValue...)
	fixture := func(t *testing.T, source string) string {
		t.Helper()
		return fixtureModule(t, map[string]string{
			"go.mod":              fixtureModuleWithExternalFile,
			"source.go":           source,
			genFile:               genHeader + "\n\npackage fixture\n\nconst Original = 42\n",
			"external/go.mod":     externalModuleFile,
			"external/source.go":  string(readFixtureFile(t, seed, "external/source.go")),
			"external/" + genFile: string(external),
		})
	}

	for _, tc := range []struct {
		name      string
		field     string
		manual    string
		embedding string
	}{
		{name: "ordinary value", field: "Manual", embedding: "Inner"},
		{name: "ordinary pointer", field: "*Manual", embedding: "Inner"},
		{name: "local generic", field: "Holder[*Manual]", embedding: "Inner"},
		{name: "external generic", field: "external.Holder[*Manual]", embedding: "Inner"},
		{
			name:      "value receiver through terminal pointer",
			field:     "Manual",
			embedding: "LegacyValue",
			manual: `type LegacyValue struct{}
func (LegacyValue) ResetWithVisited(map[interface{}]struct{}, interface { Reset() }) {}
type Manual struct { *LegacyValue; Calls int }
`,
		},
	} {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			manual := tc.manual
			if manual == "" {
				manual = "type Manual struct { *Inner; Calls int }\n"
			}
			dir := fixture(t, `package fixture
import external "example.com/reset-external"
type Inner struct { *external.Node }
`+manual+`func (m *Manual) Reset() { m.Calls++ }
// generate:reset
type Holder[T interface { Reset() }] struct { Value T }
// generate:reset
type Root struct { Value `+tc.field+` }
`)
			writeFixtureFile(t, dir, "fresh/source.go", "package fresh\n\n// generate:reset\ntype Fresh struct { Value int }\n")
			writeFixtureFile(t, dir, "obsolete/source.go", "package obsolete\n")
			writeFixtureFile(t, dir, "obsolete/"+genFile, genHeader+"\n\npackage obsolete\n\nconst Original = 42\n")
			assertGenericWrapperRejected(t, dir, binary, "Manual", "ResetWithVisited", tc.embedding, "nil")
		})
	}

	t.Run("accept direct nil embedding and shared legacy cycle", func(t *testing.T) {
		dir := fixture(t, `package fixture
import external "example.com/reset-external"
type Manual struct { *external.Node; Calls int }
func (m *Manual) Reset() { m.Calls++ }
type Adapted struct { *external.Node }
func (a *Adapted) Reset() { a.Node.Reset() }
func (a *Adapted) ResetWithVisited(visited map[interface{}]struct{}, original interface { Reset() }) {
	a.Node.ResetWithVisited(visited, a.Node)
}
type OuterAdapted struct { *Adapted }
// generate:reset
type Holder[T interface { Reset() }] struct { Value T }
// generate:reset
type Root struct {
	Node *external.Node
	Value Manual
	Pointer *Manual
	Local Holder[*Manual]
	External external.Holder[*Manual]
	Adapted Holder[*OuterAdapted]
}
`)
		writeFixtureFile(t, dir, "source_test.go", `package fixture
import (
	"runtime/debug"
	"testing"
	external "example.com/reset-external"
)
func TestLegacyCompatibility(t *testing.T) {
	previous := debug.SetMaxStack(1 << 20)
	defer debug.SetMaxStack(previous)
	for _, embedded := range []*external.Node{nil, {Number: 8}} {
		n := &external.Node{Number: 7}
		n.Next = n
		pointer, local, remote := &Manual{Node: embedded}, &Manual{Node: embedded}, &Manual{Node: embedded}
		adapter := &OuterAdapted{Adapted: &Adapted{Node: n}}
		r := Root{
			Node: n,
			Value: Manual{Node: embedded},
			Pointer: pointer,
			Local: Holder[*Manual]{Value: local},
			External: external.Holder[*Manual]{Value: remote},
			Adapted: Holder[*OuterAdapted]{Value: adapter},
		}
		r.Reset()
		if r.Value.Calls != 1 || pointer.Calls != 1 || local.Calls != 1 || remote.Calls != 1 {
			t.Fatal("прямое legacy-встраивание обошло пользовательский Reset")
		}
		if r.Value.Node != embedded || pointer.Node != embedded || local.Node != embedded || remote.Node != embedded ||
			r.Pointer != pointer || r.Local.Value != local || r.External.Value != remote || r.Adapted.Value != adapter {
			t.Fatal("сброс заменил ссылки")
		}
		if embedded != nil && (embedded.Number != 8 || embedded.Probe.Calls != 0) {
			t.Fatal("унаследованный протокол изменил вложенный узел")
		}
		if r.Node != n || n.Next != n || adapter.Node != n || n.Number != 0 || n.Probe.Calls != 1 {
			t.Fatal("legacy-протокол или пользовательский адаптер потерял карту посещений")
		}
	}
}
`)
		runFixtureGenerator(t, dir)
		testFixtureModuleRace(t, dir)
		assertFixtureGeneratorIdempotent(t, dir)
		if !bytes.Equal(external, readFixtureFile(t, dir, "external/"+genFile)) {
			t.Fatal("изменён generated-файл внешнего модуля")
		}
	})
}
