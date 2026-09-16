package resetgen

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestGeneratorGenericWrappers(t *testing.T) {
	binary := resetTestBinary

	for _, tc := range []struct {
		name      string
		nodeField string
		source    string
		tests     string
	}{
		{
			name:      "pointer cycle",
			nodeField: "Back *Holder[*Wrapper]",
			source: `type Wrapper struct { *Node }
func makeCycle() *Node {
	n := &Node{Number: 7}
	n.Back = &Holder[*Wrapper]{Value: &Wrapper{Node: n}}
	return n
}
`,
		},
		{
			name:   "value",
			source: "type Wrapper struct { *Node; Items []int }\nvar _ Holder[Wrapper]\n",
		},
		{
			name: "generic multilevel",
			source: `type Inner[T any] struct { *Node; Value T }
type Wrapper[T any] struct { *Inner[T] }
var _ Holder[*Wrapper[string]]
`,
		},
		{
			name: "aliases",
			source: `type Wrapper struct { *Node }
type Alias = *Wrapper
type Container = Holder[Alias]
var _ Container
`,
		},
		{
			name: "nested substitution",
			source: `type Wrapper struct { *Node }
// generate:reset
type Outer[T interface { Reset() }] struct { Inner Holder[T] }
var _ Outer[*Wrapper]
`,
		},
		{
			name: "pointer chain dispatch",
			source: `type Wrapper struct { *Node }
type Pointer[T any] = *T
// generate:reset
type PointerHolder[T interface { Reset() }] struct { Value Pointer[*T] }
var _ PointerHolder[*Wrapper]
`,
		},
		{
			name:   "hidden protocol",
			source: "type Wrapper struct { *Node; ResetWithVisited int }\nvar _ Holder[*Wrapper]\n",
		},
		{
			name: "incompatible protocol",
			source: `type Wrapper struct { *Node }
func (*Wrapper) ResetWithVisited() {}
var _ Holder[*Wrapper]
`,
		},
		{
			name: "wrong adapter owner",
			source: `type Wrapper struct { *Node }
func (*Wrapper) ResetWithVisited(map[interface{}]struct{}, interface { Reset() }, ...*Node) {}
var _ Holder[*Wrapper]
`,
		},
		{
			name:  "same package tests",
			tests: "package fixture\n\ntype Wrapper struct { *Node }\nvar _ Holder[*Wrapper]\n",
		},
		{
			name: "external package tests",
			tests: `package fixture_test
import fixture "example.com/reset-regression"
type Wrapper struct { *fixture.Node }
var _ fixture.Holder[Wrapper]
`,
		},
	} {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			files := map[string]string{
				"source.go": "package fixture\n\n// generate:reset\ntype Node struct { Number int; " + tc.nodeField + " }\n" +
					"// generate:reset\ntype Holder[T interface { Reset() }] struct { Value T }\n" + tc.source,
				genFile: genHeader + "\n\npackage fixture\n\nconst Original = 42\n",
			}
			if tc.tests != "" {
				files["source_test.go"] = tc.tests
			}
			if tc.name == "pointer cycle" {
				files["new/source.go"] = "package fresh\n\n// generate:reset\ntype Fresh struct { Value int }\n"
				files["obsolete/source.go"] = "package obsolete\n"
				files["obsolete/"+genFile] = genHeader + "\n\npackage obsolete\n\nconst Original = 42\n"
			}
			dir := fixtureModule(t, files)
			assertGenericWrapperRejected(t, dir, binary)
		})
	}

	t.Run("reject/flat embedded argument", func(t *testing.T) {
		dir := fixtureModule(t, map[string]string{
			"source.go": `package fixture
// generate:reset
type Flat struct { Number int }
// generate:reset
type Holder[T interface { Reset() }] struct { Value T }
type Wrapper struct { *Flat; Items []int }
var _ Holder[*Wrapper]
`,
		})
		assertGenericWrapperRejected(t, dir, binary, "Wrapper", "Flat", "плоским Reset")
	})

	t.Run("accept unrelated invalid instantiation cycle", func(t *testing.T) {
		dir := fixtureModule(t, map[string]string{
			"source.go": `package fixture
// generate:reset
type Node struct { Value int }
type Grow[T any] struct { Next *Grow[*T] }
var _ Grow[int]
`,
		})
		command := fixtureCommand{dir: dir, timeout: 10 * time.Second, name: binary}
		output, err, contextErr := executeFixtureCommand(command)
		if err != nil {
			t.Fatalf("посторонняя ошибка типов помешала генерации: %v (%v)\n%s", err, contextErr, output)
		}
		if !bytes.Contains(readFixtureFile(t, dir, genFile), []byte("*Node) Reset()")) {
			t.Fatal("не сгенерирован Reset для Node")
		}
	})

	t.Run("accept dispatch boundaries and explicit adapters", func(t *testing.T) {
		dir := fixtureModule(t, map[string]string{
			"source.go":      genericWrapperSource,
			"source_test.go": genericWrapperTests,
		})
		runFixtureGenerator(t, dir)
		first := readFixtureFile(t, dir, genFile)
		for _, receiver := range []string{"*Wrapper)", "*Adapted)", "*TypedAdapted)", "*Manual)"} {
			if bytes.Contains(first, []byte(receiver)) {
				t.Fatalf("сгенерирован метод неразмеченного типа %s:\n%s", receiver, first)
			}
		}
		testFixtureModule(t, dir, "-race", "-timeout=20s")
		assertFixtureGeneratorIdempotent(t, dir)
	})

	for _, protocol := range []string{"typed owner", "legacy"} {
		t.Run("external generated container and wrapper/"+protocol, func(t *testing.T) {
			dir := fixtureModule(t, map[string]string{
				"source.go": "package fixture\n",
				"external/source.go": `package external
// generate:reset
type Node struct { Number int; Next *Node }
// generate:reset
type Holder[T interface { Reset() }] struct { Value T }
// generate:reset
type ArrayOnly[T interface { Reset() }] struct { Values [1]T }
// generate:reset
type Unused[T interface { Reset() }] struct { Number int }
type Wrapper struct { *Node }
`,
			})
			runFixtureGenerator(t, dir)
			external := readFixtureFile(t, dir, "external/"+genFile)
			if protocol == "legacy" {
				owner := regexp.MustCompile(`, _ \.\.\.\*[^)]+`)
				if len(owner.FindAll(external, -1)) != 4 {
					t.Fatalf("не найдены параметры владельцев сгенерированного протокола:\n%s", external)
				}
				external = owner.ReplaceAll(external, nil)
				helper := bytes.Index(external, []byte("func resetValue("))
				if helper < 0 {
					t.Fatalf("не найден helper сгенерированного протокола:\n%s", external)
				}
				external = append(external[:helper], legacyResetValue...)
				writeFixtureFile(t, dir, "external/"+genFile, string(external))
			}
			writeFixtureFile(t, dir, "external/go.mod", externalModuleFile)
			writeFixtureFile(t, dir, "go.mod", fixtureModuleWithExternalFile)
			writeFixtureFile(t, dir, "external/usage.go", "package external\n\ntype Bad = Holder[*Wrapper]\n")
			writeFixtureFile(t, dir, "source.go", `package fixture
import external "example.com/reset-external"
// generate:reset
type Root struct { Value int }
var _ external.Bad
`)
			testFixtureModule(t, dir)
			assertGenericWrapperRejected(t, dir, binary)
			writeFixtureFile(t, dir, "source.go", `package fixture
import external "example.com/reset-external"
// generate:reset
type Root struct {
	Value int
	Node external.Holder[*external.Node]
	Manual external.Holder[*Manual]
}
type Manual struct { Calls int }
func (m *Manual) Reset() { m.Calls++ }
var _ external.ArrayOnly[*external.Wrapper]
var _ external.Unused[*external.Wrapper]
`)
			writeFixtureFile(t, dir, "source_test.go", `package fixture
import (
	"testing"
	external "example.com/reset-external"
)
func TestCompatibleExternalProtocol(t *testing.T) {
	n, m := &external.Node{Number: 7}, &Manual{}
	r := Root{
		Value: 3,
		Node: external.Holder[*external.Node]{Value: n},
		Manual: external.Holder[*Manual]{Value: m},
	}
	r.Reset()
	if r.Value != 0 || n.Number != 0 || m.Calls != 1 || r.Node.Value != n || r.Manual.Value != m {
		t.Fatal("внешний контейнер потерял вызов Reset или заменил ссылки")
	}
}
`)
			runFixtureGenerator(t, dir)
			testFixtureModule(t, dir)
			writeFixtureFile(t, dir, "adapter.go", `package fixture
import external "example.com/reset-external"
type TypedAdapted struct { *external.Node; Calls int }
func (a *TypedAdapted) ResetWithVisited(visited map[interface{}]struct{}, original interface { Reset() }, _ ...*TypedAdapted) {
	if a != nil {
		a.Calls++
		a.Node.ResetWithVisited(visited, a.Node)
	}
}
var _ external.Holder[*TypedAdapted]
`)
			testFixtureModule(t, dir)
			if protocol == "legacy" {
				assertGenericWrapperRejected(t, dir, binary, "TypedAdapted", "ResetWithVisited", "example.com/reset-external")
				writeFixtureFile(t, dir, "adapter.go", "package fixture\n")
			} else {
				writeFixtureFile(t, dir, "adapter_test.go", `package fixture
import (
	"testing"
	external "example.com/reset-external"
)
func TestTypedExternalAdapter(t *testing.T) {
	n := &external.Node{Number: 7}
	a := &TypedAdapted{Node: n}
	h := external.Holder[*TypedAdapted]{Value: a}
	h.Reset()
	if n.Number != 0 || a.Calls != 1 || h.Value != a || a.Node != n {
		t.Fatal("внешний контейнер не вызвал типизированный адаптер или заменил ссылки")
	}
}
`)
				runFixtureGenerator(t, dir)
				testFixtureModule(t, dir, "-race", "-timeout=20s")
				assertFixtureGeneratorIdempotent(t, dir)
			}
			if protocol == "legacy" {
				writeFixtureFile(t, dir, "source.go", `package fixture
import external "example.com/reset-external"
// generate:reset
type Root struct { Value int; Back external.Holder[*Root] }
`)
				writeFixtureFile(t, dir, "source_test.go", "package fixture\n")
				assertGenericWrapperRejected(t, dir, binary, "Root", "ResetWithVisited", "перегенерируйте", "example.com/reset-external")
			}
			if !bytes.Equal(external, readFixtureFile(t, dir, "external/"+genFile)) {
				t.Fatal("изменён generated-файл внешнего модуля")
			}
		})
	}
}

