package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

func listedPeople(t *testing.T) []map[string]any {
	t.Helper()
	out, err := store("list", nil)
	if err != nil {
		t.Fatal(err)
	}
	var lists map[string][]map[string]any
	if err = json.Unmarshal(out, &lists); err != nil {
		t.Fatal(err)
	}
	return lists["pessoas"]
}
func savePeopleRequest(t *testing.T, d map[string]map[string]string) {
	t.Helper()
	validated, err := validate(d)
	if err != nil {
		t.Fatal(err)
	}
	out, err := store("save", validated)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ID int `json:"id"`
	}
	if json.Unmarshal(out, &result) != nil || result.ID < 1 {
		t.Fatalf("request was not saved: %s", out)
	}
}
func TestPeopleSharedAcrossRolesAndContactsPreserved(t *testing.T) {
	database = filepath.Join(t.TempDir(), "test.sqlite")
	d := sample()
	delete(d, "dependente")
	savePeopleRequest(t, d)
	// A antiga solicitante passa a ser dependente sem enviar seus contatos.
	next := sample()
	next["dependente"] = map[string]string{"nome": d["solicitante"]["nome"], "cpf": d["solicitante"]["cpf"], "inscricao": d["solicitante"]["inscricao"]}
	next["solicitante"] = map[string]string{"nome": "Outra Pessoa", "cpf": "012.345.678-90", "inscricao": "000001", "email": "", "telefone": ""}
	savePeopleRequest(t, next)
	people := listedPeople(t)
	if len(people) != 2 {
		t.Fatalf("duplicate across roles: %v", people)
	}
	for _, p := range people {
		if _, exists := p["tipo"]; exists {
			t.Fatal("person still has role")
		}
		if p["nome"] == "Maria da Silva" && (p["email"] != "maria@example.com" || p["telefone"] != "(11) 99999-9999") {
			t.Fatal("dependent erased contact details")
		}
	}
	// Um dependente pode depois ser solicitante e informar seus contatos.
	next["solicitante"]["email"] = "outra@example.com"
	next["solicitante"]["telefone"] = "(11) 3234-5678"
	savePeopleRequest(t, next)
	people = listedPeople(t)
	if len(people) != 2 {
		t.Fatal("contact update duplicated person")
	}
	for _, p := range people {
		if p["nome"] == "Outra Pessoa" && (p["email"] != "outra@example.com" || p["telefone"] != "(11) 3234-5678") {
			t.Fatal("contact update missing")
		}
	}
	out, err := store("get", map[string]int{"id": 1})
	if err != nil {
		t.Fatal(err)
	}
	var original map[string]map[string]string
	json.Unmarshal(out, &original)
	if original["solicitante"]["email"] != "maria@example.com" || original["dependente"] != nil {
		t.Fatal("original request changed")
	}
}
func TestLegacyPeopleMigration(t *testing.T) {
	database = filepath.Join(t.TempDir(), "legacy.sqlite")
	original, _ := json.Marshal(sample())
	script := `import sqlite3,sys
con=sqlite3.connect(sys.argv[1])
con.executescript('''CREATE TABLE pessoas(id INTEGER PRIMARY KEY, tipo TEXT NOT NULL, inscricao TEXT NOT NULL, nome TEXT NOT NULL, email TEXT NOT NULL, telefone TEXT NOT NULL, UNIQUE(tipo,inscricao,nome,email,telefone));
CREATE TABLE pedidos(id INTEGER PRIMARY KEY,criado_em TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,dados TEXT NOT NULL);''')
con.executemany('INSERT INTO pessoas VALUES (?,?,?,?,?,?)',[
(1,'dependente','000123','Pessoa Unificada','',''),
(2,'solicitante','000123','Pessoa Unificada','antigo@example.com','(11) 3234-5678'),
(3,'solicitante','000123','Pessoa Unificada','',''),
(4,'solicitante','000123','Pessoa Unificada','atual@example.com',''),
(5,'dependente','000456','Outra Pessoa','','')])
con.execute('INSERT INTO pedidos(dados) VALUES (?)',(sys.stdin.read(),))
con.commit()`
	cmd := exec.Command("python3", "-c", script, database)
	cmd.Stdin = bytes.NewReader(original)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("legacy fixture: %s %v", out, err)
	}
	for range 2 {
		people := listedPeople(t)
		if len(people) != 2 {
			t.Fatalf("migration lost people or did not merge: %v", people)
		}
		for _, p := range people {
			if p["nome"] == "Pessoa Unificada" {
				if p["id"].(float64) != 1 || p["email"] != "atual@example.com" || p["telefone"] != "(11) 3234-5678" {
					t.Fatalf("bad merge: %v", p)
				}
			}
		}
	}
	out, err := store("get", map[string]int{"id": 1})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]map[string]string
	json.Unmarshal(out, &snapshot)
	if snapshot["solicitante"]["inscricao"] != sample()["solicitante"]["inscricao"] {
		t.Fatal("migration changed previous request")
	}
}

