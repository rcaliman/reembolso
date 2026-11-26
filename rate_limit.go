package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// O endereço do socket é confiável; cabeçalhos de proxy não autorizam novas cotas.
func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

type attemptWindow struct {
	start time.Time
	count int
}
type authLimiter struct {
	mu      sync.Mutex
	clients map[string]attemptWindow
	now     func() time.Time
	active  chan struct{}
}

func newAuthLimiter() *authLimiter {
	return &authLimiter{clients: make(map[string]attemptWindow), now: time.Now, active: make(chan struct{}, 8)}
}
func (l *authLimiter) allow(key string, limit int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for client, window := range l.clients {
		if now.Sub(window.start) >= time.Minute {
			delete(l.clients, client)
		}
	}
	window, exists := l.clients[key]
	if !exists {
		// Limita memória e falha sem expulsar cotas ainda ativas.
		if len(l.clients) >= 4096 {
			return false
		}
		window.start = now
	}
	if window.count >= limit {
		return false
	}
	window.count++
	l.clients[key] = window
	return true
}
func (l *authLimiter) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		group, limit := "", 30
		if r.Method == http.MethodPost {
			switch {
			case r.URL.Path == "/api/login":
				group = "password"
			case strings.HasPrefix(r.URL.Path, "/api/passkeys/"):
				group = "passkey"
				limit = 60
			case r.URL.Path == "/api/senha":
				group = "password"
			}
		}
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/passkeys/") {
			group = "password"
		}
		if group != "" {
			if !l.allow(group+":"+clientAddress(r), limit) {
				rateLimited(w)
				return
			}
			select {
			case l.active <- struct{}{}:
				defer func() { <-l.active }()
			default:
				rateLimited(w)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func rateLimited(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	reply(w, http.StatusTooManyRequests, map[string]string{"error": "Muitas tentativas. Aguarde um minuto e tente novamente."})
}
