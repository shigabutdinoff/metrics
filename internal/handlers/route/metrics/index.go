package metrics

import (
	"html"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/shigabutdinoff/metrics/internal/storage"
)

// Index обрабатывает GET / и отдаёт список метрик в виде HTML.
func Index(st storage.Storage) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		gauges := st.GetGauges(req.Context())
		counters := st.GetCounters(req.Context())
		rows := make([]string, 0, len(gauges)+len(counters))

		for name, v := range gauges {
			if v == nil {
				continue
			}
			rows = append(
				rows,
				"<li>gauge "+html.EscapeString(name)+": "+html.EscapeString(strconv.FormatFloat(*v, 'f', -1, 64))+"</li>",
			)
		}
		for name, v := range counters {
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
			rows = append(rows, "<li>Пока нет метрик</li>")
		}

		res.Header().Set("Content-Type", "text/html; charset=utf-8")
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write([]byte("<html><body><ul>" + strings.Join(rows, "") + "</ul></body></html>"))
	}
}
