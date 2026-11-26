package main

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDocumentValidation(t *testing.T) {
	for _, s := range []string{"52998224725", "529.982.247-25", "01234567890"} {
		if !validCPF(s) {
			t.Errorf("valid CPF rejected: %s", s)
		}
	}
	for _, s := range []string{"", "11111111111", "00000000000", "529.982.247-24", "5299822472", "529982247250", "529a98224725", "529..982.247-25"} {
		if validCPF(s) {
			t.Errorf("invalid CPF accepted: %s", s)
		}
	}
	for _, s := range []string{"11222333000181", "11.222.333/0001-81", "04.252.011/0001-10", "12.ABC.345/01DE-35", "12abc34501de35", "00.000.000/E08G-12"} {
		if !validCNPJ(s) {
			t.Errorf("valid CNPJ rejected: %s", s)
		}
	}
	for _, s := range []string{"00000000000000", "11111111111111", "11.222.333/0001-80", "12.ABC.345/01DE-34", "12.ABC.345/01DE-3A", "12_ABC34501DE35", "12.ÁBC.345/01DE-35"} {
		if validCNPJ(s) {
			t.Errorf("invalid CNPJ accepted: %s", s)
		}
	}
}
func TestFormNormalization(t *testing.T) {
	data := sample()
	data["solicitante"]["inscricao"] = "00001234"
	data["solicitante"]["telefone"] = "11999999999"
	data["prestador"]["documento"] = "12abc34501de35"
	data["conta"]["cpf"] = "52998224725"
	data["conta"]["agencia"] = "0001-x"
	data["conta"]["numero"] = "0000123-x"
	data["pedido"]["valor"] = "000250.5"
	normalized, err := validate(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ section, key, want string }{{"solicitante", "inscricao", "00001234"}, {"solicitante", "telefone", "(11) 99999-9999"}, {"prestador", "documento", "12.ABC.345/01DE-35"}, {"conta", "cpf", "529.982.247-25"}, {"conta", "agencia", "0001-X"}, {"conta", "numero", "0000123-X"}, {"pedido", "valor", "250.50"}} {
		if got := normalized[tc.section][tc.key]; got != tc.want {
			t.Errorf("%s.%s: %s, want %s", tc.section, tc.key, got, tc.want)
		}
	}
	delete(data, "dependente")
	data["solicitante"]["email"] = ""
	data["solicitante"]["telefone"] = ""
	data["conta"]["banco"] = "001"
	if _, err = validate(data); err != nil {
		t.Fatalf("optional fields or bank code rejected: %s", err)
	}
}
func TestInvalidFieldsAreRejectedWithoutPersisting(t *testing.T) {
	database = filepath.Join(t.TempDir(), "test.sqlite")
	handler := authenticatedHandler(t)
	cases := []struct{ section, key, value string }{
		{"solicitante", "inscricao", "123A"}, {"dependente", "inscricao", "123456789012345678"}, {"solicitante", "nome", "   "}, {"solicitante", "nome", strings.Repeat("Á", 66)}, {"solicitante", "email", "nome@dominio"}, {"solicitante", "email", "nome..teste@example.com"}, {"solicitante", "email", "Nome <a@example.com>"}, {"solicitante", "telefone", "1199999999"}, {"solicitante", "telefone", "(01) 99999-9999"}, {"solicitante", "telefone", "(11) 1234-5678"}, {"prestador", "documento", "11111111111"}, {"conta", "cpf", "529.982.247-24"}, {"conta", "agencia", "12--3"}, {"conta", "numero", "123-ABC"}, {"conta", "titular", "1234"}, {"pedido", "local", "S\nP"}, {"pedido", "data", "0000-01-01"}, {"pedido", "data", "2026-02-29"}, {"pedido", "valor", "1e3"}, {"pedido", "valor", "0"}, {"pedido", "valor", "-10"}, {"pedido", "valor", "10.001"}, {"pedido", "valor", "1000000000.00"},
	}
	for _, tc := range cases {
		t.Run(tc.section+"."+tc.key+"="+tc.value, func(t *testing.T) {
			d := sample()
			d[tc.section][tc.key] = tc.value
			w := request(t, handler, "POST", "/api/pedidos", d, nil)
			if w.Code != 400 {
				t.Fatalf("invalid value accepted: %d %s", w.Code, w.Body)
			}
		})
	}
	out, err := store("list", nil)
	if err != nil {
		t.Fatal(err)
	}
	var lists map[string][]any
	json.Unmarshal(out, &lists)
	for _, rows := range lists {
		if len(rows) != 0 {
			t.Fatal("invalid data persisted")
		}
	}
}
func TestAlphabeticalSavedRecords(t *testing.T) {
	database = filepath.Join(t.TempDir(), "test.sqlite")
	for i, name := range []string{"Zélia", "bruno", "Álvaro", "amanda"} {
		d := sample()
		d["solicitante"]["nome"] = name
		d["dependente"]["nome"] = name
		d["solicitante"]["inscricao"] = strconv.Itoa(i + 1)
		d["dependente"]["cpf"] = d["solicitante"]["cpf"]
		d["dependente"]["inscricao"] = d["solicitante"]["inscricao"]
		d["prestador"]["nome"] = name
		d["conta"]["titular"] = name
		if _, err := store("save", d); err != nil {
			t.Fatal(err)
		}
	}
	out, err := store("list", nil)
	if err != nil {
		t.Fatal(err)
	}
	var lists map[string][]map[string]any
	json.Unmarshal(out, &lists)
	for _, table := range []string{"pessoas", "prestadores", "contas"} {
		var got []string
		for _, row := range lists[table] {
			key := "nome"
			if table == "contas" {
				key = "titular"
			}
			got = append(got, row[key].(string))
		}
		if strings.Join(got, ",") != "Álvaro,amanda,bruno,Zélia" {
			t.Errorf("%s not alphabetical: %v", table, got)
		}
	}
}

func TestRegistrationLengthAndLeadingZeros(t *testing.T) {
	for _, value := range []string{"00001234", "0123456789012345", "01234567890123456"} {
		d := sample()
		d["solicitante"]["inscricao"] = value
		d["dependente"]["inscricao"] = value
		got, err := validate(d)
		if err != nil {
			t.Fatalf("matrícula %s rejeitada: %v", value, err)
		}
		for _, section := range []string{"solicitante", "dependente"} {
			if got[section]["inscricao"] != value {
				t.Fatalf("matrícula alterada: %s", got[section]["inscricao"])
			}
		}
	}
	for _, section := range []string{"solicitante", "dependente"} {
		d := sample()
		d[section]["inscricao"] = "012345678901234567"
		if _, err := validate(d); err == nil {
			t.Fatalf("matrícula com 18 dígitos aceita para %s", section)
		}
	}
}
