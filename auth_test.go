package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	os.Setenv("REEMBOLSO_ADMIN_PASSWORD", "admin-test-password")
	os.Exit(m.Run())
}

func request(t *testing.T, h http.Handler, method, path string, data any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if data != nil {
		if e := json.NewEncoder(&body).Encode(data); e != nil {
			t.Fatal(e)
		}
	}
	req := httptest.NewRequest(method, path, &body)
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
func loginTest(t *testing.T, h http.Handler, name, password string) *http.Cookie {
	t.Helper()
	w := request(t, h, "POST", "/api/login", map[string]string{"login": name, "senha": password}, nil)
	if w.Code != 200 {
		t.Fatalf("login %d: %s", w.Code, w.Body)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("missing secure cookie attributes")
	}
	return cookies[0]
}
func authenticatedHandler(t *testing.T) http.Handler {
	t.Helper()
	h := routes()
	cookie := loginTest(t, h, "admin", "admin-test-password")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.AddCookie(cookie); h.ServeHTTP(w, r) })
}
func TestAuthenticationAndPermissions(t *testing.T) {
	database = filepath.Join(t.TempDir(), "auth.sqlite")
	h := routes()
	for _, path := range []string{"/api/cadastros", "/api/pedidos/1", "/api/pedidos/1/nota", "/api/admin/usuarios"} {
		w := request(t, h, "GET", path, nil, nil)
		if w.Code != 401 {
			t.Fatalf("anonymous access %s: %d", path, w.Code)
		}
	}
	w := request(t, h, "GET", "/", nil, nil)
	if w.Code != 303 || w.Header().Get("Location") != "/login" {
		t.Fatal("missing login redirect")
	}
	w = request(t, h, "GET", "/login", nil, nil)
	if w.Code != 200 {
		t.Fatal("login page inaccessible")
	}
	w = request(t, h, "POST", "/api/login", map[string]string{"login": "admin", "senha": "wrong"}, nil)
	if w.Code != 401 {
		t.Fatal("incorrect password accepted")
	}
	admin := loginTest(t, h, "admin", "admin-test-password")
	w = request(t, h, "POST", "/api/admin/usuarios", map[string]string{"login": "maria", "senha": "senha-forte123", "perfil": "normal"}, admin)
	if w.Code != 200 {
		t.Fatalf("create %s", w.Body)
	}
	w = request(t, h, "POST", "/api/admin/usuarios", map[string]string{"login": "MARIA", "senha": "senha-forte123", "perfil": "normal"}, admin)
	if w.Code != 409 {
		t.Fatal("duplicate login accepted")
	}
	normal := loginTest(t, h, "maria", "senha-forte123")
	for _, op := range []struct{ method, path string }{{"GET", "/api/admin/usuarios"}, {"POST", "/api/admin/usuarios"}, {"DELETE", "/api/admin/usuarios/1"}, {"DELETE", "/api/admin/cadastros/pessoas/1"}, {"DELETE", "/api/admin/cadastros/prestadores/1"}, {"DELETE", "/api/admin/cadastros/contas/1"}, {"GET", "/admin"}} {
		w = request(t, h, op.method, op.path, nil, normal)
		if w.Code != 403 {
			t.Fatalf("normal authorized %s %s: %d", op.method, op.path, w.Code)
		}
	}
	w = request(t, h, "POST", "/api/pedidos", sample(), normal)
	if w.Code != 201 {
		t.Fatalf("normal request denied: %s", w.Body)
	}
	for _, path := range []string{"/api/admin/cadastros/pessoas/1", "/api/admin/cadastros/prestadores/1", "/api/admin/cadastros/contas/1"} {
		w = request(t, h, "DELETE", path, nil, admin)
		if w.Code != 200 {
			t.Fatalf("delete %s: %s", path, w.Body)
		}
	}
	w = request(t, h, "GET", "/api/pedidos/1", nil, normal)
	var snapshot map[string]map[string]string
	json.Unmarshal(w.Body.Bytes(), &snapshot)
	if snapshot["solicitante"]["nome"] != "Maria da Silva" {
		t.Fatal("deletion changed saved request")
	}

	if snapshot["conta"]["numero"] != sample()["conta"]["numero"] {
		t.Fatal("account deletion changed saved request")
	}
	w = request(t, h, "GET", "/api/cadastros", nil, normal)
	var records struct {
		Contas []json.RawMessage `json:"contas"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &records); err != nil {
		t.Fatal(err)
	}
	if len(records.Contas) != 0 {
		t.Fatal("deleted account still listed")
	}
	w = request(t, h, "DELETE", "/api/admin/cadastros/contas/1", nil, admin)
	if w.Code != 404 {
		t.Fatal("repeated account deletion should return 404")
	}
	w = request(t, h, "DELETE", "/api/admin/usuarios/1", nil, admin)
	if w.Code != 400 {
		t.Fatal("self deletion accepted")
	}
	w = request(t, h, "DELETE", "/api/admin/usuarios/2", nil, admin)
	if w.Code != 200 {
		t.Fatalf("delete user: %s", w.Body)
	}
	w = request(t, h, "GET", "/api/me", nil, normal)
	if w.Code != 401 {
		t.Fatal("deleted user's session active")
	}
}
func TestPasswordChangeAndLogout(t *testing.T) {
	database = filepath.Join(t.TempDir(), "auth.sqlite")
	h := routes()
	admin := loginTest(t, h, "admin", "admin-test-password")
	other := loginTest(t, h, "admin", "admin-test-password")
	w := request(t, h, "POST", "/api/senha", map[string]string{"atual": "bad", "nova": "nova-senha123"}, admin)
	if w.Code != 400 {
		t.Fatal("wrong current password accepted")
	}
	w = request(t, h, "POST", "/api/senha", map[string]string{"atual": "admin-test-password", "nova": "nova-senha123"}, admin)
	if w.Code != 200 {
		t.Fatalf("password change: %s", w.Body)
	}
	if cookies := w.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatal("password change must clear cookie")
	}
	for _, c := range []*http.Cookie{admin, other} {
		w = request(t, h, "GET", "/api/me", nil, c)
		if w.Code != 401 {
			t.Fatal("password change did not revoke sessions")
		}
	}
	w = request(t, h, "POST", "/api/login", map[string]string{"login": "admin", "senha": "admin-test-password"}, nil)
	if w.Code != 401 {
		t.Fatal("default password was recreated")
	}
	admin = loginTest(t, h, "admin", "nova-senha123")
	w = request(t, h, "POST", "/api/logout", nil, admin)
	if w.Code != 200 {
		t.Fatal("logout failed")
	}
	w = request(t, h, "GET", "/api/me", nil, admin)
	if w.Code != 401 {
		t.Fatal("logout did not revoke session")
	}
}
func TestAuthCrossOrigin(t *testing.T) {
	database = filepath.Join(t.TempDir(), "auth.sqlite")
	h := routes()
	admin := loginTest(t, h, "admin", "admin-test-password")
	for _, path := range []string{"/api/login", "/api/senha", "/api/admin/usuarios"} {
		r := httptest.NewRequest("POST", path, bytes.NewBufferString(`{}`))
		r.Header.Set("Origin", "https://other.example")
		r.AddCookie(admin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("cross origin accepted: %s", path)
		}
	}
}

func TestMigrationPreservesExistingRequests(t *testing.T) {
	database = filepath.Join(t.TempDir(), "existing.sqlite")
	raw, _ := json.Marshal(sample())
	cmd := exec.Command("python3", "-c", `import sqlite3,sys
con=sqlite3.connect(sys.argv[1])
con.execute("CREATE TABLE pedidos(id INTEGER PRIMARY KEY, criado_em TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, dados TEXT NOT NULL)")
con.execute("INSERT INTO pedidos(dados) VALUES (?)",(sys.stdin.read(),))
con.commit()`, database)
	cmd.Stdin = bytes.NewReader(raw)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("migration fixture: %s %v", out, e)
	}
	h := routes()
	cookie := loginTest(t, h, "admin", "admin-test-password")
	w := request(t, h, "GET", "/api/pedidos/1", nil, cookie)
	var data map[string]map[string]string
	json.Unmarshal(w.Body.Bytes(), &data)
	if w.Code != 200 || data["solicitante"]["nome"] != "Maria da Silva" {
		t.Fatalf("old data lost: %s", w.Body)
	}
}
