package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func readEvents(t *testing.T, path string) []Event {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("не удалось открыть файл аудита: %v", err)
	}
	defer func() { _ = f.Close() }()

	var out []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatalf("строка %q не разбирается как событие аудита: %v", line, err)
		}
		out = append(out, e)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("ошибка чтения файла аудита: %v", err)
	}
	return out
}

func TestFileSinkWritesEventPerLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")

	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink() вернул ошибку: %v", err)
	}

	want := []Event{
		{TS: 100, Metrics: []string{"Alloc"}, IPAddress: "192.168.0.42"},
		{TS: 200, Metrics: []string{"Alloc", "Frees"}, IPAddress: "10.0.0.5"},
	}
	for _, e := range want {
		if err := sink.Update(t.Context(), e); err != nil {
			t.Fatalf("Update() вернул ошибку: %v", err)
		}
	}

	if err := sink.Close(); err != nil {
		t.Fatalf("Close() вернул ошибку: %v", err)
	}

	if got := readEvents(t, path); !reflect.DeepEqual(got, want) {
		t.Errorf("события в файле = %+v, ожидается %+v", got, want)
	}
}

func TestFileSinkAppendsToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")

	if err := os.WriteFile(path, []byte(`{"ts":1,"metrics":["Old"],"ip_address":"127.0.0.1"}`+"\n"), 0o644); err != nil {
		t.Fatalf("не удалось подготовить файл: %v", err)
	}

	sink, err := NewFileSink(path)
	if err != nil {
		t.Fatalf("NewFileSink() вернул ошибку: %v", err)
	}
	if err := sink.Update(t.Context(), Event{TS: 2, Metrics: []string{"New"}, IPAddress: "127.0.0.1"}); err != nil {
		t.Fatalf("Update() вернул ошибку: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close() вернул ошибку: %v", err)
	}

	got := readEvents(t, path)
	if len(got) != 2 {
		t.Fatalf("в файле %d строк, ожидается 2: существующие записи должны сохраниться", len(got))
	}
	if got[0].Metrics[0] != "Old" || got[1].Metrics[0] != "New" {
		t.Errorf("порядок записей нарушен: %+v", got)
	}
}
