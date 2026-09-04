package server

import (
	"net/http"

	"github.com/shigabutdinoff/metrics/internal/audit"
	"go.uber.org/zap"
)

func (s *Server) setupAudit() {
	var sinks []audit.Observer

	if s.AuditFile != "" {
		sink, err := audit.NewFileSink(s.AuditFile)
		if err != nil {
			s.Logger.Warn("Не удалось открыть файл аудита", zap.Error(err))
		} else {
			sinks = append(sinks, sink)
			s.auditClosers = append(s.auditClosers, sink)
		}
	}

	if s.AuditURL != "" {
		sink, err := audit.NewHTTPSink(s.AuditURL)
		if err != nil {
			s.Logger.Warn("Приёмник аудита по HTTP не подключён", zap.Error(err))
		} else {
			sinks = append(sinks, sink)
		}
	}

	if len(sinks) == 0 {
		return
	}

	p := audit.NewPublisher(s.Logger)
	for _, sink := range sinks {
		p.Register(sink)
	}
	s.auditor = p
}

func (s *Server) audit(mw func(audit.Notifier, *zap.Logger) func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	if s.auditor == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return mw(s.auditor, s.Logger)
}

func (s *Server) closeAudit() {
	if s.auditor == nil {
		return
	}
	s.auditor.Close()
	for _, c := range s.auditClosers {
		if err := c.Close(); err != nil {
			s.Logger.Warn("Ошибка закрытия приёмника аудита", zap.Error(err))
		}
	}
}
