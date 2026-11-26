package main

import (
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	digitsOnly   = regexp.MustCompile(`^[0-9]+$`)
	cpfFormat    = regexp.MustCompile(`^(?:[0-9]{11}|[0-9]{3}\.[0-9]{3}\.[0-9]{3}-[0-9]{2})$`)
	cnpjFormat   = regexp.MustCompile(`^(?:[A-Z0-9]{12}[0-9]{2}|[A-Z0-9]{2}\.[A-Z0-9]{3}\.[A-Z0-9]{3}/[A-Z0-9]{4}-[0-9]{2})$`)
	bankNumber   = regexp.MustCompile(`^[0-9]+(?:-?[0-9X])?$`)
	phoneFormat  = regexp.MustCompile(`^(?:[0-9]{10,11}|\([0-9]{2}\) ?[0-9]{4,5}-[0-9]{4})$`)
	phoneDigits  = regexp.MustCompile(`^[1-9][0-9](?:[2-5][0-9]{7}|9[0-9]{8})$`)
	emailDomain  = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?)+$`)
	decimalMoney = regexp.MustCompile(`^[0-9]{1,9}(?:\.[0-9]{1,2})?$`)
)

func unmaskDocument(s string) string {
	return strings.NewReplacer(".", "", "-", "", "/", "").Replace(strings.ToUpper(s))
}
func repeated(s string) bool { return s == strings.Repeat(s[:1], len(s)) }
func mod11(sum int) int {
	r := sum % 11
	if r < 2 {
		return 0
	}
	return 11 - r
}
func validCPF(s string) bool {
	if !cpfFormat.MatchString(s) {
		return false
	}
	n := unmaskDocument(s)
	if repeated(n) {
		return false
	}
	for size := 9; size <= 10; size++ {
		sum := 0
		for i := 0; i < size; i++ {
			sum += int(n[i]-'0') * (size + 1 - i)
		}
		if int(n[size]-'0') != mod11(sum) {
			return false
		}
	}
	return true
}

// CNPJ numérico e alfanumérico: pesos módulo 11 e valor ASCII menos 48.
// Receita Federal: https://www.gov.br/receitafederal/pt-br/centrais-de-conteudo/publicacoes/documentos-tecnicos/cnpj/manual-dv-cnpj.pdf
func validCNPJ(s string) bool {
	s = strings.ToUpper(s)
	if !cnpjFormat.MatchString(s) {
		return false
	}
	n := unmaskDocument(s)
	if repeated(n) {
		return false
	}
	for size := 12; size <= 13; size++ {
		sum, weight := 0, 2
		for i := size - 1; i >= 0; i-- {
			sum += (int(n[i]) - 48) * weight
			weight++
			if weight == 10 {
				weight = 2
			}
		}
		if int(n[size]-'0') != mod11(sum) {
			return false
		}
	}
	return true
}
func formatCPF(s string) string {
	n := unmaskDocument(s)
	return n[:3] + "." + n[3:6] + "." + n[6:9] + "-" + n[9:]
}
func formatCNPJ(s string) string {
	n := unmaskDocument(s)
	return n[:2] + "." + n[2:5] + "." + n[5:8] + "/" + n[8:12] + "-" + n[12:]
}
func normalizePhone(s string) (string, bool) {
	if !phoneFormat.MatchString(s) {
		return "", false
	}
	n := strings.NewReplacer("(", "", ")", "", " ", "", "-", "").Replace(s)
	if !phoneDigits.MatchString(n) {
		return "", false
	}
	cut := len(n) - 4
	return "(" + n[:2] + ") " + n[2:cut] + "-" + n[cut:], true
}
func validEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	parts := strings.Split(s, "@")
	return len(parts) == 2 && !strings.ContainsAny(parts[0], "\"()[]:;\\,<> ") && emailDomain.MatchString(parts[1])
}
func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

type fieldRule struct {
	section, key, label string
	max                 int
	required            bool
}

var fieldRules = []fieldRule{
	{"solicitante", "cpf", "CPF do solicitante", 14, true},
	{"dependente", "cpf", "CPF do dependente", 14, true},
	{"solicitante", "inscricao", "Inscrição do solicitante", 17, true}, {"solicitante", "nome", "Nome do solicitante", 65, true}, {"solicitante", "email", "E-mail", 65, false}, {"solicitante", "email_secundario", "E-mail secundário", 65, false}, {"solicitante", "telefone", "Telefone", 15, false},
	{"dependente", "inscricao", "Inscrição do dependente", 17, true}, {"dependente", "nome", "Nome do dependente", 65, true}, {"dependente", "email", "E-mail do dependente", 65, false}, {"dependente", "telefone", "Telefone do dependente", 15, false},
	{"prestador", "nome", "Nome / razão social do prestador", 65, true}, {"prestador", "documento", "CPF / CNPJ do prestador", 18, true}, {"prestador", "descricao", "Descrição do prestador", 120, false},
	{"conta", "tipo", "Tipo de conta", 8, true}, {"conta", "banco", "Banco", 35, true}, {"conta", "agencia", "Agência", 30, true}, {"conta", "numero", "Número da conta", 25, true}, {"conta", "titular", "Titular da conta", 70, true}, {"conta", "cpf", "CPF do titular", 14, true},
	{"pedido", "valor", "Valor pago", 12, true}, {"pedido", "local", "Local", 65, true}, {"pedido", "data", "Data", 10, true},
}

func validate(d map[string]map[string]string) (map[string]map[string]string, error) {
	r := map[string]map[string]string{}
	for _, rule := range fieldRules {
		if rule.section == "dependente" && len(d["dependente"]) == 0 {
			continue
		}
		if r[rule.section] == nil {
			r[rule.section] = map[string]string{}
		}
		v := strings.TrimSpace(d[rule.section][rule.key])
		r[rule.section][rule.key] = v
		if rule.required && v == "" {
			return nil, fmt.Errorf("%s: preencha este campo.", rule.label)
		}
		if utf8.RuneCountInString(v) > rule.max {
			return nil, fmt.Errorf("%s: use até %d caracteres.", rule.label, rule.max)
		}
		for _, ch := range v {
			if unicode.IsControl(ch) {
				return nil, fmt.Errorf("%s: remova caracteres de controle e quebras de linha.", rule.label)
			}
		}
	}
	for _, section := range []string{"solicitante", "dependente"} {
		p := r[section]
		if p == nil {
			continue
		}
		if p["cpf"] != "" {
			if !validCPF(p["cpf"]) {
				return nil, fmt.Errorf("CPF do %s inválido.", section)
			}
			p["cpf"] = formatCPF(p["cpf"])
		}
		if !digitsOnly.MatchString(p["inscricao"]) {
			return nil, fmt.Errorf("Inscrição do %s: use somente números, até 17 dígitos.", section)
		}
		if p["email"] != "" && !validEmail(p["email"]) {
			return nil, fmt.Errorf("E-mail do %s: informe um endereço válido, como nome@dominio.com.", section)
		}
		if p["email_secundario"] != "" && !validEmail(p["email_secundario"]) {
			return nil, fmt.Errorf("E-mail secundário do %s: informe um endereço válido, como nome@dominio.com.", section)
		}
		if p["telefone"] != "" {
			phone, ok := normalizePhone(p["telefone"])
			if !ok {
				return nil, fmt.Errorf("Telefone do %s: informe DDD e telefone fixo ou celular com 10 ou 11 dígitos.", section)
			}
			p["telefone"] = phone
		}
	}
	for _, key := range [][2]string{{"solicitante", "nome"}, {"dependente", "nome"}, {"prestador", "nome"}, {"conta", "titular"}, {"conta", "banco"}, {"pedido", "local"}} {
		if r[key[0]] == nil {
			continue
		}
		v := r[key[0]][key[1]]
		// Banco pode ser identificado apenas por seu código numérico.
		if key == [2]string{"conta", "banco"} && digitsOnly.MatchString(v) {
			continue
		}
		if utf8.RuneCountInString(v) < 2 || !hasLetter(v) {
			return nil, fmt.Errorf("%s: informe um texto válido com pelo menos 2 caracteres.", fieldLabel(key[0], key[1]))
		}
	}
	doc := r["prestador"]["documento"]
	if validCPF(doc) {
		r["prestador"]["documento"] = formatCPF(doc)
	} else if validCNPJ(doc) {
		r["prestador"]["documento"] = formatCNPJ(doc)
	} else {
		return nil, fmt.Errorf("CPF / CNPJ do prestador: número ou dígitos verificadores inválidos.")
	}
	if !validCPF(r["conta"]["cpf"]) {
		return nil, fmt.Errorf("CPF do titular: número ou dígitos verificadores inválidos.")
	}
	r["conta"]["cpf"] = formatCPF(r["conta"]["cpf"])
	for _, key := range []string{"agencia", "numero"} {
		v := strings.ToUpper(r["conta"][key])
		if !bankNumber.MatchString(v) {
			return nil, fmt.Errorf("%s: use números e, se houver, o dígito após um hífen. O dígito X também é aceito.", fieldLabel("conta", key))
		}
		r["conta"][key] = v
	}
	if t := r["conta"]["tipo"]; t != "corrente" && t != "poupanca" {
		return nil, fmt.Errorf("Tipo de conta inválido.")
	}
	date, err := time.Parse("2006-01-02", r["pedido"]["data"])
	if err != nil || date.Year() < 1 {
		return nil, fmt.Errorf("Data: informe uma data válida.")
	}
	value := r["pedido"]["valor"]
	cents, err := moneyCents(value)
	if err != nil {
		return nil, fmt.Errorf("Valor pago: informe um valor de até R$ 999.999.999,99 com no máximo duas casas decimais.")
	}
	if cents == 0 {
		return nil, fmt.Errorf("Valor pago: informe um valor maior que zero.")
	}
	r["pedido"]["valor"] = fmt.Sprintf("%d.%02d", cents/100, cents%100)
	return r, nil
}
func fieldLabel(section, key string) string {
	for _, r := range fieldRules {
		if r.section == section && r.key == key {
			return r.label
		}
	}
	return key
}

// moneyCents converte o decimal da API sem usar ponto flutuante.
func moneyCents(value string) (int64, error) {
	if !decimalMoney.MatchString(value) {
		return 0, fmt.Errorf("Valor monetário inválido.")
	}
	parts := strings.SplitN(value, ".", 2)
	whole, _ := strconv.ParseInt(parts[0], 10, 64)
	fraction := "00"
	if len(parts) == 2 {
		fraction = (parts[1] + "0")[:2]
	}
	cents, _ := strconv.ParseInt(fraction, 10, 64)
	return whole*100 + cents, nil
}