func TestSecondaryEmail(t *testing.T) {
	database = filepath.Join(t.TempDir(), "secondary.sqlite")
	d := sample()
	delete(d, "dependente")
	d["solicitante"]["email_secundario"] = " secundario@example.com "
	savePeopleRequest(t, d)
	if got := listedPeople(t)[0]["email_secundario"]; got != "secundario@example.com" {
		t.Fatalf("secondary email not saved: %v", got)
	}
	out, err := store("get", map[string]int{"id": 1})
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]map[string]string
	if err := json.Unmarshal(out, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["solicitante"]["email_secundario"] != "secundario@example.com" {
		t.Fatal("secondary email missing from request")
	}
	d["solicitante"]["email_secundario"] = "invalido"
	if _, err := validate(d); err == nil {
		t.Fatal("invalid secondary email accepted")
	}
	d["solicitante"]["email_secundario"] = ""
	savePeopleRequest(t, d)
	if got := listedPeople(t)[0]["email_secundario"]; got != "" {
		t.Fatalf("secondary email not cleared: %v", got)
	}
}

func TestPersonCPFRequiredAndSaved(t *testing.T) {
	database = filepath.Join(t.TempDir(), "cpf.sqlite")
	d := sample()
	d["solicitante"]["cpf"] = "52998224725"
	savePeopleRequest(t, d)
	for _, person := range listedPeople(t) {
		if person["nome"] == "Maria da Silva" && person["cpf"] != "529.982.247-25" {
			t.Fatal("CPF not normalized and saved")
		}
	}
	for _, role := range []string{"solicitante", "dependente"} {
		for _, cpf := range []string{"", "12345678900"} {
			invalid := sample()
			invalid[role]["cpf"] = cpf
			if _, err := validate(invalid); err == nil {
				t.Fatalf("accepted missing/invalid CPF for %s", role)
			}
		}
	}
	delete(d, "dependente")
	if _, err := validate(d); err != nil {
		t.Fatalf("dependent CPF required without dependent: %v", err)
	}
}

func TestPeopleIdentityUsesCPFAndRegistration(t *testing.T) {
	database = filepath.Join(t.TempDir(), "identity.sqlite")
	d := sample()
	delete(d, "dependente")
	savePeopleRequest(t, d)
	original := listedPeople(t)[0]
	d["solicitante"]["nome"] = "MARIA DA SILVA"
	savePeopleRequest(t, d)
	if people := listedPeople(t); len(people) != 1 || people[0]["id"] != original["id"] {
		t.Fatalf("case edit duplicated identity: %v", people)
	}
	d["solicitante"]["nome"] = "Maria da Silva Souza"
	savePeopleRequest(t, d)
	if people := listedPeople(t); len(people) != 1 || people[0]["id"] != original["id"] {
		t.Fatalf("name correction duplicated identity: %v", people)
	}
	d["solicitante"]["inscricao"] = "99999"
	savePeopleRequest(t, d)
	if len(listedPeople(t)) != 2 {
		t.Fatal("different registrations must remain available")
	}
	out, err := store("get", map[string]int{"id": 1})
	if err != nil || !bytes.Contains(out, []byte("Maria da Silva")) || bytes.Contains(out, []byte("Souza")) {
		t.Fatal("historical name was changed")
	}
}

