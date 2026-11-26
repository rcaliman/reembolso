package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Apenas recursos públicos da PWA podem ser acessados sem sessão.
func publicPWAAsset(path string) bool {
	switch path {
	case "/manifest.webmanifest", "/sw.js", "/pwa.js", "/offline.html",
		"/icon-192.png", "/icon-512.png", "/icon-maskable-512.png", "/apple-touch-icon.png":
		return true
	}
	return false
}

func pwaRoutes(mux *http.ServeMux) {
	// Qualquer mudança nos recursos da interface oferece uma nova versão da PWA.
	hash := sha256.New()
	entries, _ := assets.ReadDir("web")
	for _, entry := range entries {
		content, _ := assets.ReadFile("web/" + entry.Name())
		fmt.Fprintln(hash, entry.Name())
		hash.Write(content)
	}
	version := fmt.Sprintf("%x", hash.Sum(nil))[:16]
	source, _ := assets.ReadFile("web/sw.js")
	worker := []byte(strings.ReplaceAll(string(source), "__CACHE_VERSION__", version))
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Service-Worker-Allowed", "/")
		http.ServeContent(w, r, "sw.js", time.Time{}, bytes.NewReader(worker))
	})
}
