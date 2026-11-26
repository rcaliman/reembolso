package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed web/*.html web/*.js web/*.css web/*.png web/*.webmanifest store.py
var assets embed.FS
var database string

func reply(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
func routes() http.Handler {
	mux := http.NewServeMux()
	authRoutes(mux)
	passkeyRoutes(mux)
	pedidoRoutes(mux)
	redeRoutes(mux)
	pwaRoutes(mux)
	mux.HandleFunc("POST /api/notas/extrair", limitPDFProcessing(extractInvoice))
	mux.HandleFunc("GET /api/cadastros", func(w http.ResponseWriter, r *http.Request) {
		out, e := store("list", nil)
		if e != nil {
			log.Print(e)
			reply(w, 500, map[string]string{"error": "Não foi possível consultar os cadastros."})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(out)
	})
	mux.HandleFunc("POST /api/pedidos", limitPDFProcessing(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			o := r.Header.Get("Origin")
			if o != "http://"+r.Host && o != "https://"+r.Host {
				reply(w, 403, map[string]string{"error": "Origem inválida."})
				return
			}
		}
		d, cleanup, err := readSubmission(w, r)
		if err != nil {
			reply(w, 400, map[string]string{"error": err.Error()})
			return
		}
		saved := false
		defer func() {
			if !saved {
				cleanup()
			}
		}()
		valid, e := validate(d)
		if e != nil {
			reply(w, 400, map[string]string{"error": e.Error()})
			return
		}
		for _, key := range attachmentKeys {
			if d[key] != nil {
				valid[key] = d[key]
			}
		}
		out, e := store("save", valid)
		if e != nil {
			log.Print(e)
			reply(w, 500, map[string]string{"error": "Não foi possível salvar o pedido."})
			return
		}
		var failure struct {
			Error  string `json:"error"`
			Status int    `json:"status"`
		}
		if err := json.Unmarshal(out, &failure); err != nil {
			reply(w, 500, map[string]string{"error": "Resposta inválida da persistência."})
			return
		}
		if failure.Error != "" {
			reply(w, failure.Status, map[string]string{"error": failure.Error})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		saved = true
		w.WriteHeader(201)
		w.Write(out)
	}))
	mux.HandleFunc("GET /api/pedidos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, e := strconv.Atoi(r.PathValue("id"))
		if e != nil || id < 1 {
			http.NotFound(w, r)
			return
		}
		out, e := store("get", map[string]int{"id": id})
		if e != nil {
			reply(w, 500, map[string]string{"error": "Erro ao consultar pedido."})
			return
		}
		if strings.TrimSpace(string(out)) == "null" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(out)
	})
	mux.HandleFunc("GET /api/pedidos/{id}/documentos/{document}", invoiceFile)
	mux.HandleFunc("GET /api/pedidos/{id}/documentos/{document}/paginas/{page}", invoiceFile)
	mux.HandleFunc("GET /api/pedidos/{id}/nota", invoiceFile)
	mux.HandleFunc("GET /api/pedidos/{id}/nota/paginas/{page}", invoiceFile)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path
		if name == "/" {
			name = "/index.html"
		}
		if name == "/login" {
			name = "/login.html"
		}
		if name == "/conta" {
			name = "/account.html"
		}
		if name == "/admin" {
			name = "/admin.html"
		}
		if name == "/imprimir" {
			name = "/print.html"
		}
		if name == "/rede-credenciada" {
			name = "/rede.html"
		}
		if name == "/pedidos" {
			name = "/pedidos.html"
		}
		content, e := assets.ReadFile("web" + name)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		if name == "/manifest.webmanifest" {
			w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
	})
	protected := newAuthLimiter().protect(authenticate(mux))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		protected.ServeHTTP(w, r)
	})
}
func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "Endereço HTTP")
	db := flag.String("db", "data/reembolso.sqlite", "Arquivo SQLite")
	cert := flag.String("tls-cert", "", "Certificado HTTPS (PEM)")
	key := flag.String("tls-key", "", "Chave privada HTTPS (PEM)")
	flag.StringVar(&passkeyOrigin, "webauthn-origin", "", "Origem pública do login por passkey (ex.: https://reembolso.exemplo.com:5002)")
	flag.Parse()
	if passkeyOrigin != "" {
		parsed, err := parsePasskeyOrigin(passkeyOrigin)
		if err != nil {
			log.Fatal(err)
		}
		passkeyOrigin = parsed.String()
	}
	if (*cert == "") != (*key == "") {
		log.Fatal("Informe -tls-cert e -tls-key juntos.")
	}
	database = *db
	if e := os.MkdirAll(filepath.Dir(database), 0700); e != nil {
		log.Fatal(e)
	}
	f, e := os.OpenFile(database, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		log.Fatal(e)
	}
	f.Close()
	if e = initializeStore(); e != nil {
		log.Fatal(e)
	}
	listener, e := net.Listen("tcp", *addr)
	if e != nil {
		log.Fatal(e)
	}
	scheme := "http"
	if *cert != "" {
		scheme = "https"
	}
	log.Printf("Reembolso disponível em %s://%s", scheme, listener.Addr())
	server := &http.Server{Handler: routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second}
	if *cert != "" {
		e = server.ServeTLS(listener, *cert, *key)
	} else {
		e = server.Serve(listener)
	}
	if e != nil && e != io.EOF {
		log.Fatal(e)
	}
}
