package main

import (
	"bytes"
	"encoding/json"
	"image/png"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestPWAPublicAssetsAndManifest(t *testing.T) {
	database = filepath.Join(t.TempDir(), "pwa.sqlite")
	h := routes()
	for _, path := range []string{"/manifest.webmanifest", "/sw.js", "/pwa.js", "/offline.html", "/icon-192.png", "/icon-512.png", "/icon-maskable-512.png", "/apple-touch-icon.png"} {
		w := request(t, h, "GET", path, nil, nil)
		if w.Code != 200 || len(w.Result().Cookies()) != 0 {
			t.Fatalf("public asset %s: %d %s", path, w.Code, w.Body)
		}
	}
	w := request(t, h, "GET", "/manifest.webmanifest", nil, nil)
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/manifest+json") {
		t.Fatal("manifest MIME type")
	}
	var manifest struct {
		ID       string `json:"id"`
		Scope    string `json:"scope"`
		Display  string `json:"display"`
		StartURL string `json:"start_url"`
		Icons    []struct {
			Src     string `json:"src"`
			Sizes   string `json:"sizes"`
			Purpose string `json:"purpose"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "/" || manifest.Scope != "/" || manifest.Display != "standalone" || manifest.StartURL != "/pedidos" || len(manifest.Icons) != 3 {
		t.Fatalf("manifest: %s", w.Body)
	}
	expectedSizes := map[string]int{"/icon-192.png": 192, "/icon-512.png": 512, "/icon-maskable-512.png": 512, "/apple-touch-icon.png": 180}
	for path, size := range expectedSizes {
		icon := request(t, h, "GET", path, nil, nil)
		cfg, err := png.DecodeConfig(bytes.NewReader(icon.Body.Bytes()))
		if err != nil || cfg.Width != size || cfg.Height != size {
			t.Fatalf("icon %s: %+v %v", path, cfg, err)
		}
		if !strings.HasPrefix(icon.Header().Get("Content-Type"), "image/png") {
			t.Fatal("icon MIME")
		}
	}
	w = request(t, h, "GET", "/sw.js", nil, nil)
	if strings.Contains(w.Body.String(), "__CACHE_VERSION__") || !strings.Contains(w.Header().Get("Content-Type"), "javascript") || w.Header().Get("Service-Worker-Allowed") != "/" || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("service worker headers or version: %v", w.Header())
	}
	for _, path := range []string{"/api/me", "/api/pedidos", "/api/pedidos/1/nota", "/api/admin/usuarios"} {
		w = request(t, h, "GET", path, nil, nil)
		if w.Code != 401 {
			t.Fatalf("protected %s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"/", "/pedidos", "/conta", "/admin", "/imprimir"} {
		w = request(t, h, "GET", path, nil, nil)
		if w.Code != 303 {
			t.Fatalf("protected page %s: %d", path, w.Code)
		}
	}
	for _, name := range []string{"index", "login", "account", "admin", "pedidos", "print"} {
		html, err := assets.ReadFile("web/" + name + ".html")
		if err != nil || !bytes.Contains(html, []byte(`rel="manifest"`)) || !bytes.Contains(html, []byte(`src="/pwa.js"`)) || !bytes.Contains(html, []byte(`rel="apple-touch-icon"`)) {
			t.Fatalf("PWA metadata missing: %s", name)
		}
	}
}
func TestHTTPSLoginUsesSecureSession(t *testing.T) {
	database = filepath.Join(t.TempDir(), "https.sqlite")
	req := httptest.NewRequest("POST", "https://reembolso.example/api/login", strings.NewReader(`{"login":"admin","senha":"admin-test-password"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	routes().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("HTTPS login: %d %s", w.Code, w.Body)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatal("HTTPS session cookie must be secure")
	}
}
