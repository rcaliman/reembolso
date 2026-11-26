package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderDescriptionMigrationAndUpdate(t *testing.T) {
	database = filepath.Join(t.TempDir(), "legacy.sqlite")
	cmd := exec.Command("python3", "-c", `import sqlite3,sys
con=sqlite3.connect(sys.argv[1])
con.execute('CREATE TABLE prestadores(id INTEGER PRIMARY KEY,nome TEXT NOT NULL,documento TEXT NOT NULL,UNIQUE(nome,documento))')
con.execute('INSERT INTO prestadores VALUES (7,?,?)',('Clínica Exemplo','11.222.333/0001-81'))
con.commit()`, database)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s %v", out, err)
	}
	check := func(want string) {
		t.Helper()
		out, err := store("list", nil)
		if err != nil {
			t.Fatal(err)
		}
		var lists map[string][]map[string]any
		if err = json.Unmarshal(out, &lists); err != nil {
			t.Fatal(err)
		}
		rows := lists["prestadores"]
		if len(rows) != 1 || rows[0]["id"] != float64(7) || rows[0]["descricao"] != want {
			t.Fatalf("unexpected providers: %v", rows)
		}
	}
	check("")
	check("") // Migração pode ser executada novamente.
	d := sample()
	for _, description := range []string{"Dra. Ana — cardiologista", "Dr. João", ""} {
		d["prestador"]["descricao"] = " " + description + " "
		savePeopleRequest(t, d)
		check(description)
	}
	out, err := store("get", map[string]int{"id": 1})
	if err != nil {
		t.Fatal(err)
	}
	var original map[string]map[string]string
	if err = json.Unmarshal(out, &original); err != nil {
		t.Fatal(err)
	}
	if original["prestador"]["nome"] != "Clínica Exemplo" || original["prestador"]["descricao"] != "Dra. Ana — cardiologista" {
		t.Fatal("original request changed")
	}
}

func TestProviderDescriptionValidation(t *testing.T) {
	for _, description := range []string{strings.Repeat("Á", 121), "Dra. Ana\nCardiologia"} {
		d := sample()
		d["prestador"]["descricao"] = description
		if _, err := validate(d); err == nil {
			t.Fatalf("accepted invalid description: %q", description)
		}
	}
}
