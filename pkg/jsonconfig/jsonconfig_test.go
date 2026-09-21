package jsonconfig

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type testConfig struct {
	Address  string  `json:"address"`
	Restore  bool    `json:"restore"`
	Interval Seconds `json:"interval"`
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"restore": false, "interval": "1m"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig{Address: "localhost:8080", Restore: true, Interval: 300}

	if err := Load(path, &cfg); err != nil {
		t.Fatalf("Load() ошибка = %v", err)
	}
	if want := (testConfig{Address: "localhost:8080", Restore: false, Interval: 60}); cfg != want {
		t.Fatalf("Load() = %+v, ожидается %+v", cfg, want)
	}
}

func TestLoad_NotExist(t *testing.T) {
	var cfg testConfig
	err := Load(filepath.Join(t.TempDir(), "missing.json"), &cfg)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load() ошибка = %v, ожидается os.ErrNotExist", err)
	}
}

func TestLoad_Comment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{\n\"restore\": true // пояснение\n}"), 0o644); err != nil {
		t.Fatal(err)
	}

	var cfg testConfig
	if err := Load(path, &cfg); err == nil {
		t.Fatal("Load() для JSON с комментарием вернул nil, ожидается ошибка")
	}
}

func TestSeconds_UnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    string
		want    Seconds
		wantErr bool
	}{
		{name: "секунды", data: `"1s"`, want: 1},
		{name: "минуты", data: `"2m"`, want: 120},
		{name: "доли секунды отбрасываются", data: `"1.5s"`, want: 1},
		{name: "не длительность", data: `"abc"`, wantErr: true},
		{name: "пустая строка", data: `""`, wantErr: true},
		{name: "число вместо строки", data: `5`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got Seconds
			err := json.Unmarshal([]byte(tc.data), &got)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Unmarshal(%s) ошибка = %v, ожидается ошибка: %t", tc.data, err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("Unmarshal(%s) = %d, ожидается %d", tc.data, got, tc.want)
			}
		})
	}
}
