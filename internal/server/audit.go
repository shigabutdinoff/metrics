package server

import (
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"github.com/shigabutdinoff/metrics/internal/audit"
)

func (s *Server) setupAudit() error {
	var sinks []audit.Observer

	if s.AuditFile != "" {
		sink, err := audit.NewFileSink(s.AuditFile)
		if err != nil {
			return fmt.Errorf("файл аудита: %w", err)
		}
		sinks = append(sinks, sink)
		s.auditClosers = append(s.auditClosers, sink)
	}

	if s.AuditURL != "" {
		sink, err := audit.NewHTTPSink(s.AuditURL)
		if err != nil {
			return err
		}
		sinks = append(sinks, sink)
	}

	if len(sinks) == 0 {
		s.Logger.Info("Аудит отключён: приёмники не настроены")
		return nil
	}

	p := audit.NewPublisher(s.Logger)
	for _, sink := range sinks {
		p.Register(sink)
	}
	s.auditor = p
	return nil
}

func (s *Server) audit(mw func(audit.Notifier, *zap.Logger) func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	if s.auditor == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return mw(s.auditor, s.Logger)
}

func (s *Server) notifier() audit.Notifier {
	if s.auditor == nil {
		return nil
	}
	return s.auditor
}

func (s *Server) closeAudit() {
	if s.auditor != nil {
		s.auditor.Close()
	}

	for _, c := range s.auditClosers {
		if err := c.Close(); err != nil {
			s.Logger.Warn("Ошибка закрытия приёмника аудита", zap.Error(err))
		}
	}

	s.auditor = nil
	s.auditClosers = nil
}