func assertGenericWrapperRejected(t *testing.T, dir, binary string, want ...string) {
	t.Helper()
	if len(want) == 0 {
		want = []string{"Wrapper", "ResetWithVisited"}
	}
	before := snapshotFixtureFiles(t, dir)
	command := fixtureCommand{dir: dir, timeout: 30 * time.Second, name: binary}
	output, err, contextErr := executeFixtureCommand(command)
	if contextErr != nil {
		t.Errorf("генератор не завершил проверку обёртки: %v", contextErr)
	} else if err == nil {
		t.Errorf("генератор принял небезопасную generic-инстанциацию:\n%s", output)
	} else {
		for _, want := range want {
			if !strings.Contains(string(output), want) {
				t.Errorf("ошибка не содержит %q:\n%s", want, output)
			}
		}
	}
	assertFixtureFilesUnchanged(t, dir, before)
}

const legacyResetValue = `func resetValue(value interface{ Reset() }, visited map[interface{}]struct{}) {
	if value == nil {
		return
	}
	v := resetReflect.ValueOf(value)
	switch v.Kind() {
	case resetReflect.Chan, resetReflect.Func, resetReflect.Map, resetReflect.Pointer, resetReflect.Slice:
		if v.IsNil() {
			return
		}
	}
	if resetter, ok := value.(interface {
		ResetWithVisited(map[interface{}]struct{}, interface{ Reset() })
	}); ok {
		resetter.ResetWithVisited(visited, value)
		return
	}
	value.Reset()
}
`

