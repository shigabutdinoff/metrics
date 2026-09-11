package resetgen

import "testing"

func TestGeneratorCompositeZero(t *testing.T) {

	t.Run("architecture dependent array", func(t *testing.T) {
		dir := fixtureModule(t, map[string]string{
			"source.go": `package fixture
import "unsafe"
// generate:reset
type Box struct { Value [unsafe.Sizeof(uintptr(0))]byte }
`,
			"source_test.go": `package fixture
import "testing"
func TestReset(t *testing.T) {
	var box Box
	for i := range box.Value { box.Value[i] = byte(i + 1) }
	box.Reset()
	if box != (Box{}) { t.Fatalf("массив не обнулён: %#v", box) }
}
`,
		})
		runFixtureGenerator(t, dir)
		testFixtureModule(t, dir)
		requireFixtureCommand(t, fixtureCommand{
			dir:  dir,
			env:  []string{"GOOS=linux", "GOARCH=386", "CGO_ENABLED=0"},
			name: "go",
			args: []string{"build", "./..."},
		})
		assertFixtureGeneratorIdempotent(t, dir)
	})

	for _, tc := range []struct {
		name  string
		files map[string]string
	}{
		{
			name: "inherited array with shadowed int",
			files: map[string]string{
				"source.go": `package fixture
import "example.com/reset-regression/child"
type int = string
// generate:reset
type Box child.Child
`,
				"child/source.go": `package child
type Child struct { Value [2]int }
func New() Child { return Child{Value: [2]int{7, 9}} }
`,
				"source_test.go": `package fixture
import (
	"example.com/reset-regression/child"
	"testing"
)
func TestReset(t *testing.T) {
	box := Box(child.New())
	box.Reset()
	if box != (Box{}) { t.Fatalf("унаследованный массив не обнулён: %#v", box) }
}
`,
			},
		},
		{
			name: "helper and builtin name collisions",
			files: map[string]string{
				"source.go": `package fixture
type any = string
var resetZero, resetZero2 = 1, 2
// generate:reset
type Value[T interface{}] struct { Array [2]T }
// generate:reset
type Box struct {
	Nested struct { Number int }
	Array [2]int
	Generic Value[int]
}
`,
				"source_test.go": `package fixture
import "testing"
func TestReset(t *testing.T) {
	box := Box{Array: [2]int{1, 2}, Generic: Value[int]{Array: [2]int{3, 4}}}
	box.Nested.Number = 5
	box.Reset()
	if box != (Box{}) { t.Fatalf("составные поля не обнулены: %#v", box) }
	if resetZero != 1 || resetZero2 != 2 { t.Fatal("объявления пакета изменены") }
}
`,
			},
		},
		{
			name: "helper collides with imports across files",
			files: map[string]string{
				"source.go": `package fixture
import resetZero "time"
// generate:reset
type Box struct {
	At resetZero.Time
	Array [2]int
}
`,
				"other.go": `package fixture
import "example.com/reset-regression/helper"
var ImportedValue = resetZero2.Value{Number: 7}
`,
				"helper/source.go": `package resetZero2
type Value struct { Number int }
`,
				"source_test.go": `package fixture
import (
	"testing"
	"time"
)
func TestReset(t *testing.T) {
	box := Box{At: time.Unix(1, 0), Array: [2]int{3, 4}}
	box.Reset()
	if box != (Box{}) { t.Fatalf("поля не обнулены: %#v", box) }
	if ImportedValue.Number != 7 { t.Fatal("изменено значение из соседнего файла") }
}
`,
			},
		},
		{
			name: "named and nested composite pointers",
			files: map[string]string{
				"source.go": `package fixture
type Value struct { Number int; Items []int }
type ValuePointer *Value
type ValuePointerPointer *ValuePointer
type ArrayPointer *[2]int
type ArrayPointerPointer *ArrayPointer
// generate:reset
type Box struct {
	Named ValuePointer
	Pointer *Value
	Double **Value
	NamedDouble ValuePointerPointer
	Array ArrayPointer
	ArrayDouble ArrayPointerPointer
	Nil ValuePointer
	NilDouble **Value
	InnerNil **Value
	NamedInnerNil ValuePointerPointer
	NilArray ArrayPointer
	InnerNilArray ArrayPointerPointer
}
`,
				"source_test.go": `package fixture
import (
	"reflect"
	"testing"
)
func TestReset(t *testing.T) {
	named := &Value{Number: 1, Items: []int{2}}
	pointer := &Value{Number: 3, Items: []int{4}}
	double := &Value{Number: 5, Items: []int{6}}
	originalDouble := double
	namedDouble := ValuePointer(&Value{Number: 7, Items: []int{8}})
	originalNamedDouble := namedDouble
	array := &[2]int{9, 10}
	arrayDouble := ArrayPointer(&[2]int{11, 12})
	originalArrayDouble := arrayDouble
	var empty *Value
	var namedEmpty ValuePointer
	var arrayEmpty ArrayPointer
	box := Box{
		Named: ValuePointer(named), Pointer: pointer, Double: &double,
		NamedDouble: ValuePointerPointer(&namedDouble), Array: ArrayPointer(array),
		ArrayDouble: ArrayPointerPointer(&arrayDouble), InnerNil: &empty,
		NamedInnerNil: ValuePointerPointer(&namedEmpty), InnerNilArray: ArrayPointerPointer(&arrayEmpty),
	}
	before := box
	box.Reset()
	if box != before || double != originalDouble || namedDouble != originalNamedDouble || arrayDouble != originalArrayDouble {
		t.Fatal("Reset заменил указатели")
	}
	if empty != nil || namedEmpty != nil || arrayEmpty != nil { t.Fatal("Reset изменил nil-указатели") }
	for _, value := range []*Value{named, pointer, originalDouble, originalNamedDouble} {
		if !reflect.DeepEqual(*value, Value{}) { t.Fatalf("структура не обнулена: %#v", value) }
	}
	if *array != ([2]int{}) || *originalArrayDouble != ([2]int{}) { t.Fatal("массивы не обнулены") }
	var absent *Box
	absent.Reset()
}
`,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixtureModule(t, tc.files)
			runFixtureGenerator(t, dir)
			testFixtureModule(t, dir)
			assertFixtureGeneratorIdempotent(t, dir)
		})
	}
}
