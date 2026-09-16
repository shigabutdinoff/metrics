package resetgen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var resetTestBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "reset-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	resetTestBinary = filepath.Join(dir, "reset")
	command := fixtureCommand{name: "go", args: []string{"build", "-o", resetTestBinary, "../../cmd/reset"}}
	if output, err, _ := executeFixtureCommand(command); err != nil {
		fmt.Fprintf(os.Stderr, "go build: %v\n%s", err, output)
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			fmt.Fprintln(os.Stderr, removeErr)
		}
		os.Exit(1)
	}
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil && code == 0 {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func TestGeneratorRegression(t *testing.T) {
	const genFile = "reset.gen.go"
	binary := resetTestBinary

	for _, change := range []string{"remove marker", "remove struct", "remove source"} {
		t.Run(change, func(t *testing.T) {
			const source = "package sample\n\n// generate:reset\ntype Sample struct { Value int }\n"
			dir := fixtureModule(t, map[string]string{
				"root.go":          "package fixture\n",
				"sample/source.go": source,
			})
			runFixtureGenerator(t, dir)
			generated := filepath.Join(dir, "sample", genFile)
			if _, err := os.Stat(generated); err != nil {
				t.Fatal(err)
			}
			testFixtureModule(t, dir)

			switch change {
			case "remove marker":
				writeFixtureFile(t, dir, "sample/source.go", strings.ReplaceAll(source, "// generate:reset\n", ""))
			case "remove struct":
				writeFixtureFile(t, dir, "sample/source.go", "package sample\n")
			case "remove source":
				if err := os.Remove(filepath.Join(dir, "sample", "source.go")); err != nil {
					t.Fatal(err)
				}
			}

			runFixtureGenerator(t, dir)
			if _, err := os.Stat(generated); !os.IsNotExist(err) {
				t.Fatalf("устаревший файл не удалён: %v", err)
			}
			runFixtureGenerator(t, dir)
			testFixtureModule(t, dir)
		})
	}

	t.Run("preserve unowned file", func(t *testing.T) {
		const source = "package fixture\n\nfunc answer() int { return 42 }\n"
		dir := fixtureModule(t, map[string]string{
			"root.go": "package fixture\n",
			genFile:   source,
			"root_test.go": `package fixture
import "testing"
func TestAnswer(t *testing.T) {
	if answer() != 42 { t.Fatal("изменён пользовательский файл") }
}
`,
		})
		runFixtureGenerator(t, dir)
		if got := readFixtureFile(t, dir, genFile); string(got) != source {
			t.Fatalf("изменён пользовательский файл:\n%s", got)
		}
		testFixtureModule(t, dir)
	})

	t.Run("parse errors keep outputs", func(t *testing.T) {
		for _, broken := range []string{
			"package sample\nfunc broken(\n\n// generate:reset\ntype Sample struct { Value int }\n",
			"packag sample\n\n// generate:reset\ntype Sample struct { Value int }\n",
		} {
			dir := fixtureModule(t, map[string]string{
				"source.go":        "package fixture\n\n// generate:reset\ntype Root struct { Value int }\n",
				"sample/source.go": "package sample\n\n// generate:reset\ntype Sample struct { Value int }\n",
			})
			runFixtureGenerator(t, dir)
			root := readFixtureFile(t, dir, genFile)
			sample := readFixtureFile(t, dir, "sample/"+genFile)
			writeFixtureFile(t, dir, "source.go", "package fixture\n\n// generate:reset\ntype Root struct { Other string }\n")
			writeFixtureFile(t, dir, "sample/source.go", broken)

			if output, err, _ := executeFixtureCommand(fixtureCommand{dir: dir, name: binary}); err == nil {
				t.Fatalf("генератор не сообщил об ошибке разбора:\n%s", output)
			}
			if !bytes.Equal(root, readFixtureFile(t, dir, genFile)) ||
				!bytes.Equal(sample, readFixtureFile(t, dir, "sample/"+genFile)) {
				t.Fatal("ошибка разбора привела к изменению файлов")
			}
		}
	})

	t.Run("package list error keeps all outputs", func(t *testing.T) {
		dir := fixtureModule(t, map[string]string{
			"root.go":            "package fixture\n",
			"sample/source.go":   "package sample\n\n// generate:reset\ntype Sample struct { Value int }\n",
			"existing/source.go": "package existing\n\n// generate:reset\ntype Existing struct { Value int }\n",
			"obsolete/source.go": "package obsolete\n\n// generate:reset\ntype Obsolete struct { Value int }\n",
			"new/source.go":      "package fresh\n",
		})
		runFixtureGenerator(t, dir)
		testFixtureModule(t, dir)
		for _, name := range []string{"sample/", "existing/", "obsolete/"} {
			readFixtureFile(t, dir, name+genFile)
		}

		writeFixtureFile(t, dir, "sample/aaa.go", "package other\n")
		writeFixtureFile(t, dir, "existing/source.go", "package existing\n\n// generate:reset\ntype Existing struct { Value int; Other string }\n")
		writeFixtureFile(t, dir, "obsolete/source.go", "package obsolete\n\ntype Obsolete struct { Value int }\n")
		writeFixtureFile(t, dir, "new/source.go", "package fresh\n\n// generate:reset\ntype Fresh struct { Value int }\n")
		before := snapshotFixtureFiles(t, dir)

		output, err, _ := executeFixtureCommand(fixtureCommand{dir: dir, name: binary})
		if err == nil {
			t.Errorf("генератор не сообщил об ошибке загрузки пакета:\n%s", output)
		} else {
			for _, want := range []string{"found packages", "aaa.go"} {
				if !strings.Contains(string(output), want) {
					t.Errorf("ошибка не содержит %q:\n%s", want, output)
				}
			}
		}
		assertFixtureFilesUnchanged(t, dir, before)
	})

	for _, tc := range []struct {
		name   string
		source string
		tests  string
	}{
		{"import and local name collisions", collisionSource, collisionTests},
		{"generic structs", genericSource, genericTests},
		{"generic Reset constraints", genericResetSource, genericResetTests},
		{"named pointers", pointerSource, pointerTests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixtureModule(t, map[string]string{
				"source.go":      tc.source,
				"source_test.go": tc.tests,
			})
			runFixtureGenerator(t, dir)
			testFixtureModule(t, dir)
			assertFixtureGeneratorIdempotent(t, dir)
		})
	}
}

