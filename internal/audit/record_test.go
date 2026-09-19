package audit

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func TestRecordCollectsNames(t *testing.T) {
	rec := &Names{}
	ctx := WithRecord(context.Background(), rec)

	Record(ctx, "Alloc")
	Record(ctx, "PollCount", "RandomValue")

	want := []string{"Alloc", "PollCount", "RandomValue"}
	if got := rec.Collected(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Collected() = %v, ожидается %v", got, want)
	}
}

func TestCollectedReturnsCopy(t *testing.T) {
	rec := &Names{}
	Record(WithRecord(context.Background(), rec), "Alloc")

	got := rec.Collected()
	got[0] = "испорчено"

	if again := rec.Collected(); again[0] != "Alloc" {
		t.Fatalf("Collected()[0] = %q, ожидается %q", again[0], "Alloc")
	}
}

func TestRecordWithoutNamesInContext(t *testing.T) {
	// Запрос вне аудита не должен падать на записи имён.
	Record(context.Background(), "Alloc")
}

func TestRecordConcurrent(t *testing.T) {
	const workers = 50

	rec := &Names{}
	ctx := WithRecord(context.Background(), rec)

	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Record(ctx, fmt.Sprintf("metric-%d", i))
		}()
	}
	wg.Wait()

	seen := make(map[string]bool, workers)
	for _, name := range rec.Collected() {
		seen[name] = true
	}
	if len(seen) != workers {
		t.Fatalf("собрано уникальных имён = %d, ожидается %d", len(seen), workers)
	}
}
