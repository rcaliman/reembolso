package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeletePedidoPermissionsConflictAndAttachments(t *testing.T) {
	database = filepath.Join(t.TempDir(), "pedidos.sqlite")
	h := routes()
	admin := loginTest(t, h, "admin", "admin-test-password")
	request(t, h, "POST", "/api/admin/usuarios", map[string]string{"login": "normal", "senha": "senha-normal", "perfil": "normal"}, admin)
	normal := loginTest(t, h, "normal", "senha-normal")
	snapshot := sample()
	snapshot["nota"] = map[string]string{"arquivo": "nota-test", "paginas": "1", "nome": "nota.pdf"}
	if _, err := store("save", snapshot); err != nil {
		t.Fatal(err)
	}
	dir, valid := invoiceDirectory("nota-test")
	if !valid {
		t.Fatal("valid note rejected")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"original.pdf", "page-1.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	payload := map[string]int{"versao": 0}
	if w := request(t, h, "DELETE", "/api/pedidos/1", payload, nil); w.Code != 401 {
		t.Fatalf("anonymous: %d", w.Code)
	}
	if w := request(t, h, "DELETE", "/api/pedidos/1", payload, normal); w.Code != 403 {
		t.Fatalf("normal user: %d", w.Code)
	}
	r := httptest.NewRequest("DELETE", "/api/pedidos/1", strings.NewReader(`{"versao":0}`))
	r.Header.Set("Origin", "https://outro.example")
	r.AddCookie(admin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin deletion permitted")
	}
	for _, body := range []any{map[string]int{}, map[string]int{"versao": -1}, map[string]string{"versao": "0"}} {
		if w := request(t, h, "DELETE", "/api/pedidos/1", body, admin); w.Code != 400 {
			t.Fatalf("invalid version: %d", w.Code)
		}
	}
	w = request(t, h, "PATCH", "/api/pedidos/1/reembolso", map[string]any{"versao": 0, "valor_reembolsado": "15"}, admin)
	if w.Code != 200 {
		t.Fatalf("update: %s", w.Body)
	}
	if w := request(t, h, "DELETE", "/api/pedidos/1", payload, admin); w.Code != 409 {
		t.Fatalf("stale deletion: %d", w.Code)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("attachment removed on conflict")
	}
	if w := request(t, h, "GET", "/api/pedidos/1", nil, admin); w.Code != 200 {
		t.Fatal("order removed on conflict")
	}
	w = request(t, h, "DELETE", "/api/pedidos/1", map[string]int{"versao": 1}, admin)
	if w.Code != 200 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("note not removed: %v", err)
	}
	for _, path := range []string{"/api/pedidos/1", "/api/pedidos/1/nota", "/api/pedidos/1/nota/paginas/1"} {
		if w := request(t, h, "GET", path, nil, admin); w.Code != 404 {
			t.Fatalf("deleted resource %s: %d", path, w.Code)
		}
	}
	if w := request(t, h, "DELETE", "/api/pedidos/1", payload, admin); w.Code != 404 {
		t.Fatal("repeat deletion should return 404")
	}
	if w := request(t, h, "GET", "/api/pedidos", nil, admin); strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("list: %s", w.Body)
	}
	out, err := store("list", nil)
	if err != nil {
		t.Fatal(err)
	}
	var records map[string][]any
	json.Unmarshal(out, &records)
	for _, table := range []string{"pessoas", "prestadores", "contas"} {
		if len(records[table]) == 0 {
			t.Fatalf("shared data removed: %s", table)
		}
	}
	// Uma migração posterior não reduz a sequência de IDs do banco vazio.
	if _, err := runStore("init", nil); err != nil {
		t.Fatal(err)
	}
	out, err = store("save", sample())
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		ID int `json:"id"`
	}
	json.Unmarshal(out, &saved)
	if saved.ID != 2 {
		t.Fatalf("deleted ID reused: %s", out)
	}
	// Uma resposta atrasada de exclusão não pode apagar o novo pedido.
	if w := request(t, h, "DELETE", "/api/pedidos/1", payload, admin); w.Code != 404 {
		t.Fatal("old identifier reused")
	}
}

func TestDeletePedidoRejectsUnsafeAttachmentPaths(t *testing.T) {
	database = filepath.Join(t.TempDir(), "pedidos.sqlite")
	h := authenticatedHandler(t)
	keep := filepath.Join(filepath.Dir(database), "keep")
	if err := os.Mkdir(keep, 0700); err != nil {
		t.Fatal(err)
	}
	snapshot := sample()
	snapshot["nota"] = map[string]string{"arquivo": "../keep", "paginas": "1", "nome": "nota.pdf"}
	if _, err := store("save", snapshot); err != nil {
		t.Fatal(err)
	}
	w := request(t, h, "DELETE", "/api/pedidos/1", map[string]int{"versao": 0}, nil)
	if w.Code != 200 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("unsafe path removed a different directory")
	}
	for _, folder := range []string{"../keep", "nota-a/../../keep", `nota-a\..\keep`, "/tmp/nota-test", ""} {
		if _, valid := invoiceDirectory(folder); valid {
			t.Fatalf("unsafe folder accepted: %q", folder)
		}
	}
}

func TestDeletePedidoRemovesAllDocuments(t *testing.T) {
	database = filepath.Join(t.TempDir(), "pedidos.sqlite")
	h := routes()
	admin := loginTest(t, h, "admin", "admin-test-password")
	snapshot := sample()
	var dirs []string
	for _, key := range attachmentKeys {
		folder := "nota-" + key
		snapshot[key] = map[string]string{"arquivo": folder, "paginas": "1", "nome": key + ".pdf"}
		dir, _ := invoiceDirectory(folder)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "original.pdf"), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, dir)
	}
	if _, err := store("save", snapshot); err != nil {
		t.Fatal(err)
	}
	w := request(t, h, "DELETE", "/api/pedidos/1", map[string]int{"versao": 0}, admin)
	if w.Code != 200 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("attachment remains: %s (%v)", dir, err)
		}
	}
}
