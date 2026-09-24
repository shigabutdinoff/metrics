package updates_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"go.uber.org/zap"

	"github.com/shigabutdinoff/metrics/internal/handlers/route/updates"
	"github.com/shigabutdinoff/metrics/internal/storage"
)

// Приём пачки метрик одним запросом. В ответе возвращается присланный массив.
func ExampleStoreApplicationJSONBatch() {
	st := storage.NewMemStorage()
	h := updates.StoreApplicationJSONBatch(st, nil, zap.NewNop())

	body := strings.NewReader(
		`[{"id":"Alloc","type":"gauge","value":12.5},{"id":"PollCount","type":"counter","delta":5}]`,
	)
	req := httptest.NewRequest(http.MethodPost, "/updates/", body)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	fmt.Println(rr.Code)
	fmt.Print(rr.Body.String())

	// Output:
	// 200
	// [{"id":"Alloc","type":"gauge","value":12.5},{"id":"PollCount","type":"counter","delta":5}]
}
