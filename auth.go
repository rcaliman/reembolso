package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type user struct {
	ID     int    `json:"id"`
	Login  string `json:"login"`
	Perfil string `json:"perfil"`
}
type userKey struct{}

func currentUser(r *http.Request) user { u, _ := r.Context().Value(userKey{}).(user); return u }
func authJSON(w http.ResponseWriter, r *http.Request, d any) bool {
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(d) != nil {
		reply(w, 400, map[string]string{"error": "Dados inválidos."})
		return false
	}
	return true
}
func authStore(w http.ResponseWriter, action string, data any, onSuccess ...func()) bool {
	out, e := store(action, data)
	if e != nil {
		reply(w, 500, map[string]string{"error": "Não foi possível concluir a operação."})
		return false
	}
	var failure struct {
		Error  string `json:"error"`
		Status int    `json:"status"`
	}
	json.Unmarshal(out, &failure)
	if failure.Error != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(failure.Status)
		w.Write(out)
		return false
	}
	for _, callback := range onSuccess {
		callback()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
	return true
}
func sessionCookie(w http.ResponseWriter, r *http.Request, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "reembolso_session", Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: configuredHTTPS(r), MaxAge: age})
}
func authRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		var d struct {
			Login string `json:"login"`
			Senha string `json:"senha"`
		}
		if !authJSON(w, r, &d) {
			return
		}
		d.Login = strings.TrimSpace(d.Login)
		if len(d.Login) > 64 || len([]rune(d.Senha)) > 128 {
			reply(w, 401, map[string]string{"error": "Usuário ou senha incorretos."})
			return
		}
		out, e := store("login", map[string]string{"login": d.Login, "senha": d.Senha})
		if e != nil {
			reply(w, 500, map[string]string{"error": "Não foi possível entrar."})
			return
		}
		var result struct {
			Token   string `json:"token"`
			Usuario user   `json:"usuario"`
			Error   string `json:"error"`
		}
		json.Unmarshal(out, &result)
		if result.Token == "" {
			reply(w, 401, map[string]string{"error": "Usuário ou senha incorretos."})
			return
		}
		sessionCookie(w, r, result.Token, 43200)
		reply(w, 200, result.Usuario)
	})
	mux.HandleFunc("POST /api/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, e := r.Cookie("reembolso_session"); e == nil {
			if _, e = store("logout", map[string]string{"token": c.Value}); e != nil {
				reply(w, 500, map[string]string{"error": "Não foi possível sair. Tente novamente."})
				return
			}
		}
		sessionCookie(w, r, "", -1)
		reply(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, currentUser(r)) })
	mux.HandleFunc("POST /api/senha", func(w http.ResponseWriter, r *http.Request) {
		var d struct {
			Atual string `json:"atual"`
			Nova  string `json:"nova"`
		}
		if !authJSON(w, r, &d) {
			return
		}
		if len([]rune(d.Nova)) < 8 || len([]rune(d.Nova)) > 128 || len(d.Atual) > 512 {
			reply(w, 400, map[string]string{"error": "A nova senha deve ter entre 8 e 128 caracteres."})
			return
		}
		authStore(w, "password", map[string]any{"id": currentUser(r).ID, "atual": d.Atual, "nova": d.Nova}, func() { sessionCookie(w, r, "", -1) })
	})
	mux.HandleFunc("GET /api/admin/usuarios", func(w http.ResponseWriter, r *http.Request) { authStore(w, "users", nil) })
	mux.HandleFunc("POST /api/admin/usuarios", func(w http.ResponseWriter, r *http.Request) {
		var d struct {
			Login  string `json:"login"`
			Senha  string `json:"senha"`
			Perfil string `json:"perfil"`
		}
		if !authJSON(w, r, &d) {
			return
		}
		d.Login = strings.TrimSpace(d.Login)
		if !regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,64}$`).MatchString(d.Login) || len([]rune(d.Senha)) < 8 || len([]rune(d.Senha)) > 128 || (d.Perfil != "admin" && d.Perfil != "normal") {
			reply(w, 400, map[string]string{"error": "Use um login de 3 a 64 letras, números, ponto, traço ou sublinhado, senha de 8 a 128 caracteres e um perfil válido."})
			return
		}
		authStore(w, "create_user", map[string]string{"login": d.Login, "senha": d.Senha, "perfil": d.Perfil})
	})
	mux.HandleFunc("DELETE /api/admin/usuarios/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, e := strconv.Atoi(r.PathValue("id"))
		if e != nil || id < 1 {
			http.NotFound(w, r)
			return
		}
		authStore(w, "delete_user", map[string]int{"id": id, "ator": currentUser(r).ID})
	})
	mux.HandleFunc("DELETE /api/admin/cadastros/{tabela}/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, e := strconv.Atoi(r.PathValue("id"))
		table := r.PathValue("tabela")
		if e != nil || id < 1 || (table != "pessoas" && table != "prestadores" && table != "contas") {
			http.NotFound(w, r)
			return
		}
		authStore(w, "delete_record", map[string]any{"id": id, "tabela": table})
	})
}
func authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bloqueia operações iniciadas por outra origem, inclusive login e exclusões.
		if r.Method != "GET" && r.Method != "HEAD" {
			origin := r.Header.Get("Origin")
			if (origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				reply(w, 403, map[string]string{"error": "Origem inválida."})
				return
			}
		}
		path := r.URL.Path
		if publicPasskeyPath(path) || publicPWAAsset(path) || path == "/login" || path == "/login.html" || path == "/login.js" || path == "/style.css" || path == "/api/login" {
			next.ServeHTTP(w, r)
			return
		}
		var u user
		if c, e := r.Cookie("reembolso_session"); e == nil && len(c.Value) <= 128 {
			out, e := store("session", map[string]string{"token": c.Value})
			if e != nil {
				reply(w, 500, map[string]string{"error": "Não foi possível verificar a sessão."})
				return
			}
			json.Unmarshal(out, &u)
		}
		if u.ID == 0 {
			if strings.HasPrefix(path, "/api/") {
				reply(w, 401, map[string]string{"error": "Entre para continuar."})
			} else {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
			}
			return
		}
		if (strings.HasPrefix(path, "/api/admin/") || path == "/admin" || path == "/admin.html") && u.Perfil != "admin" {
			if strings.HasPrefix(path, "/api/") {
				reply(w, 403, map[string]string{"error": "Acesso restrito a administradores."})
			} else {
				http.Error(w, "Acesso restrito a administradores.", 403)
			}
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	})
}