func TestConflictingCPFRollsBackWholeRequest(t *testing.T) {
	database = filepath.Join(t.TempDir(), "identity-conflict.sqlite")
	d := sample()
	delete(d, "dependente")
	savePeopleRequest(t, d)
	next := sample()
	next["solicitante"]["nome"] = "Nova Pessoa"
	next["solicitante"]["inscricao"] = "99999"
	next["dependente"] = map[string]string{"nome": d["solicitante"]["nome"], "inscricao": d["solicitante"]["inscricao"], "cpf": "012.345.678-90"}
	validated, err := validate(next)
	if err != nil {
		t.Fatal(err)
	}
	out, err := store("save", validated)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status int `json:"status"`
	}
	json.Unmarshal(out, &result)
	if result.Status != 409 {
		t.Fatalf("conflicting identity accepted: %s", out)
	}
	people := listedPeople(t)
	if len(people) != 1 || people[0]["cpf"] != d["solicitante"]["cpf"] {
		t.Fatalf("partial request persisted: %v", people)
	}
	out, err = store("get", map[string]int{"id": 2})
	if err != nil || string(bytes.TrimSpace(out)) != "null" {
		t.Fatalf("conflicting request persisted: %s", out)
	}
}

func TestCPFIdentityMigrationPreservesHistoryAndConflicts(t *testing.T) {
	database = filepath.Join(t.TempDir(), "identity-migration.sqlite")
	original, _ := json.Marshal(sample())
	script := `import sqlite3,sys
con=sqlite3.connect(sys.argv[1])
con.executescript('''CREATE TABLE pessoas(id INTEGER PRIMARY KEY, inscricao TEXT NOT NULL, nome TEXT NOT NULL, email TEXT NOT NULL, telefone TEXT NOT NULL, cpf TEXT NOT NULL, email_secundario TEXT NOT NULL, UNIQUE(inscricao,nome));
CREATE TABLE pedidos(id INTEGER PRIMARY KEY,criado_em TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,dados TEXT NOT NULL);''')
con.executemany('INSERT INTO pessoas VALUES (?,?,?,?,?,?,?)',[
(1,'001','Maria da Silva','antigo@example.com','','529.982.247-25',''),
(2,'001','MARIA DA SILVA','novo@example.com','(11) 3234-5678','52998224725',''),
(3,'001','Mária da Silva','','','','secundario@example.com'),
(4,'002','Maria da Silva','','','529.982.247-25',''),
(5,'003','Pessoa em conflito','','','529.982.247-25',''),
(6,'003','PESSOA EM CONFLITO','','','012.345.678-90','')])
con.execute('INSERT INTO pedidos(dados) VALUES (?)',(sys.stdin.read(),))
con.commit()`
	cmd := exec.Command("python3", "-c", script, database)
	cmd.Stdin = bytes.NewReader(original)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s %v", out, err)
	}
	for i := 0; i < 2; i++ {
		if _, err := runStore("init", map[string]string{"admin_password": "migration-password"}); err != nil {
			t.Fatal(err)
		}
		people := listedPeople(t)
		if len(people) != 4 {
			t.Fatalf("migration lost conflicts or failed to merge: %v", people)
		}
		for _, p := range people {
			if p["id"] == float64(1) && (p["email"] != "novo@example.com" || p["email_secundario"] != "secundario@example.com" || p["telefone"] != "(11) 3234-5678") {
				t.Fatalf("contacts lost: %v", p)
			}
		}
	}
	out, err := store("get", map[string]int{"id": 1})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]map[string]string
	json.Unmarshal(out, &got)
	if got["solicitante"]["cpf"] != sample()["solicitante"]["cpf"] {
		t.Fatal("historical CPF changed")
	}
}

func TestConcurrentPeopleCPFConflictIsAtomic(t *testing.T) {
	database = filepath.Join(t.TempDir(), "concurrent-cpf.sqlite")
	if err := initializeStore(); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		raw []byte
		err error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	for _, cpf := range []string{"529.982.247-25", "012.345.678-90"} {
		d := sample()
		delete(d, "dependente")
		d["solicitante"]["cpf"] = cpf
		go func(data map[string]map[string]string) {
			<-start
			out, err := store("save", data)
			results <- outcome{out, err}
		}(d)
	}
	close(start)
	saved, conflicted := 0, 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		var response struct {
			ID     int `json:"id"`
			Status int `json:"status"`
		}
		if err := json.Unmarshal(result.raw, &response); err != nil {
			t.Fatal(err)
		}
		if response.ID > 0 {
			saved++
		}
		if response.Status == 409 {
			conflicted++
		}
	}
	if saved != 1 || conflicted != 1 || len(listedPeople(t)) != 1 {
		t.Fatalf("concurrent identity conflict not atomic: saved=%d conflicts=%d", saved, conflicted)
	}
}