const genericWrapperSource = `package fixture

type Probe struct { Calls int }
func (p *Probe) Reset() { p.Calls++ }

// generate:reset
type Node struct {
	Number int
	Probe Probe
	Next *Node
	Adapted *Holder[*Adapted]
	TypedAdapted *Holder[*TypedAdapted]
	Marked *Holder[*Marked]
}

// generate:reset
type Holder[T interface { Reset() }] struct { Value T }

type Wrapper struct { *Node; Items []int }

// generate:reset
type Ordinary struct { Value Wrapper }
// generate:reset
type Unused[T interface { Reset() }] struct { Number int }
// generate:reset
type ArrayOnly[T interface { Reset() }] struct { Values [1]T }
// generate:reset
type Unconstrained[T any] struct { Value T }

func localShadow() {
	type Holder[T interface { Reset() }] struct { Value [1]T }
	var _ Holder[*Wrapper]
}

type Adapted struct { *Node }
func (a *Adapted) ResetWithVisited(visited map[interface{}]struct{}, original interface { Reset() }) {
	if a != nil { a.Node.ResetWithVisited(visited, a.Node) }
}
type OuterAdapted struct { *Adapted }

type TypedAdapted struct { *Node }
func (a *TypedAdapted) ResetWithVisited(visited map[interface{}]struct{}, original interface { Reset() }, _ ...*TypedAdapted) {
	if a != nil { a.Node.ResetWithVisited(visited, a.Node) }
}

// generate:reset
type Marked struct { *Node; Items []int }

type Manual struct { *Node; Calls int }
func (m *Manual) Reset() { m.Calls++ }
type OuterManual struct { *Manual }
type ManualValue struct { *Node; Calls *int }
func (m ManualValue) Reset() { *m.Calls++ }
type GenericManual[T any] struct { *Node; Value T; Calls int }
func (m *GenericManual[T]) Reset() { m.Calls++ }
`

