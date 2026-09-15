package pool_test

import (
	"fmt"

	"github.com/shigabutdinoff/metrics/pkg/pool"
)

type item struct{ id int }

func (i *item) Reset() {
	fmt.Println("reset", i.id)
	i.id = 0
}

// Пул объектов: Put сбрасывает объект через Reset.
func ExamplePool() {
	items := pool.New(func() *item { return new(item) })

	it := items.Get()
	it.id = 7
	items.Put(it)

	// Output:
	// reset 7
}
