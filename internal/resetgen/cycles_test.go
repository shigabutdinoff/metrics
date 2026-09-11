package resetgen

import (
	"bytes"
	"testing"
)

func TestGeneratorCycles(t *testing.T) {
	dir := fixtureModule(t, map[string]string{
		"source.go":      cycleSource,
		"source_test.go": cycleTests,
		"child/source.go": `package child
// generate:reset
type Box[T interface { Reset() }] struct { Value T; Items []int }
`,
	})
	for _, layout := range []string{"packages", "external module"} {
		t.Run(layout, func(t *testing.T) {
			var external []byte
			if layout == "external module" {
				external = readFixtureFile(t, dir, "child/"+genFile)
				writeFixtureFile(t, dir, "go.mod", "module example.com/reset-regression\n\ngo 1.25.8\n\nrequire example.com/reset-regression/child v0.0.0\n\nreplace example.com/reset-regression/child => ./child\n")
				writeFixtureFile(t, dir, "child/go.mod", "module example.com/reset-regression/child\n\ngo 1.25.8\n")
			}
			runFixtureGenerator(t, dir)
			child := readFixtureFile(t, dir, "child/"+genFile)
			if external != nil && !bytes.Equal(external, child) {
				t.Fatal("изменён generated-файл внешнего модуля")
			}
			testFixtureModuleRace(t, dir)
			assertFixtureGeneratorIdempotent(t, dir)
		})
	}
}

const cycleSource = `package fixture

import "example.com/reset-regression/child"

type Probe struct { Calls int }
func (p *Probe) Reset() { p.Calls++ }

// generate:reset
type Node struct {
	Number int
	Items []int
	Entries map[string]int
	Probe Probe
	Next *Node
	Other *Node
}

type NodePointer *Node
type NodePointers *NodePointer

// generate:reset
type Pointers struct { First NodePointer; Second NodePointers; Direct **Node }

// generate:reset
type GenericNode[T any] struct { Value T; Next *GenericNode[T] }

// generate:reset
type Root struct { Box child.Box[*Root]; Number int; Probe Probe }

// generate:reset
type Holder[T interface { Reset() }] struct { Value T }

type Manual struct { *Node; Calls int }
func (m *Manual) Reset() { m.Calls++ }

type Inner struct { *Node }
type NestedManual struct { *Inner; Calls int }
func (m *NestedManual) Reset() { m.Calls++ }

// generate:reset
type ManualFields struct { Value NestedManual; Pointer *NestedManual }

// generate:reset
type OwnedGeneric[T any] struct {
	*Inner
	Value T
	Probe Probe
	Next *Holder[*OwnedGeneric[T]]
}

type WrongParameters struct { Calls int }
func (m *WrongParameters) Reset() { m.Calls++ }
func (*WrongParameters) ResetWithVisited(int, string, ...*WrongParameters) { panic("несовместимый протокол") }

type WrongResult struct { Calls int }
func (m *WrongResult) Reset() { m.Calls++ }
func (*WrongResult) ResetWithVisited(map[interface{}]struct{}, interface { Reset() }, ...*WrongResult) int {
	panic("несовместимый результат протокола")
}

type ManualSlice []int
func (s ManualSlice) Reset() { s[0]++ }

// generate:reset
type AdapterNode struct {
	Number int
	Probe Probe
	Back ValueAdapter
	Pointer *ValueAdapter
	Alias ValueAdapterAlias
	Generic *Holder[ValueAdapter]
	Mixed MixedAdapter
}

type ValueAdapter struct { Node *AdapterNode; Calls *int }
type ValueAdapterAlias = ValueAdapter
func (a ValueAdapter) Reset() { a.Node.Reset() }
func (a ValueAdapter) ResetWithVisited(visited map[interface{}]struct{}, original interface { Reset() }, _ ...ValueAdapter) {
	if original != a { panic("потерян value receiver адаптера") }
	if a.Node != nil {
		*a.Calls++
		a.Node.ResetWithVisited(visited, a.Node)
	}
}

type MixedAdapter struct { Node *AdapterNode; Calls *int }
func (a MixedAdapter) Reset() { a.Node.Reset() }
func (a *MixedAdapter) ResetWithVisited(visited map[interface{}]struct{}, original interface { Reset() }, _ ...*MixedAdapter) {
	if original != a { panic("потерян pointer receiver адаптера") }
	if a.Node != nil {
		*a.Calls++
		a.Node.ResetWithVisited(visited, a.Node)
	}
}
`