const genericWrapperTests = `package fixture

import "testing"

func TestUnusedAndNonDispatchingParameters(t *testing.T) {
	n := &Node{Number: 7}
	w := &Wrapper{Node: n, Items: []int{3}}
	u := Unused[*Wrapper]{Number: 7}
	u.Reset()
	a := ArrayOnly[*Wrapper]{Values: [1]*Wrapper{w}}
	a.Reset()
	z := Unconstrained[*Wrapper]{Value: w}
	z.Reset()
	o := Ordinary{Value: *w}
	o.Reset()
	if u.Number != 0 || a.Values[0] != nil || z.Value != nil || o.Value.Node != nil || n.Number != 7 {
		t.Fatal("неправильно сброшены поля без generic-вызова Reset")
	}
	if len(w.Items) != 1 { t.Fatal("изменена неразмеченная обёртка") }
}

func TestExplicitAdapterCycle(t *testing.T) {
	n := &Node{Number: 7}
	a := &Adapted{Node: n}
	n.Adapted = &Holder[*Adapted]{Value: a}
	n.Next = n
	n.Reset()
	if n.Number != 0 || n.Probe.Calls != 1 || n.Next != n || n.Adapted.Value != a {
		t.Fatal("адаптер потерял карту посещений или заменил ссылки")
	}
	n.Number = 9
	outer := Holder[*OuterAdapted]{Value: &OuterAdapted{Adapted: a}}
	outer.Reset()
	if n.Number != 0 || n.Probe.Calls != 2 { t.Fatal("унаследованный пользовательский адаптер потерял карту") }
}

func TestTypedAdapterCycle(t *testing.T) {
	n := &Node{Number: 7}
	a := &TypedAdapted{Node: n}
	h := &Holder[*TypedAdapted]{Value: a}
	n.TypedAdapted = h
	n.Next = n
	n.Reset()
	if n.Number != 0 || n.Probe.Calls != 1 || n.Next != n || n.TypedAdapted != h || h.Value != a || a.Node != n {
		t.Fatal("типизированный адаптер потерял карту посещений или заменил ссылки")
	}
}

func TestMarkedWrapperCycle(t *testing.T) {
	n := &Node{Number: 7}
	w := &Marked{Node: n, Items: []int{3}}
	n.Marked = &Holder[*Marked]{Value: w}
	n.Reset()
	if n.Number != 0 || n.Probe.Calls != 1 || w.Node != n || len(w.Items) != 0 || cap(w.Items) != 1 {
		t.Fatal("размеченная обёртка потеряла карту посещений")
	}
}

func TestManualOverrides(t *testing.T) {
	n := &Node{Number: 7}
	m := &Manual{Node: n}
	h := Holder[*Manual]{Value: m}
	h.Reset()
	o := Holder[*OuterManual]{Value: &OuterManual{Manual: m}}
	o.Reset()
	calls := 0
	v := Holder[ManualValue]{Value: ManualValue{Node: n, Calls: &calls}}
	v.Reset()
	g := &GenericManual[string]{Node: n, Value: "keep"}
	gh := Holder[*GenericManual[string]]{Value: g}
	gh.Reset()
	if m.Calls != 2 || calls != 1 || g.Calls != 1 || g.Value != "keep" || n.Number != 7 || n.Probe.Calls != 0 {
		t.Fatal("пользовательский Reset заменён унаследованным протоколом")
	}
}

func TestDynamicInterfaceBoundary(t *testing.T) {
	n := &Node{Number: 7}
	h := Holder[interface { Reset() }]{Value: &Wrapper{Node: n}}
	h.Reset()
	if n.Number != 0 || n.Probe.Calls != 1 { t.Fatal("динамическое interface-значение не сброшено") }
}
`
