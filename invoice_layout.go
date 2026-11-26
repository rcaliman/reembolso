package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type invoiceWord struct {
	X      float64 `xml:"xMin,attr"`
	Y      float64 `xml:"yMin,attr"`
	Right  float64 `xml:"xMax,attr"`
	Bottom float64 `xml:"yMax,attr"`
	Text   string  `xml:",chardata"`
}
type invoiceLine struct {
	X, Y, Right, Bottom float64
	Text                string
}
type invoiceRegion struct {
	X, Y, Right, Bottom float64
	Role                string
}

func invoiceKey(s string) string {
	s = strings.ToUpper(s)
	s = strings.NewReplacer("Á", "A", "À", "A", "Ã", "A", "Â", "A", "É", "E", "Ê", "E", "Í", "I", "Ó", "O", "Ô", "O", "Õ", "O", "Ú", "U", "Ü", "U", "Ç", "C").Replace(s)
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

func invoiceHeading(text string) string {
	key := invoiceKey(text)
	for _, prefix := range []string{"DADOS DO PRESTADOR", "IDENTIFICACAO DO PRESTADOR", "PRESTADOR DE SERVICOS", "PRESTADOR DOS SERVICOS", "PRESTADOR FORNECEDOR", "DADOS DO EMITENTE", "IDENTIFICACAO DO EMITENTE"} {
		if strings.HasPrefix(key, prefix) {
			return "prestador"
		}
	}
	for _, prefix := range []string{"DADOS DO TOMADOR", "IDENTIFICACAO DO TOMADOR", "TOMADOR ADQUIRENTE", "TOMADOR DE SERVICOS", "TOMADOR DOS SERVICOS"} {
		if strings.HasPrefix(key, prefix) {
			return "tomador"
		}
	}
	if key == "PRESTADOR" || key == "EMITENTE" {
		return "prestador"
	}
	if key == "TOMADOR" {
		return "tomador"
	}
	for _, prefix := range []string{"DESTINATARIO DA OPERACAO", "INTERMEDIARIO", "DADOS DO INTERMEDIARIO", "SERVICO PRESTADO", "DISCRIMINACAO", "DESCRICAO DO SERVICO", "TRIBUTACAO", "INFORMACOES COMPLEMENTARES"} {
		if strings.HasPrefix(key, prefix) {
			return "stop"
		}
	}
	return ""
}

// Parse Poppler's word coordinates rather than depending on text line order or
// fixed columns. Only first-page, explicitly labeled fields are considered.
func parseInvoiceLayout(data []byte) (map[string]map[string]string, error) {
	return parseInvoiceLayoutContext(context.Background(), data)
}
func parseInvoiceLayoutContext(ctx context.Context, data []byte) (map[string]map[string]string, error) {
	if len(data) > maxInvoiceTextBytes {
		return nil, errInvoiceComplexity
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := map[string]map[string]string{"prestador": {}, "tomador": {}, "pedido": {}}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	lines := []invoiceLine{}
	wordCount := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(lines) > maxInvoiceLines {
			return nil, errInvoiceComplexity
		}
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "line" {
			continue
		}
		var raw struct {
			Words []invoiceWord `xml:"word"`
		}
		if err := decoder.DecodeElement(&raw, &start); err != nil {
			return nil, err
		}
		wordCount += len(raw.Words)
		if wordCount > maxInvoiceWords {
			return nil, errInvoiceComplexity
		}
		if len(raw.Words) == 0 {
			continue
		}
		sort.Slice(raw.Words, func(i, j int) bool { return raw.Words[i].X < raw.Words[j].X })
		// Some producers put several columns in one line. Split at large geometric
		// gaps; retain ordinary spacing within a name or a label.
		var line invoiceLine
		for i, w := range raw.Words {
			if i == 0 || w.X-line.Right > math.Max(12, (w.Bottom-w.Y)*2) {
				if i > 0 {
					lines = append(lines, line)
				}
				line = invoiceLine{X: w.X, Y: w.Y, Right: w.Right, Bottom: w.Bottom, Text: w.Text}
			} else {
				line.Text += " " + w.Text
				line.Right = w.Right
				line.Bottom = math.Max(line.Bottom, w.Bottom)
			}
		}
		lines = append(lines, line)
	}
	if len(lines) > maxInvoiceLines {
		return nil, errInvoiceComplexity
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].Y != lines[j].Y {
			return lines[i].Y < lines[j].Y
		}
		return lines[i].X < lines[j].X
	})
	regions := []invoiceRegion{}
	for _, line := range lines {
		if role := invoiceHeading(line.Text); role != "" {
			regions = append(regions, invoiceRegion{line.X, line.Y, math.Inf(1), math.Inf(1), role})
		}
	}
	if len(regions) > maxInvoiceRegions {
		return nil, errInvoiceComplexity
	}
	for i := range regions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r := &regions[i]
		for j, other := range regions {
			if i == j {
				continue
			}
			if math.Abs(other.Y-r.Y) < 5 && other.X > r.X+10 {
				r.Right = math.Min(r.Right, other.X-2)
			}
		}
		for j, other := range regions {
			if i != j && other.Y > r.Y+5 && other.X < r.Right && other.X >= r.X-5 {
				r.Bottom = math.Min(r.Bottom, other.Y)
			}
		}
	}
	candidates := map[string]map[string][]string{"prestador": {}, "tomador": {}, "pedido": {}}
	amounts := map[int][]string{}
	amountSeen := map[int]bool{}
	amountInvalid := map[int]bool{}
	fieldCount := 0
	for _, label := range lines {
		kind, priority := invoiceField(label.Text)
		if kind == "" || kind == "boundary" {
			continue
		}
		fieldCount++
		if fieldCount > maxInvoiceFields {
			return nil, errInvoiceComplexity
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		role := ""
		var region invoiceRegion
		if kind == "valor" {
			role = "pedido"
			region = invoiceRegion{X: 0, Y: 0, Right: math.Inf(1), Bottom: math.Inf(1)}
		} else {
			for _, r := range regions {
				if r.Role != "stop" && label.X >= r.X-3 && label.X < r.Right && label.Y >= r.Y-2 && label.Y < r.Bottom-2 {
					if role != "" {
						role = ""
						break
					}
					role = r.Role
					region = r
				}
			}
		}
		if role == "" {
			continue
		}
		right := region.Right
		// Neighboring columns bound the search even if their headings are unknown.
		for _, other := range lines {
			if math.Abs(other.Y-label.Y) < 3 && other.X > label.X+3 {
				right = math.Min(right, other.X-2)
			}
		}
		values := []invoiceLine{}
		// Inline values require a colon, so a number in a heading cannot be a value.
		if _, after, ok := strings.Cut(label.Text, ":"); ok && strings.TrimSpace(after) != "" {
			inline := label
			inline.Text = strings.TrimSpace(after)
			values = append(values, inline)
		} else {
			for _, value := range lines {
				if value.X >= label.X-3 && value.X < right && value.Right <= right+3 && value.Y > label.Y+3 && value.Y < math.Min(region.Bottom, label.Bottom+40) {
					values = append(values, value)
				}
			}
		}
		sort.Slice(values, func(i, j int) bool { return values[i].Y < values[j].Y })
		collected := []string{}
		for index, value := range values {
			if index > 0 && value.Y-values[index-1].Bottom > 5 {
				break
			}
			if k, _ := invoiceField(value.Text); k != "" {
				break
			}
			if invoiceHeading(value.Text) != "" {
				break
			}
			text := strings.TrimSpace(value.Text)
			if text == "-" || text == "" {
				break
			}
			switch kind {
			case "documento":
				for _, doc := range invoiceDocuments.FindAllString(text, -1) {
					if validCPF(doc) {
						collected = append(collected, formatCPF(doc))
					} else if validCNPJ(doc) {
						collected = append(collected, formatCNPJ(doc))
					}
				}
			case "valor":
				amountSeen[priority] = true
				amount, ok := parseInvoiceAmount(text)
				if !ok {
					amountInvalid[priority] = true
				} else {
					collected = append(collected, amount)
				}
			case "nome":
				if !hasLetter(text) || len([]rune(text)) > 65 {
					break
				}
				collected = append(collected, text)
			}
			// Amounts use the nearest row; adjacent name/document rows are
			// checked together so conflicting documents are not silently ignored.
			if kind == "valor" {
				break
			}
			if len(collected) >= 3 {
				break
			}
		}
		if kind == "nome" && len(collected) > 0 {
			name := strings.Join(collected, " ")
			if len([]rune(name)) <= 65 {
				candidates[role][kind] = append(candidates[role][kind], name)
			}
		} else if kind == "valor" {
			amounts[priority] = append(amounts[priority], collected...)
		} else {
			candidates[role][kind] = append(candidates[role][kind], collected...)
		}
	}
	for role, fields := range candidates {
		for field, values := range fields {
			if value := uniqueInvoiceValue(values); value != "" {
				result[role][field] = value
			}
		}
	}
	for _, priority := range []int{1, 2, 3} {
		if amountSeen[priority] {
			if amountInvalid[priority] {
				break
			}
			if value := uniqueInvoiceValue(amounts[priority]); value != "" && value != "0.00" {
				result["pedido"]["valor"] = value
			}
			break
		}
	}
	return result, nil
}