const cycleTests = `package fixture

import (
	"runtime/debug"
	"sync"
	"testing"
)

func TestSelfCycle(t *testing.T) {
	previous := debug.SetMaxStack(1 << 20)
	defer debug.SetMaxStack(previous)
	items := make([]int, 2, 4)
	entries := map[string]int{"old": 3}
	n := &Node{Number: 7, Items: items, Entries: entries}
	n.Next = n
	n.Reset()
	if n.Next != n || n.Number != 0 || n.Probe.Calls != 1 { t.Fatal("не сброшена самоссылка") }
	if len(n.Items) != 0 || cap(n.Items) != cap(items) || &n.Items[:1][0] != &items[0] {
		t.Fatal("изменён backing array")
	}
	if n.Entries == nil || len(n.Entries) != 0 { t.Fatal("мапа не очищена") }
	entries["new"] = 4
	if n.Entries["new"] != 4 { t.Fatal("мапа заменена") }
	n.Number = 9
	n.Reset()
	if n.Number != 0 || n.Probe.Calls != 2 { t.Fatal("повторный Reset пропустил объект") }
}

func TestMutualCycleAndSharedChild(t *testing.T) {
	first, second := &Node{Number: 1}, &Node{Number: 2}
	first.Next, first.Other, second.Next = second, second, first
	first.Reset()
	if first.Next != second || first.Other != second || second.Next != first {
		t.Fatal("заменены циклические ссылки")
	}
	if first.Number != 0 || second.Number != 0 || first.Probe.Calls != 1 || second.Probe.Calls != 1 {
		t.Fatal("объект пропущен или посещён повторно")
	}
}

func TestNamedPointerChains(t *testing.T) {
	n := &Node{Number: 3}
	n.Next = n
	alias := NodePointer(n)
	p := Pointers{First: alias, Second: NodePointers(&alias), Direct: &n}
	p.Reset()
	if p.First != alias || p.Second != NodePointers(&alias) || p.Direct != &n || n.Next != n || n.Probe.Calls != 1 {
		t.Fatal("не сохранены указатели или объект сброшен повторно")
	}
}

func TestGenericCycles(t *testing.T) {
	n := &GenericNode[string]{Value: "value"}
	n.Next = n
	n.Reset()
	if n.Next != n || n.Value != "" { t.Fatal("не сброшен generic-цикл") }
	r := &Root{Number: 3}
	r.Box.Value = r
	r.Box.Items = []int{1, 2}
	r.Reset()
	if r.Box.Value != r || r.Number != 0 || r.Probe.Calls != 1 || len(r.Box.Items) != 0 || cap(r.Box.Items) != 2 {
		t.Fatal("не сброшен generic-цикл между пакетами")
	}
}

func TestManualResetWithPromotedProtocol(t *testing.T) {
	for _, embedded := range []*Node{nil, {Number: 8}} {
		manual := &Manual{Node: embedded}
		h := Holder[*Manual]{Value: manual}
		h.Reset()
		if manual.Calls != 1 || h.Value != manual { t.Fatal("пользовательский Reset обойдён") }
		if embedded != nil && (embedded.Number != 8 || embedded.Probe.Calls != 0) {
			t.Fatal("вызван унаследованный служебный метод вместо пользовательского Reset")
		}
	}
	slice := ManualSlice{1}
	h := Holder[ManualSlice]{Value: slice}
	h.Reset()
	if slice[0] != 2 { t.Fatal("не вызван Reset несравнимого значения") }
}

func TestManualResetWithNilEmbeddingPath(t *testing.T) {
	for _, tc := range []struct {
		name string
		inner *Inner
	}{
		{name: "nil intermediate"},
		{name: "nil node", inner: &Inner{}},
		{name: "populated", inner: &Inner{Node: &Node{Number: 8}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, dispatch := range []string{"ordinary", "generic", "interface"} {
				t.Run(dispatch, func(t *testing.T) {
					manual := &NestedManual{Inner: tc.inner}
					switch dispatch {
					case "ordinary":
						fields := ManualFields{Value: NestedManual{Inner: tc.inner}, Pointer: manual}
						fields.Reset()
						if fields.Value.Calls != 1 || fields.Value.Inner != tc.inner || fields.Pointer != manual {
							t.Fatal("обычное поле потеряло пользовательский Reset или ссылки")
						}
					case "generic":
						h := Holder[*NestedManual]{Value: manual}
						h.Reset()
						if h.Value != manual { t.Fatal("заменено generic-поле") }
					case "interface":
						h := Holder[interface { Reset() }]{Value: manual}
						h.Reset()
						if h.Value != manual { t.Fatal("заменено interface-поле") }
					}
					if manual.Calls != 1 || manual.Inner != tc.inner {
						t.Fatal("пользовательский Reset обойдён или заменено вложение")
					}
					if tc.inner != nil && tc.inner.Node != nil && (tc.inner.Number != 8 || tc.inner.Probe.Calls != 0) {
						t.Fatal("вызван унаследованный протокол вместо пользовательского Reset")
					}
				})
			}
		})
	}
}

func TestOwnedGenericProtocolWithNilEmbedding(t *testing.T) {
	n := &OwnedGeneric[int]{Value: 7}
	n.Next = &Holder[*OwnedGeneric[int]]{Value: n}
	n.Reset()
	if n.Inner != nil || n.Value != 0 || n.Probe.Calls != 1 || n.Next.Value != n {
		t.Fatal("собственный generic-протокол потерял карту посещений или изменил ссылки")
	}
}

func TestIncompatibleTraversalProtocol(t *testing.T) {
	parameters, result := &WrongParameters{}, &WrongResult{}
	for _, value := range []interface { Reset() }{parameters, result} {
		h := Holder[interface { Reset() }]{Value: value}
		h.Reset()
	}
	if parameters.Calls != 1 || result.Calls != 1 { t.Fatal("не вызван пользовательский Reset") }
}

func TestValueAdapterCycles(t *testing.T) {
	previous := debug.SetMaxStack(1 << 20)
	defer debug.SetMaxStack(previous)
	for _, dispatch := range []string{"ordinary", "pointer", "alias", "generic", "mixed receiver"} {
		t.Run(dispatch, func(t *testing.T) {
			n := &AdapterNode{Number: 7}
			calls := 0
			a := ValueAdapter{Node: n, Calls: &calls}
			switch dispatch {
			case "ordinary": n.Back = a
			case "pointer": n.Pointer = &a
			case "alias": n.Alias = a
			case "generic": n.Generic = &Holder[ValueAdapter]{Value: a}
			case "mixed receiver": n.Mixed = MixedAdapter{Node: n, Calls: &calls}
			}
			back, pointer, alias, generic, mixed := n.Back, n.Pointer, n.Alias, n.Generic, n.Mixed
			for want := 1; want <= 2; want++ {
				n.Number = 7
				n.Reset()
				if n.Number != 0 || n.Probe.Calls != want || calls != want {
					t.Fatalf("адаптер потерял общую карту: number=%d, visits=%d, calls=%d", n.Number, n.Probe.Calls, calls)
				}
				if n.Back != back || n.Pointer != pointer || n.Alias != alias || n.Generic != generic || n.Mixed != mixed ||
					(n.Pointer != nil && *n.Pointer != a) || (n.Generic != nil && n.Generic.Value != a) {
					t.Fatal("адаптер заменил циклическую ссылку")
				}
			}
		})
	}
}

func TestNilAndConcurrentGraphs(t *testing.T) {
	var n *Node
	n.Reset()
	h := Holder[*Node]{}
	h.Reset()
	var empty Node
	empty.Reset()
	if empty.Next != nil || empty.Items != nil || empty.Entries != nil { t.Fatal("изменено nil-поле") }
	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			n := &Node{Number: 1}
			n.Next = n
			n.Reset()
			if n.Number != 0 || n.Probe.Calls != 1 { t.Error("пересеклись независимые обходы") }
		}()
	}
	group.Wait()
}

func TestTraversalProtocol(t *testing.T) {
	var absent *Node
	absent.ResetWithVisited(nil, absent)
	absent.ResetWithVisited(nil, nil)
	n := &Node{Number: 1}
	n.Next = n
	n.ResetWithVisited(nil, n)
	if n.Number != 0 || n.Probe.Calls != 1 { t.Fatal("nil-карта не инициализирована") }
	seen := map[interface{}]struct{}{}
	n.ResetWithVisited(seen, n)
	n.Number = 3
	n.ResetWithVisited(seen, n)
	if n.Number != 3 || n.Probe.Calls != 2 { t.Fatal("общая карта не остановила повторный обход") }
	slice := ManualSlice{1}
	absent.ResetWithVisited(nil, slice)
	if slice[0] != 2 { t.Fatal("несовпадающий receiver обошёл пользовательский Reset") }
}
`
