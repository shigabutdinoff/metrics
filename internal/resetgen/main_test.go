package resetgen

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

const examplePkg = "./testdata/example"

func TestGenerate(t *testing.T) {
	result, err := generate(examplePkg)
	if err != nil {
		t.Fatalf("generate() вернул ошибку: %v", err)
	}

	path, err := filepath.Abs(filepath.Join(examplePkg, "reset.gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.files[path]; !ok {
		t.Fatalf("генератор не создал %s", path)
	}

	cfg := &packages.Config{Mode: packages.NeedTypes, Overlay: result.files}
	pkgs, err := packages.Load(cfg, examplePkg)
	if err != nil {
		t.Fatal(err)
	}

	if packages.PrintErrors(pkgs) > 0 {
		t.Error("сгенерированный код не компилируется")
	}
}

func TestGenerateBroken(t *testing.T) {
	if _, err := generate("./testdata/broken"); err != nil {
		t.Fatalf("generate() вернул ошибку: %v", err)
	}
}

func TestProjectUpToDate(t *testing.T) {
	result, err := generate("../../...")
	if err != nil {
		t.Fatalf("generate() вернул ошибку: %v", err)
	}

	if len(result.files) == 0 {
		t.Fatal("не найдено ни одной размеченной структуры")
	}

	for path, src := range result.files {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(src) {
			t.Errorf("%s устарел, запустите go run ./cmd/reset", path)
		}
	}
	for _, path := range result.obsolete {
		t.Errorf("%s больше не нужен, запустите go run ./cmd/reset", path)
	}
}