const collisionSource = `package fixture

import (
	htmltemplate "html/template"
	texttemplate "text/template"
)

type template struct { Value int }
type template1 struct { Value int }
type b struct { Value int }

// generate:reset
type Box struct {
	HTML htmltemplate.Template
	Text texttemplate.Template
	Local template
	Other template1
	Receiver b
	Array [1]texttemplate.Template
}
`

const collisionTests = `package fixture

import (
	htmltemplate "html/template"
	"reflect"
	texttemplate "text/template"
	"testing"
)

func TestReset(t *testing.T) {
	box := Box{
		HTML: *htmltemplate.New("html"), Text: *texttemplate.New("text"),
		Local: template{7}, Other: template1{8}, Receiver: b{9},
		Array: [1]texttemplate.Template{*texttemplate.New("array")},
	}
	box.Reset()
	if !reflect.DeepEqual(box, Box{}) { t.Fatalf("Reset оставил значения: %#v", box) }
}
`

const genericSource = `package fixture

type Value[T any] = *T

// generate:reset
type Child[T any] struct {
	Value T
	Items []T
}

// generate:reset
type Box[b any, nil comparable, clear any] struct {
	Value b
	Alias Value[b]
	Key nil
	Extra clear
	Ptr *b
	Slice []b
	Map map[nil]clear
	Child Child[b]
	ChildPtr *Child[b]
	Array [2]b
}
`

