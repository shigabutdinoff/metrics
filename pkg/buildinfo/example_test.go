package buildinfo_test

import (
	"os"

	"github.com/shigabutdinoff/metrics/pkg/buildinfo"
)

// Пустые значения печатаются как N/A.
func ExamplePrint() {
	buildinfo.Print(os.Stdout, "v1.0.1", "", "")

	// Output:
	// Build version: v1.0.1
	// Build date: N/A
	// Build commit: N/A
}
