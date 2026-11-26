package main

import (
	"encoding/json"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPedidosListAndReimbursement(t *testing.T) {
	database = filepath.Join(t.TempDir(), "pedidos.sqlite")
	h := authenticatedHandler(t)
	w := request(t, h, "GET", "/api/pedidos", nil, nil)
	if w.Code != 200 || w.Body.String() != "[]\n" {
		t.Fatalf("empty: %d %s", w.Code, w.Body)
	}
	for _, date := range []string{"2025-12-31", "2026-01-01", "2026-01-01"} {
		d := sample()
		d["pedido"]["data"] = date
		if date == "2025-12-31" {
			delete(d, "dependente")
		}
		w = request(t, h, "POST", "/api/pedidos", d, nil)
		if w.Code != 201 {
			t.Fatalf("save: %s", w.Body)
		}
	}
	var original map[string]any
	w = request(t, h, "GET", "/api/pedidos/1", nil, nil)
	json.Unmarshal(w.Body.Bytes(), &original)
	// A listagem usa as cópias históricas, sem depender dos cadastros atuais.
	if _, err := store("delete_record", map[string]any{"tabela": "pessoas", "id": 1}); err != nil {
		t.Fatal(err)
	}
	w = request(t, h, "GET", "/api/pedidos", nil, nil)
	var rows []struct {
		ID          int               `json:"id"`
		Data        string            `json:"data"`
		Valor       string            `json:"valor"`
		Reembolso   *int64            `json:"reembolsado_centavos"`
		Solicitante map[string]string `json:"solicitante"`
		Dependente  map[string]string `json:"dependente"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].ID != 3 || rows[1].ID != 2 || rows[2].ID != 1 || rows[2].Data != "2025-12-31" || rows[2].Dependente != nil || rows[0].Reembolso != nil || rows[0].Valor != "250.50" || rows[0].Solicitante["nome"] != "Maria da Silva" {
		t.Fatalf("list: %s", w.Body)
	}
	for version, value := range []string{"123.45", "0", "999999999.99", ""} {
		w = request(t, h, "PATCH", "/api/pedidos/1/reembolso", map[string]any{"valor_reembolsado": value, "versao": version}, nil)
		if w.Code != 200 {
			t.Fatalf("update %q: %d %s", value, w.Code, w.Body)
		}
		w = request(t, h, "GET", "/api/pedidos", nil, nil)
		json.Unmarshal(w.Body.Bytes(), &rows)
		got := rows[2].Reembolso
		expected := map[string]int64{"123.45": 12345, "0": 0, "999999999.99": 99999999999}
		if value == "" {
			if got != nil {
				t.Fatal("clear failed")
			}
		} else if got == nil || *got != expected[value] {
			t.Fatalf("cents: %v", got)
		}
	}

	w = request(t, h, "PATCH", "/api/pedidos/1/reembolso", map[string]any{"valor_reembolsado": "50", "versao": 0}, nil)
	var conflict map[string]any
	json.Unmarshal(w.Body.Bytes(), &conflict)
	if w.Code != 409 || conflict["reembolso_versao"] != float64(4) || conflict["reembolsado_centavos"] != nil {
		t.Fatalf("stale update: %d %s", w.Code, w.Body)
	}
	w = request(t, h, "PATCH", "/api/pedidos/1/reembolso", map[string]string{"valor_reembolsado": "50"}, nil)
	if w.Code != 400 {
		t.Fatal("missing version accepted")
	}
	for _, value := range []any{"-1", "1.001", "NaN", "1000000000", "1,50", 12.5, nil} {
		w = request(t, h, "PATCH", "/api/pedidos/1/reembolso", map[string]any{"valor_reembolsado": value}, nil)
		if w.Code != 400 {
			t.Fatalf("invalid value %v: %d", value, w.Code)
		}
	}
	w = request(t, h, "PATCH", "/api/pedidos/999/reembolso", map[string]any{"valor_reembolsado": "0", "versao": 0}, nil)
	if w.Code != 404 {
		t.Fatal("missing order accepted")
	}
	w = request(t, h, "GET", "/api/pedidos/1", nil, nil)
	var after map[string]any
	json.Unmarshal(w.Body.Bytes(), &after)
	beforeJSON, _ := json.Marshal(original)
	afterJSON, _ := json.Marshal(after)
	if string(beforeJSON) != string(afterJSON) {
		t.Fatal("reimbursement changed original print data")
	}
	for _, path := range []string{"/pedidos", "/pedidos.js"} {
		w = request(t, h, "GET", path, nil, nil)
		if w.Code != 200 {
			t.Fatalf("asset %s: %d", path, w.Code)
		}
	}
	// Mantém a mesma permissão dos demais pedidos compartilhados.
	raw := routes()
	for _, method := range []string{"GET", "PATCH"} {
		path := "/api/pedidos"
		if method == "PATCH" {
			path += "/1/reembolso"
		}
		w = request(t, raw, method, path, map[string]string{"valor_reembolsado": "1"}, nil)
		if w.Code != 401 {
			t.Fatal("anonymous access allowed")
		}
	}
	request(t, h, "POST", "/api/admin/usuarios", map[string]string{"login": "normal", "senha": "senha12345", "perfil": "normal"}, nil)
	cookie := loginTest(t, raw, "normal", "senha12345")
	w = request(t, raw, "PATCH", "/api/pedidos/1/reembolso", map[string]any{"valor_reembolsado": "10.01", "versao": 4}, cookie)
	if w.Code != 200 {
		t.Fatalf("normal user: %d %s", w.Code, w.Body)
	}
	req := httptest.NewRequest("PATCH", "/api/pedidos/1/reembolso", nil)
	req.Header.Set("Origin", "https://outro.example")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("cross origin write allowed")
	}
}

func TestLegacyReimbursementMigration(t *testing.T) {
	database = filepath.Join(t.TempDir(), "legacy.sqlite")
	snapshot, _ := json.Marshal(sample())
	cmd := exec.Command("python3", "-c", `import sqlite3,sys
con=sqlite3.connect(sys.argv[1])
con.execute('CREATE TABLE pedidos(id INTEGER PRIMARY KEY, criado_em TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, dados TEXT NOT NULL)')
con.execute('INSERT INTO pedidos(dados) VALUES (?)',(sys.argv[2],))
con.commit()`, database, string(snapshot))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s %v", out, err)
	}
	for range 2 {
		out, err := store("list_pedidos", nil)
		if err != nil {
			t.Fatal(err)
		}
		var rows []map[string]any
		json.Unmarshal(out, &rows)
		if len(rows) != 1 || rows[0]["reembolsado_centavos"] != nil || rows[0]["valor"] != "250.50" || rows[0]["observacoes"] != "" {
			t.Fatalf("migration: %s", out)
		}
	}
}

func TestPedidoObservacoes(t *testing.T) {
	database = filepath.Join(t.TempDir(), "notes.sqlite")
	h := authenticatedHandler(t)
	request(t, h, "POST", "/api/pedidos", sample(), nil)
	notes := "Problema no recebimento\nEnviar documento <pendente>" + strings.Repeat("界", 1900)
	for version, payload := range []map[string]any{
		{"valor_reembolsado": "", "observacoes": notes},
		{"valor_reembolsado": "10.00"},
		{"valor_reembolsado": "10.00", "observacoes": ""},
	} {
		payload["versao"] = version
		w := request(t, h, "PATCH", "/api/pedidos/1/reembolso", payload, nil)
		if w.Code != 200 {
			t.Fatalf("save: %d %s", w.Code, w.Body)
		}
		w = request(t, h, "GET", "/api/pedidos", nil, nil)
		var rows []map[string]any
		json.Unmarshal(w.Body.Bytes(), &rows)
		expected := notes
		if version == 2 {
			expected = ""
		}
		if rows[0]["observacoes"] != expected {
			t.Fatalf("notes: %s", w.Body)
		}
	}
	w := request(t, h, "PATCH", "/api/pedidos/1/reembolso", map[string]any{"valor_reembolsado": "", "observacoes": "rascunho", "versao": 0}, nil)
	var conflict map[string]any
	json.Unmarshal(w.Body.Bytes(), &conflict)
	if w.Code != 409 || conflict["observacoes"] != "" {
		t.Fatalf("conflict: %d %s", w.Code, w.Body)
	}
	for _, invalid := range []any{strings.Repeat("a", 2001), 123} {
		w = request(t, h, "PATCH", "/api/pedidos/1/reembolso", map[string]any{"valor_reembolsado": "", "observacoes": invalid, "versao": 3}, nil)
		if w.Code != 400 {
			t.Fatalf("invalid notes: %d %s", w.Code, w.Body)
		}
	}
}
