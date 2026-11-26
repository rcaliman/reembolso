package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

type redeResult struct {
	Cidades        []string `json:"cidades"`
	Especialidades []string `json:"especialidades"`
	Prestadores    []struct {
		Nome           string   `json:"nome"`
		Cidade         string   `json:"cidade"`
		Especialidades []string `json:"especialidades"`
		Telefones      []string `json:"telefones"`
		Web            []any    `json:"telefones_web"`
	} `json:"prestadores"`
	Total int `json:"total"`
}

func readRede(t *testing.T, city, specialty string) redeResult {
	t.Helper()
	out, err := consultarRede(context.Background(), city, specialty)
	if err != nil {
		t.Fatal(err)
	}
	var result redeResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestRedeFiltersAndContacts(t *testing.T) {
	city := readRede(t, "COLATINA", "")
	if city.Total != 41 {
		t.Fatalf("Colatina: %d", city.Total)
	}
	if len(city.Especialidades) == 0 || len(city.Cidades) < 40 {
		t.Fatal("missing filter options")
	}
	previous := ""
	found := false
	for _, p := range city.Prestadores {
		if p.Cidade != "COLATINA" || p.Nome < previous {
			t.Fatalf("wrong city or order: %s", p.Nome)
		}
		previous = p.Nome
		if p.Nome == "CLINICA CARDIOPED LTDA" {
			found = true
			if len(p.Web) == 0 || len(p.Telefones) != 2 || p.Telefones[1] != "27998291410" {
				t.Fatal("original and web contacts must stay separate")
			}
		}
	}
	if !found {
		t.Fatal("Cardioped missing")
	}
	specialty := readRede(t, "", "CARDIOLOGIA")
	previous = ""
	for _, p := range specialty.Prestadores {
		if p.Cidade < previous {
			t.Fatal("city grouping order")
		}
		previous = p.Cidade
		if !strings.Contains(strings.Join(p.Especialidades, "|"), "CARDIOLOGIA") {
			t.Fatal("specialty mismatch")
		}
	}
	both := readRede(t, "COLATINA", "CARDIOLOGIA")
	if both.Total == 0 || both.Total >= city.Total {
		t.Fatal("combined filters ignored")
	}
	for _, p := range both.Prestadores {
		if p.Cidade != "COLATINA" {
			t.Fatal("wrong city")
		}
	}
	for _, filters := range [][2]string{{"COLATINA", "CARDIO"}, {"' OR 1=1 --", ""}, {"COLATINA", "INEXISTENTE"}} {
		if readRede(t, filters[0], filters[1]).Total != 0 {
			t.Fatal("filter should match exactly and safely")
		}
	}
}
func TestRedeHTTPAndAuthentication(t *testing.T) {
	database = filepath.Join(t.TempDir(), "test.sqlite")
	h := routes()
	for _, path := range []string{"/api/rede-credenciada", "/rede-credenciada"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 && w.Code != 303 {
			t.Fatalf("unprotected %s", path)
		}
	}
	auth := authenticatedHandler(t)
	for _, path := range []string{"/rede-credenciada", "/rede.js", "/api/rede-credenciada?cidade=COLATINA&especialidade=" + url.QueryEscape("CARDIOLOGIA")} {
		w := httptest.NewRecorder()
		auth.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	w := httptest.NewRecorder()
	auth.ServeHTTP(w, httptest.NewRequest("GET", "/api/rede-credenciada?cidade="+strings.Repeat("a", 201), nil))
	if w.Code != 400 {
		t.Fatal("oversize filter accepted")
	}
}
