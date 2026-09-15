package pool

import (
	"sync"
	"testing"
)

type item struct {
	data   []byte
	resets int
}

func (i *item) Reset() {
	i.data = i.data[:0]
	i.resets++
}

func newItem() *item { return new(item) }

func TestPool_GetEmpty(t *testing.T) {
	calls := 0
	p := New(func() *item {
		calls++
		return new(item)
	})

	if p.Get() == nil {
		t.Fatal("Get вернул nil")
	}
	if calls != 1 {
		t.Fatalf("конструктор вызван %d раз, ожидается 1", calls)
	}
}

func TestPool_PutResets(t *testing.T) {
	p := New(newItem)
	it := p.Get()
	it.data = append(it.data, 1, 2, 3)

	p.Put(it)

	if it.resets != 1 {
		t.Fatalf("Reset вызван %d раз, ожидается 1", it.resets)
	}
	if len(it.data) != 0 {
		t.Fatalf("len(data) = %d после Put, ожидается 0", len(it.data))
	}
}

func TestPool_Concurrent(t *testing.T) {
	p := New(newItem)
	var wg sync.WaitGroup
	for g := range 64 {
		wg.Go(func() {
			for range 100 {
				it := p.Get()
				if len(it.data) != 0 {
					t.Errorf("горутина %d: объект из Get не сброшен", g)
				}
				it.data = append(it.data, byte(g))
				p.Put(it)
			}
		})
	}
	wg.Wait()
}
