package buildinfo

import (
	"bytes"
	"testing"
)

func TestPrint(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version string
		date    string
		commit  string
		want    string
	}{
		{name: "пусто", want: "Build version: N/A\nBuild date: N/A\nBuild commit: N/A\n"},
		{name: "заполнено", version: "v1.0.1", date: "2026/09/15 12:00:00", commit: "86ee898", want: "Build version: v1.0.1\nBuild date: 2026/09/15 12:00:00\nBuild commit: 86ee898\n"},
		{name: "только коммит", commit: "86ee898", want: "Build version: N/A\nBuild date: N/A\nBuild commit: 86ee898\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			Print(&buf, tc.version, tc.date, tc.commit)
			if got := buf.String(); got != tc.want {
				t.Fatalf("вывод = %q, ожидается %q", got, tc.want)
			}
		})
	}
}