const genericTests = `package fixture

import "testing"

func TestReset(t *testing.T) {
	checkBox(t, 42, "number", struct{ N int }{7})
	checkBox(t, "value", 2, true)
	checkBox(t, struct{ N int }{9}, "struct", 3)
	n := 12
	checkBox(t, &n, "pointer", 4)
}

func checkBox[V comparable, K comparable, W comparable](t *testing.T, value V, key K, extra W) {
	t.Helper()
	alias := value
	pointed := value
	items := make([]V, 2, 4)
	items[0] = value
	entries := map[K]W{key: extra}
	child := &Child[V]{Value: value, Items: []V{value}}
	childItems := child.Items
	box := Box[V, K, W]{
		Value: value, Alias: &alias, Key: key, Extra: extra,
		Ptr: &pointed, Slice: items, Map: entries,
		Child: Child[V]{Value: value, Items: []V{value}}, ChildPtr: child,
		Array: [2]V{value, value},
	}
	box.Reset()
	var zeroV V
	var zeroK K
	var zeroW W
	if box.Value != zeroV || alias != zeroV || box.Key != zeroK || box.Extra != zeroW || box.Array != [2]V{} {
		t.Fatalf("параметризованные поля не обнулены: %#v", box)
	}
	if box.Alias != &alias { t.Fatal("изменён указатель через generic alias") }
	if box.Ptr != &pointed || pointed != zeroV { t.Fatal("изменён указатель или не сброшено его значение") }
	if len(box.Slice) != 0 || cap(box.Slice) != cap(items) || &box.Slice[:1][0] != &items[0] {
		t.Fatal("слайс не очищен с сохранением backing array")
	}
	if box.Map == nil || len(box.Map) != 0 { t.Fatal("мапа не очищена") }
	entries[key] = extra
	if got, ok := box.Map[key]; !ok || got != extra { t.Fatal("мапа была заменена") }
	if box.Child.Value != zeroV || len(box.Child.Items) != 0 || cap(box.Child.Items) != 1 {
		t.Fatal("вложенная структура не сброшена через Reset")
	}
	if box.ChildPtr != child || child.Value != zeroV || len(child.Items) != 0 || &child.Items[:1][0] != &childItems[0] {
		t.Fatal("указатель на вложенную структуру сброшен неправильно")
	}
	var empty Box[V, K, W]
	empty.Reset()
	if empty.Alias != nil || empty.Ptr != nil || empty.Slice != nil || empty.Map != nil || empty.ChildPtr != nil {
		t.Fatal("Reset изменил nil-поля")
	}
	var absent *Box[V, K, W]
	absent.Reset()
}
`

const genericResetSource = `package fixture

import resetValue "time"

var _ resetValue.Time
var resetReflect = 1

type Resetter interface { Reset() }
type EmbeddedResetter interface { Resetter }
type Value[T any] = *T

// generate:reset
type Box[b interface { Reset() }, nil Resetter, clear EmbeddedResetter] struct {
	Value b
	Alias Value[b]
	Pointer *b
	Named nil
	Embedded clear
}

// generate:reset
type Related[T any, R interface { Reset(); Put(T) }] struct {
	Value R
	Plain T
}

// generate:reset
type Single[T Resetter] struct { Value T }

// generate:reset
type Fallback[A any, B interface { Reset(int) }, C interface { Reset() int }] struct {
	Plain A
	Args B
	Result C
	Interface Resetter
}

type WithArgs struct{}
func (*WithArgs) Reset(int) { panic("неожиданный вызов Reset(int)") }

type WithResult struct{}
func (*WithResult) Reset() int { panic("неожиданный вызов Reset() int") }

type Store[T any] struct { Value T; Calls int }
func (s *Store[T]) Reset() { var zero T; s.Value = zero; s.Calls++ }
func (s *Store[T]) Put(value T) { s.Value = value }

var resetCalls = make(map[string]int)

type ResetSlice []int
func (s ResetSlice) Reset() {
	if s == nil { panic("Reset вызван для nil-слайса") }
	resetCalls["slice"]++
	clear(s)
}

type ResetMap map[string]int
func (m ResetMap) Reset() {
	if m == nil { panic("Reset вызван для nil-мапы") }
	resetCalls["map"]++
	clear(m)
}

type ResetChan chan int
func (c ResetChan) Reset() {
	if c == nil { panic("Reset вызван для nil-канала") }
	resetCalls["chan"]++
}

type ResetFunc func()
func (f ResetFunc) Reset() {
	if f == nil { panic("Reset вызван для nil-функции") }
	f()
}

type ResetScalar int
func (ResetScalar) Reset() { resetCalls["scalar"]++ }

type ResetStruct struct{}
func (ResetStruct) Reset() { resetCalls["struct"]++ }

type ResetRecord struct { Items []int }
func (r ResetRecord) Reset() { resetCalls["record"]++; clear(r.Items) }
`

