package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthenticationRateLimit(t *testing.T) {
	limiter := newAuthLimiter()
	now := time.Unix(1000, 0)
	limiter.now = func() time.Time { return now }
	calls := 0
	h := limiter.protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	send := func(path, address string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, nil)
		r.RemoteAddr = address
		r.Header.Set("X-Forwarded-For", time.Now().String())
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for range 30 {
		if w := send("/api/login", "192.0.2.1:1234"); w.Code != 204 {
			t.Fatal(w.Code)
		}
	}
	w := send("/api/login", "192.0.2.1:5678")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" || calls != 30 {
		t.Fatalf("limit bypassed: %d, calls %d", w.Code, calls)
	}
	if w := send("/api/login", "192.0.2.2:1234"); w.Code != 204 {
		t.Fatal("independent client blocked")
	}
	if w := send("/api/pedidos", "192.0.2.1:1234"); w.Code != 204 {
		t.Fatal("unrelated action blocked")
	}
	now = now.Add(time.Minute)
	if w := send("/api/login", "192.0.2.1:1234"); w.Code != 204 {
		t.Fatal("quota did not renew")
	}
}

func TestAuthenticationLimiterBoundsMemoryAndConcurrentWork(t *testing.T) {
	limiter := newAuthLimiter()
	for i := range 4096 {
		limiter.clients[string(rune(i))] = attemptWindow{start: time.Now(), count: 1}
	}
	if limiter.allow("new-client", 30) {
		t.Fatal("unbounded client map")
	}
	limiter = newAuthLimiter()
	for range cap(limiter.active) {
		limiter.active <- struct{}{}
	}
	h := limiter.protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("expensive handler reached") }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/passkeys/login/begin", nil))
	if w.Code != 429 {
		t.Fatal(w.Code)
	}
}
