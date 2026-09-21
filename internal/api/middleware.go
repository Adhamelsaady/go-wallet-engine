package api

import (
	"log/slog"
	"net/http"
	"time"
)

func AdminAuthMiddleware (expectedKey string) func (http.Handler) http.Handler {
	
	return func (next http.Handler) http.Handler{
		return http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			clientIP := r.RemoteAddr
			providedKey := r.Header.Get("X-Admin-API-Key")
			if providedKey == "" || providedKey != expectedKey {
				slog.Warn("AUDIT: unauthorized admin access attempt",
					slog.String("path", r.URL.Path),
					slog.String("method", r.Method),
					slog.String("client_ip", clientIP),
					slog.Duration("duration", time.Since(start)),
				)
				writeError(w, http.StatusUnauthorized, "unauthorized: invalid or missing admin API key")
				return 
			}
			slog.Info("AUDIT: admin endpoint accessed",
				slog.String("path", r.URL.Path),
				slog.String("method", r.Method),
				slog.String("client_ip", clientIP),
			)
			next.ServeHTTP(w, r)
		})
	}
}