var invoiceDocuments = regexp.MustCompile(`\b(?:[A-Z0-9]{2}\.[A-Z0-9]{3}\.[A-Z0-9]{3}/[A-Z0-9]{4}-\d{2}|\d{3}\.\d{3}\.\d{3}-\d{2}|[A-Z0-9]{12}\d{2}|\d{11})\b`)
var invoiceAmount = regexp.MustCompile(`^(?:R\$\s*)?(?:[0-9]{1,3}(?:\.[0-9]{3})+|[0-9]+),[0-9]{2}$`)

func parseInvoiceAmount(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if !invoiceAmount.MatchString(text) {
		return "", false
	}
	value := strings.TrimSpace(strings.TrimPrefix(text, "R$"))
	value = strings.ReplaceAll(strings.ReplaceAll(value, ".", ""), ",", ".")
	cents, err := moneyCents(value)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100), true
}

func uniqueInvoiceValue(values []string) string {
	unique := map[string]bool{}
	for _, value := range values {
		unique[value] = true
	}
	if len(unique) != 1 {
		return ""
	}
	for value := range unique {
		return value
	}
	return ""
}

func invoiceField(text string) (string, int) {
	// Strip inline content before classifying the label.
	label, _, _ := strings.Cut(text, ":")
	key := invoiceKey(label)
	switch key {
	case "ENDERECO", "E MAIL", "TELEFONE", "MUNICIPIO SIGLA UF", "MUNICIPIO UF", "INSCRICAO MUNICIPAL", "INSCRICAO ESTADUAL", "CODIGO IBGE CEP", "SIMPLES NACIONAL NA DATA DE COMPETENCIA":
		return "boundary", 0
	case "CPF", "CNPJ", "CPF CNPJ", "CNPJ CPF", "CNPJ CPF NIF", "CPF CNPJ NIF":
		return "documento", 0
	case "NOME", "RAZAO SOCIAL", "NOME RAZAO SOCIAL", "NOME NOME EMPRESARIAL", "NOME EMPRESARIAL":
		return "nome", 0
	case "VALOR LIQUIDO DA NFS E", "VALOR LIQUIDO DA NOTA", "VALOR LIQUIDO":
		return "valor", 1
	case "VALOR DA OPERACAO SERVICO", "VALOR DOS SERVICOS", "VALOR TOTAL DOS SERVICOS", "VALOR DO SERVICO":
		return "valor", 2
	case "VALOR TOTAL DA NFS E", "VALOR TOTAL DA NOTA", "VALOR TOTAL":
		return "valor", 3
	}
	return "", 0
}
