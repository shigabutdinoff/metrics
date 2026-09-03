package audit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

type fakeObserver struct {
	mu     sync.Mutex
	err    error
	delay  time.Duration
	events []Event
	closed bool
}

func (f *fakeObserver) Update(ctx context.Context, e Event) error {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return f.err
}

func (f *fakeObserver) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeObserver) snapshot() ([]Event, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Event, len(f.events))
	copy(out, f.events)
	return out, f.closed
}

func newEvent() Event {
	return Event{TS: 1, Metrics: []string{"Alloc"}, IPAddress: "192.168.0.42"}
}

func newTestPublisher(opts []Option, observers ...Observer) *Publisher {
	p := NewPublisher(zap.NewNop(), opts...)
	for _, o := range observers {
		p.Register(o)
	}
	return p
}

func TestNewPublisherOptions(t *testing.T) {
	p := NewPublisher(zap.NewNop(), WithBuffer(7), WithCloseTimeout(time.Second))
	if p.buffer != 7 || p.closeTimeout != time.Second {
		t.Fatalf("опции не применились: buffer=%d closeTimeout=%v", p.buffer, p.closeTimeout)
	}

	d := NewPublisher(zap.NewNop())
	if d.buffer != defaultBuffer || d.closeTimeout != defaultCloseTimeout {
		t.Fatalf("без опций ожидаются значения по умолчанию: buffer=%d closeTimeout=%v", d.buffer, d.closeTimeout)
	}
}

func TestPublisherFanOut(t *testing.T) {
	// ошибка одного приёмника не должна мешать доставке остальным
	first := &fakeObserver{err: errors.New("приёмник недоступен")}
	second := &fakeObserver{}
	p := newTestPublisher(nil, first, second)

	p.Publish(newEvent())

	p.Close()

	for i, o := range []*fakeObserver{first, second} {
		events, closed := o.snapshot()
		if len(events) != 1 {
			t.Fatalf("наблюдатель %d получил %d событий, ожидается 1", i, len(events))
		}
		if events[0].IPAddress != "192.168.0.42" || events[0].Metrics[0] != "Alloc" {
			t.Errorf("наблюдатель %d получил некорректное событие: %+v", i, events[0])
		}
		if closed {
			t.Errorf("издатель закрыл наблюдателя %d, закрывать должен владелец", i)
		}
	}
}

func TestPublisherPublishNeverBlocks(t *testing.T) {
	slow := &fakeObserver{delay: 50 * time.Millisecond}
	p := newTestPublisher([]Option{WithBuffer(1)}, slow)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			p.Publish(newEvent())
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish заблокировался при переполненном буфере")
	}

	p.Close()
}

func TestPublisherCloseIsIdempotentAndStopsPublish(t *testing.T) {
	observer := &fakeObserver{}
	p := newTestPublisher(nil, observer)

	p.Close()
	p.Close()

	p.Publish(newEvent())

	if events, _ := observer.snapshot(); len(events) != 0 {
		t.Errorf("после Close() доставлено %d событий, ожидается 0", len(events))
	}
}

func TestPublisherWithoutObservers(t *testing.T) {
	p := NewPublisher(zap.NewNop())

	p.Publish(newEvent())
	p.Close()
}

func TestPublisherCloseInterruptsHangingObserver(t *testing.T) {
	hanging := &fakeObserver{delay: time.Hour}
	p := newTestPublisher([]Option{WithCloseTimeout(100 * time.Millisecond)}, hanging)
	p.Publish(newEvent())

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Close()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close() не завершился: зависший приёмник блокирует остановку")
	}
}

func TestPublisherSlowObserverDoesNotDelayOthers(t *testing.T) {
	const events = 5

	slow := &fakeObserver{delay: 300 * time.Millisecond}
	fast := &fakeObserver{}
	p := newTestPublisher([]Option{WithCloseTimeout(100 * time.Millisecond)}, slow, fast)

	for i := 0; i < events; i++ {
		p.Publish(newEvent())
	}

	deadline := time.After(250 * time.Millisecond)
	for {
		got, _ := fast.snapshot()
		if len(got) == events {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("быстрый приёмник получил %d событий из %d: его тормозит медленный", len(got), events)
		case <-time.After(5 * time.Millisecond):
		}
	}

	p.Close()
}

func TestPublisherDeregisterStopsDelivery(t *testing.T) {
	first := &fakeObserver{}
	second := &fakeObserver{}
	p := newTestPublisher(nil, first, second)

	p.Publish(newEvent())
	p.Deregister(first)
	p.Publish(newEvent())

	p.Close()

	events, _ := first.snapshot()
	if len(events) != 1 {
		t.Errorf("отписанный наблюдатель получил %d событий, ожидается 1", len(events))
	}

	events, _ = second.snapshot()
	if len(events) != 2 {
		t.Errorf("оставшийся наблюдатель получил %d событий, ожидается 2", len(events))
	}
}

func TestPublisherDeregisterIsSafeForUnknownRepeatedAndClosed(t *testing.T) {
	observer := &fakeObserver{}
	p := newTestPublisher(nil, observer)

	p.Deregister(&fakeObserver{})
	p.Deregister(observer)
	p.Deregister(observer)

	p.Close()

	p.Deregister(observer)
}

type uncomparableObserver struct{ tags []string }

func (uncomparableObserver) Update(context.Context, Event) error { return nil }

func TestPublisherDeregisterIgnoresUncomparableObserver(t *testing.T) {
	first := uncomparableObserver{tags: []string{"a"}}
	second := uncomparableObserver{tags: []string{"b"}}
	p := newTestPublisher(nil, first, second)

	p.Deregister(first)
	p.Deregister(nil)

	p.Close()
}
