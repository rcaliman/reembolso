package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func sample() map[string]map[string]string {
	return map[string]map[string]string{
		"solicitante": {"inscricao": "01234567890123456", "nome": "Maria da Silva", "cpf": "529.982.247-25", "email": "maria@example.com", "telefone": "(11) 99999-9999"},
		"dependente":  {"inscricao": "09876543210987654", "nome": "João da Silva", "cpf": "012.345.678-90"},
		"prestador":   {"nome": "Clínica Exemplo", "documento": "11.222.333/0001-81"},
		"conta":       {"tipo": "corrente", "banco": "Banco Exemplo", "agencia": "1234", "numero": "12345-6", "titular": "Maria da Silva", "cpf": "529.982.247-25"},
		"pedido":      {"valor": "250.50", "local": "São Paulo / SP", "data": "2026-10-16"},
	}
}
func TestPedidoPersistenceAndDedup(t *testing.T) {
	database = filepath.Join(t.TempDir(), "test.sqlite")
	handler := authenticatedHandler(t)
	var id float64
	for range 2 {
		body, _ := json.Marshal(sample())
		req := httptest.NewRequest("POST", "/api/pedidos", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != 201 {
			t.Fatalf("save %d: %s", w.Code, w.Body)
		}
		var result map[string]any
		json.Unmarshal(w.Body.Bytes(), &result)
		id = result["id"].(float64)
	}
	if id != 2 {
		t.Fatalf("expected two distinct requests, got %v", id)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/cadastros", nil))
	var lists map[string][]map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &lists); err != nil {
		t.Fatal(err)
	}
	if len(lists["pessoas"]) != 2 || len(lists["prestadores"]) != 1 || len(lists["contas"]) != 1 {
		t.Fatalf("duplicate records: %v", lists)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/pedidos/2", nil))
	var loaded map[string]map[string]string
	json.Unmarshal(w.Body.Bytes(), &loaded)
	if loaded["dependente"]["nome"] != "João da Silva" || loaded["pedido"]["valor"] != "250.50" {
		t.Fatalf("bad persisted request: %s", w.Body)
	}
	for _, url := range []string{"/", "/imprimir", "/formulario.png", "/print.js"} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		if w.Code != 200 {
			t.Fatalf("asset %s: %d", url, w.Code)
		}
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/pedidos/999", nil))
	if w.Code != 404 {
		t.Fatal("expected 404")
	}
}
func TestInvalidRequestsDoNotPersist(t *testing.T) {
	database = filepath.Join(t.TempDir(), "test.sqlite")
	handler := authenticatedHandler(t)
	for _, mutate := range []func(map[string]map[string]string){func(d map[string]map[string]string) { d["pedido"]["valor"] = "0" }, func(d map[string]map[string]string) { d["pedido"]["data"] = "2026-02-31" }, func(d map[string]map[string]string) { d["conta"]["tipo"] = "outro" }, func(d map[string]map[string]string) { d["solicitante"]["nome"] = " " }} {
		d := sample()
		mutate(d)
		body, _ := json.Marshal(d)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/pedidos", bytes.NewReader(body)))
		if w.Code != 400 {
			t.Fatalf("invalid request accepted: %s", w.Body)
		}
	}
	out, e := store("list", nil)
	if e != nil {
		t.Fatal(e)
	}
	var result map[string][]any
	json.Unmarshal(out, &result)
	for _, v := range result {
		if len(v) != 0 {
			t.Fatal("invalid data persisted")
		}
	}
}
