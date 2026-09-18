package jsonconfig_test

import (
	"fmt"
	"os"

	"github.com/shigabutdinoff/metrics/pkg/jsonconfig"
)

// Ключи из файла заменяют значения по умолчанию, прочие поля не меняются.
func ExampleLoad() {
	f, err := os.CreateTemp("", "config-*.json")
	if err != nil {
		panic(err)
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(`{"store_interval": "1m"}`); err != nil {
		panic(err)
	}
	if err = f.Close(); err != nil {
		panic(err)
	}

	cfg := struct {
		Address       string             `json:"address"`
		StoreInterval jsonconfig.Seconds `json:"store_interval"`
	}{Address: "localhost:8080", StoreInterval: 300}

	if err = jsonconfig.Load(f.Name(), &cfg); err != nil {
		panic(err)
	}
	fmt.Println(cfg.Address, cfg.StoreInterval)

	// Output:
	// localhost:8080 60
}