const genericResetTests = `package fixture

import (
	"bytes"
	"reflect"
	"testing"
)

func TestReset(t *testing.T) {
	value := bytes.NewBufferString("value")
	alias := bytes.NewBufferString("alias")
	originalAlias := alias
	pointed := bytes.NewBufferString("pointer")
	pointer := pointed
	named := bytes.NewBufferString("named")
	embedded := bytes.NewBufferString("embedded")
	box := Box[*bytes.Buffer, *bytes.Buffer, *bytes.Buffer]{value, &alias, &pointer, named, embedded}
	box.Reset()
	if box.Alias != &alias { t.Fatal("Reset заменил указатель через generic alias") }
	for _, pair := range [][2]*bytes.Buffer{
		{box.Value, value}, {*box.Alias, originalAlias}, {pointer, pointed},
		{box.Named, named}, {box.Embedded, embedded},
	} {
		if pair[0] != pair[1] { t.Fatal("Reset заменил указатель на буфер") }
		if pair[1].Len() != 0 { t.Fatal("Reset не очистил исходный буфер") }
	}
	if box.Pointer != &pointer { t.Fatal("Reset заменил указатель на параметризованное поле") }
	box.Pointer = nil
	box.Reset()
	if box.Pointer != nil { t.Fatal("Reset изменил nil-указатель") }
	var absent *Box[*bytes.Buffer, *bytes.Buffer, *bytes.Buffer]
	absent.Reset()

	store := &Store[int]{Value: 42}
	related := Related[int, *Store[int]]{Value: store, Plain: 7}
	related.Reset()
	if related.Value != store || store.Value != 0 || store.Calls != 1 || related.Plain != 0 {
		t.Fatal("Reset не сохранил ограничение со ссылкой на другой параметр типа")
	}

	untouched := bytes.NewBufferString("keep")
	fallback := Fallback[*bytes.Buffer, *WithArgs, *WithResult]{untouched, &WithArgs{}, &WithResult{}, untouched}
	fallback.Reset()
	if fallback.Plain != nil || fallback.Args != nil || fallback.Result != nil || fallback.Interface != nil {
		t.Fatal("Reset не обнулил поля без гарантированного совместимого метода")
	}
	if untouched.String() != "keep" { t.Fatal("Reset вызвал метод, отсутствующий в ограничении") }
}

func TestResetGenericNil(t *testing.T) {
	t.Run("zero pointer and alias", func(t *testing.T) {
		var box Box[*bytes.Buffer, *bytes.Buffer, *bytes.Buffer]
		box.Reset()
		if box.Value != nil || box.Alias != nil || box.Pointer != nil || box.Named != nil || box.Embedded != nil {
			t.Fatal("Reset изменил nil-поля")
		}
	})
	t.Run("pointer to nil parameter", func(t *testing.T) {
		var value *bytes.Buffer
		box := Box[*bytes.Buffer, *bytes.Buffer, *bytes.Buffer]{Alias: &value, Pointer: &value}
		box.Reset()
		if box.Alias != &value || box.Pointer != &value || value != nil { t.Fatal("Reset изменил указатель на nil-параметр") }
	})
	t.Run("nil interface", func(t *testing.T) {
		var value Resetter
		box := Box[Resetter, Resetter, Resetter]{Pointer: &value}
		box.Reset()
		if box.Value != nil || box.Alias != nil || box.Pointer != &value || value != nil || box.Named != nil || box.Embedded != nil {
			t.Fatal("Reset изменил nil-интерфейс")
		}
	})
	t.Run("interface containing nil pointer", func(t *testing.T) {
		var pointer *bytes.Buffer
		var value Resetter = pointer
		box := Box[Resetter, Resetter, Resetter]{value, &value, &value, value, value}
		box.Reset()
		if box.Alias != &value { t.Fatal("Reset заменил указатель через generic alias") }
		for _, field := range []Resetter{box.Value, *box.Alias, *box.Pointer, box.Named, box.Embedded} {
			got, ok := field.(*bytes.Buffer)
			if !ok || got != nil { t.Fatal("Reset заменил типизированный nil внутри интерфейса") }
		}
		if box.Pointer != &value { t.Fatal("Reset заменил указатель на интерфейс") }
	})
	t.Run("interface containing buffer", func(t *testing.T) {
		buffer := bytes.NewBufferString("value")
		box := Single[Resetter]{Value: buffer}
		box.Reset()
		if box.Value != buffer || buffer.Len() != 0 { t.Fatal("Reset не сбросил значение внутри интерфейса") }
	})
}

func TestResetGenericNilableKinds(t *testing.T) {
	checkNilable(t, "slice", ResetSlice(nil), ResetSlice{1})
	checkNilable(t, "map", ResetMap(nil), ResetMap{"value": 1})
	checkNilable(t, "chan", ResetChan(nil), make(ResetChan))
	checkNilable(t, "func", ResetFunc(nil), ResetFunc(func() { resetCalls["func"]++ }))
}

func checkNilable[T Resetter](t *testing.T, name string, empty, value T) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		resetCalls[name] = 0
		box := Single[T]{Value: empty}
		box.Reset()
		if resetCalls[name] != 0 || !reflect.ValueOf(box.Value).IsNil() {
			t.Fatal("Reset вызван для nil-значения или изменил его")
		}
		box.Value = value
		box.Reset()
		if resetCalls[name] != 1 || reflect.ValueOf(box.Value).IsNil() {
			t.Fatal("Reset должен вызвать метод ненулевого значения ровно один раз и сохранить поле")
		}
	})
}

func TestResetGenericValueKinds(t *testing.T) {
	resetCalls["scalar"] = 0
	resetCalls["struct"] = 0
	resetCalls["record"] = 0
	var scalar Single[ResetScalar]
	var structure Single[ResetStruct]
	var record Single[ResetRecord]
	scalar.Reset()
	structure.Reset()
	record.Reset()
	for _, name := range []string{"scalar", "struct", "record"} {
		if resetCalls[name] != 1 { t.Fatalf("Reset нулевого значения %s вызван %d раз", name, resetCalls[name]) }
	}
	items := []int{1, 2}
	record.Value.Items = items
	record.Reset()
	if resetCalls["record"] != 2 || len(record.Value.Items) != 2 || &record.Value.Items[0] != &items[0] || items[0] != 0 || items[1] != 0 {
		t.Fatal("Reset несравнимого значения не вызван или поле заменено")
	}
}
`

