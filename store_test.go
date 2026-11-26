package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestStoreReadsDuringWrite(t *testing.T) {
	database = filepath.Join(t.TempDir(), "readers.sqlite")
	if err := initializeStore(); err != nil {
		t.Fatal(err)
	}
	script, err := assets.ReadFile("store.py")
	if err != nil {
		t.Fatal(err)
	}
	// Uma escrita aberta não deve bloquear consultas em WAL, inclusive a sessão.
	cmd := exec.Command("python3", "-c", `import sqlite3,sys,subprocess,json
con=sqlite3.connect(sys.argv[1])
con.execute('BEGIN IMMEDIATE')
try:
    for action,data in [('list_pedidos',None),('pedido_filters',None),('session',{'token':'inexistente'})]:
        result=subprocess.run(['python3','-c',sys.argv[2],sys.argv[1],action],input=json.dumps(data),text=True,capture_output=True,timeout=3)
        assert result.returncode==0,result.stderr
        json.loads(result.stdout)
finally:
    con.rollback()
    con.close()
`, database, string(script))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("read during write: %v\n%s", err, out)
	}
}

func TestPedidoYearQueryAndHistoricalFilters(t *testing.T) {
	database = filepath.Join(t.TempDir(), "years.sqlite")
	h := authenticatedHandler(t)
	for _, date := range []string{"2025-12-31", "2026-01-01", "2026-02-01"} {
		d := sample()
		d["pedido"]["data"] = date
		if date == "2025-12-31" {
			d["solicitante"]["nome"] = "Pessoa antiga"
			delete(d, "dependente")
		}
		savePeopleRequest(t, d)
	}
	for _, tc := range []struct {
		year  string
		count int
	}{{"2025", 1}, {"2026", 2}, {"2027", 0}} {
		w := request(t, h, "GET", "/api/pedidos?ano="+tc.year, nil, nil)
		var rows []map[string]any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &rows) != nil || len(rows) != tc.count {
			t.Fatalf("year %s: %d %s", tc.year, w.Code, w.Body)
		}
		for _, row := range rows {
			if row["data"].(string)[:4] != tc.year {
				t.Fatal("another year leaked")
			}
		}
	}
	for _, year := range []string{"abc", "26", "2026-01", "2026%27"} {
		w := request(t, h, "GET", "/api/pedidos?ano="+year, nil, nil)
		if w.Code != 400 {
			t.Fatalf("invalid year %s: %d", year, w.Code)
		}
	}
	if _, err := store("delete_record", map[string]any{"tabela": "pessoas", "id": 1}); err != nil {
		t.Fatal(err)
	}
	w := request(t, h, "GET", "/api/pedidos/filtros", nil, nil)
	var filters struct {
		Anos        []string            `json:"anos"`
		Solicitante []map[string]string `json:"solicitante"`
		Dependente  []map[string]string `json:"dependente"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &filters) != nil || len(filters.Anos) != 2 || filters.Anos[0] != "2026" || len(filters.Solicitante) != 2 || len(filters.Dependente) != 1 {
		t.Fatalf("filters: %d %s", w.Code, w.Body)
	}
	found := false
	for _, p := range filters.Solicitante {
		if p["nome"] == "Pessoa antiga" {
			found = true
		}
	}
	if !found {
		t.Fatal("deleted historical requester missing")
	}
	w = request(t, routes(), "GET", "/api/pedidos/filtros", nil, nil)
	if w.Code != 401 {
		t.Fatal("anonymous filters access")
	}
	// Inicialização repetida preserva os pedidos e o índice da consulta anual.
	for range 2 {
		if _, err := runStore("init", nil); err != nil {
			t.Fatal(err)
		}
	}
	script := `import sqlite3,sys
con=sqlite3.connect(sys.argv[1])
assert con.execute('SELECT COUNT(*) FROM pedidos').fetchone()[0]==3
plan=con.execute("EXPLAIN QUERY PLAN SELECT * FROM pedidos WHERE substr(json_extract(dados,'$.pedido.data'),1,4)=?",('2026',)).fetchall()
assert any('pedidos_ano' in str(row) for row in plan),plan
`
	if out, err := exec.Command("python3", "-c", script, database).CombinedOutput(); err != nil {
		t.Fatalf("index or initialization: %v %s", err, out)
	}
}

func TestMoneyCents(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
	}{{"0", 0}, {"0.01", 1}, {"000250.5", 25050}, {"999999999.99", 99999999999}} {
		got, err := moneyCents(tc.value)
		if err != nil || got != tc.want {
			t.Fatalf("money %q: %d %v", tc.value, got, err)
		}
	}
	for _, value := range []string{"", "-1", "1.001", "1000000000", "1,20", "NaN"} {
		if _, err := moneyCents(value); err == nil {
			t.Fatalf("invalid money accepted: %s", value)
		}
	}
}
