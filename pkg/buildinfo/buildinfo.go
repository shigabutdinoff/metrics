package buildinfo

import (
	"cmp"
	"fmt"
	"io"
)

// Print пишет в w версию, дату и коммит сборки, пустое значение как N/A.
func Print(w io.Writer, version, date, commit string) {
	fmt.Fprintf(w, "Build version: %s\n", cmp.Or(version, "N/A"))
	fmt.Fprintf(w, "Build date: %s\n", cmp.Or(date, "N/A"))
	fmt.Fprintf(w, "Build commit: %s\n", cmp.Or(commit, "N/A"))
}