const pointerSource = `package fixture

import "bytes"

type BufferPtr *bytes.Buffer
type BufferPtrPtr *BufferPtr

// generate:reset
type Widget struct {
	Buffer BufferPtr
	Pointer *bytes.Buffer
	Double **bytes.Buffer
	NamedDouble BufferPtrPtr
	Nil BufferPtr
	NilDouble **bytes.Buffer
	InnerNil **bytes.Buffer
	NamedInnerNil BufferPtrPtr
}
`

const pointerTests = `package fixture

import (
	"bytes"
	"testing"
)

func TestReset(t *testing.T) {
	named := bytes.NewBufferString("named")
	ordinary := bytes.NewBufferString("ordinary")
	double := bytes.NewBufferString("double")
	originalDouble := double
	namedDouble := bytes.NewBufferString("named double")
	inner := BufferPtr(namedDouble)
	var empty *bytes.Buffer
	var namedEmpty BufferPtr
	w := Widget{
		Buffer: BufferPtr(named), Pointer: ordinary, Double: &double,
		NamedDouble: BufferPtrPtr(&inner), InnerNil: &empty, NamedInnerNil: BufferPtrPtr(&namedEmpty),
	}
	w.Reset()
	if w.Buffer != BufferPtr(named) || w.Pointer != ordinary || w.Double != &double || *w.Double != originalDouble ||
		w.NamedDouble != BufferPtrPtr(&inner) || *w.NamedDouble != BufferPtr(namedDouble) {
		t.Fatal("Reset заменил указатели")
	}
	for _, buffer := range []*bytes.Buffer{named, ordinary, originalDouble, namedDouble} {
		if buffer.Len() != 0 { t.Fatalf("буфер не сброшен: %q", buffer.String()) }
	}
	if w.Nil != nil || w.NilDouble != nil || w.InnerNil != &empty || *w.InnerNil != nil ||
		w.NamedInnerNil != BufferPtrPtr(&namedEmpty) || *w.NamedInnerNil != nil {
		t.Fatal("Reset изменил nil-указатели")
	}
	var absent *Widget
	absent.Reset()
}
`
