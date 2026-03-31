package metrics

import (
	"html"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/shigabutdinoff/metrics/internal/storage"
)

func Index(st storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		rows := make([]string, 0)

		for name, v := range st.GetGauges(req.Context()) {
			if v == nil {
				continue
			}
			rows = append(
				rows,
				"<li>gauge "+html.EscapeString(name)+": "+html.EscapeString(strconv.FormatFloat(*v, 'f', -1, 64))+"</li>",
			)
		}
		for name, v := range st.GetCounters(req.Context()) {
			if v == nil {
				continue
			}
			rows = append(
				rows,
				"<li>counter "+html.EscapeString(name)+": "+html.EscapeString(strconv.FormatInt(*v, 10))+"</li>",
			)
		}

		sort.Strings(rows)
		if len(rows) == 0 {
			rows = append(rows, "<li>No metrics yet</li>")
		}

		res.Header().Set("Content-Type", "text/html; charset=utf-8")
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write([]byte("<html><body><ul>" + strings.Join(rows, "") + "</ul></body></html>"))
	}
}
