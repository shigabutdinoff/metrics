package audit

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestHTTPSinkPostsEvent(t *testing.T) {
	type captured struct {
		method      string
		contentType string
		body        []byte
	}

	got := make(chan captured, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- captured{method: r.Method, contentType: r.Header.Get("Content-Type"), body: body}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sink, err := NewHTTPSink(ts.URL + "/audit")
	if err != nil {
		t.Fatalf("NewHTTPSink() вернул ошибку: %v", err)
	}

	want := Event{TS: 12345678, Metrics: []string{"Alloc", "Frees"}, IPAddress: "192.168.0.42"}
	if err := sink.Update(t.Context(), want); err != nil {
		t.Fatalf("Update() вернул ошибку: %v", err)
	}

	c := <-got
	if c.method != http.MethodPost {
		t.Errorf("метод = %q, ожидается %q", c.method, http.MethodPost)
	}
	if c.contentType != "application/json" {
		t.Errorf("Content-Type = %q, ожидается %q", c.contentType, "application/json")
	}

	var decoded Event
	if err := json.Unmarshal(c.body, &decoded); err != nil {
		t.Fatalf("тело %q не разбирается как событие аудита: %v", c.body, err)
	}
	if decoded.TS != want.TS || decoded.IPAddress != want.IPAddress || len(decoded.Metrics) != 2 {
		t.Errorf("получено событие %+v, ожидается %+v", decoded, want)
	}
}

func TestHTTPSinkReturnsErrorOnBadStatus(t *testing.T) {
	// 404 не входит в условия повтора, поэтому ошибка возвращается сразу
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	sink, err := NewHTTPSink(ts.URL)
	if err != nil {
		t.Fatalf("NewHTTPSink() вернул ошибку: %v", err)
	}
	if err := sink.Update(t.Context(), Event{}); err == nil {
		t.Fatal("Update() вернул nil, ожидается ошибка при статусе 404")
	}
}

func TestNewHTTPSinkRejectsBadURL(t *testing.T) {
	for _, raw := range []string{"", "localhost:8080", "ftp://audit.local/events", "http://"} {
		if _, err := NewHTTPSink(raw); err == nil {
			t.Errorf("NewHTTPSink(%q) вернул nil, ожидается ошибка", raw)
		}
	}
}

func TestRetryCondition(t *testing.T) {
	tests := []struct {
		name   string
		status int
		err    error
		want   bool
	}{
		{name: "сетевая ошибка", err: errors.New("временная ошибка"), want: true},
		{name: "ответа нет", want: false},
		{name: "429", status: http.StatusTooManyRequests, want: true},
		{name: "500", status: http.StatusInternalServerError, want: true},
		{name: "400", status: http.StatusBadRequest, want: false},
		{name: "200", status: http.StatusOK, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r *resty.Response
			if tt.status != 0 {
				r = &resty.Response{RawResponse: &http.Response{StatusCode: tt.status}}
			}
			if got := retryCondition(r, tt.err); got != tt.want {
				t.Fatalf("условие повтора = %v, ожидается %v", got, tt.want)
			}
		})
	}
}
