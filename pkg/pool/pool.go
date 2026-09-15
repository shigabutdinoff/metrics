package pool

import "sync"

// Resetter ограничивает элементы пула типами с методом Reset.
type Resetter interface {
	Reset()
}

// Pool хранит объекты типа T, Put сбрасывает их через Reset.
type Pool[T Resetter] struct {
	pool sync.Pool
}

// New создаёт пул; newFn делает новый объект, когда пул пуст.
func New[T Resetter](newFn func() T) *Pool[T] {
	var p Pool[T]
	p.pool.New = func() any { return newFn() }
	return &p
}

// Get возвращает объект из пула или новый от newFn.
func (p *Pool[T]) Get() T {
	return p.pool.Get().(T)
}

// Put сбрасывает объект через Reset и кладёт его в пул.
func (p *Pool[T]) Put(v T) {
	v.Reset()
	p.pool.Put(v)
}
