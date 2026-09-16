package compress

import (
	"bufio"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

var (
	writers = sync.Pool{New: func() any {
		cw := &compressWriter{}
		cw.zw = gzip.NewWriter(&cw.w)
		return cw
	}}
	readers = sync.Pool{New: func() any {
		return &compressReader{br: bufio.NewReader(nil), zr: new(gzip.Reader)}
	}}
)

type sink struct {
	http.ResponseWriter
}

type compressWriter struct {
	w           sink
	zw          *gzip.Writer
	wroteHeader bool
}

func (c *compressWriter) Header() http.Header {
	return c.w.Header()
}

func (c *compressWriter) Write(p []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	return c.zw.Write(p)
}

func (c *compressWriter) WriteHeader(statusCode int) {
	if c.wroteHeader {
		return
	}
	c.wroteHeader = true
	if statusCode < 300 {
		c.w.Header().Set("Content-Encoding", "gzip")
		c.w.Header().Del("Content-Length")
	}
	c.w.WriteHeader(statusCode)
}

// Close закрывает gzip.Writer и досылает все данные из буфера.
func (c *compressWriter) Close() error {
	return c.zw.Close()
}

type compressReader struct {
	r  io.ReadCloser
	br *bufio.Reader
	zr *gzip.Reader
}

func (c *compressReader) Read(p []byte) (n int, err error) {
	return c.zr.Read(p)
}

func (c *compressReader) Close() error {
	if err := c.r.Close(); err != nil {
		return err
	}
	return c.zr.Close()
}

func (c *compressReader) release() {
	c.br.Reset(nil)
	c.r = nil
	readers.Put(c)
}

// GzipMiddleware распаковывает запрос и сжимает ответ в формате gzip.
func GzipMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contentEncoding := r.Header.Get("Content-Encoding")
			sendsGzip := strings.Contains(contentEncoding, "gzip")
			if sendsGzip {
				cr := readers.Get().(*compressReader)
				cr.br.Reset(r.Body)
				if err := cr.zr.Reset(cr.br); err != nil {
					cr.release()
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				cr.r = r.Body
				r.Body = cr
				defer func() {
					_ = cr.Close()
					r.Body = cr.r
					cr.release()
				}()
			}

			ow := w

			acceptEncoding := r.Header.Get("Accept-Encoding")
			supportsGzip := strings.Contains(acceptEncoding, "gzip")
			if supportsGzip {
				cw := writers.Get().(*compressWriter)
				cw.w.ResponseWriter = w
				cw.wroteHeader = false
				cw.zw.Reset(&cw.w)
				ow = cw
				defer func() {
					_ = cw.Close()
					cw.w.ResponseWriter = nil
					writers.Put(cw)
				}()
			}

			next.ServeHTTP(ow, r)
		})
	}
}
