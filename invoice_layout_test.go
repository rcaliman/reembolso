package main

import (
	"fmt"
	"html"
	"strings"
	"testing"
)

func layoutFixture(lines []invoiceLine) []byte {
	var out strings.Builder
	out.WriteString(`<html><body><doc><page width="600" height="842"><flow><block>`)
	for _, line := range lines {
		fmt.Fprintf(&out, `<line><word xMin="%f" yMin="%f" xMax="%f" yMax="%f">%s</word></line>`, line.X, line.Y, line.Right, line.Bottom, html.EscapeString(line.Text))
	}
	out.WriteString(`</block></flow></page></doc></body></html>`)
	return []byte(out.String())
}
func box(x, y float64, text string) invoiceLine {
	return invoiceLine{X: x, Y: y, Right: x + 100, Bottom: y + 8, Text: text}
}
func checkLayout(t *testing.T, lines []invoiceLine) map[string]map[string]string {
	t.Helper()
	result, err := parseInvoiceLayout(layoutFixture(lines))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestInvoiceLayoutColumnsAndAliases(t *testing.T) {
	lines := []invoiceLine{
		box(10, 10, "Dados do Emitente"), box(310, 10, "Identificação do Tomador"),
		box(10, 30, "CPF/CNPJ"), box(310, 30, "CPF:"),
		box(10, 43, "11.222.333/0001-81"), box(310, 43, "52998224725"),
		box(10, 60, "Razão Social"), box(310, 60, "Nome"),
		box(10, 73, "Clínica Exemplo"), box(310, 73, "Maria da Silva"),
		box(10, 90, "Endereço"), box(310, 90, "Endereço"),
		box(10, 103, "Rua Um"), box(310, 103, "Rua Dois"),
		box(10, 130, "Discriminação dos serviços"),
		box(10, 160, "Valor dos serviços"), box(310, 160, "Valor líquido da nota"),
		box(10, 173, "R$ 1.000,00"), box(310, 173, "R$ 950,00"),
	}
	// The producer's stream order must not affect extraction.
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	got := checkLayout(t, lines)
	if got["prestador"]["nome"] != "Clínica Exemplo" || got["tomador"]["nome"] != "Maria da Silva" || got["prestador"]["documento"] != "11.222.333/0001-81" || got["tomador"]["documento"] != "529.982.247-25" || got["pedido"]["valor"] != "950.00" {
		t.Fatalf("unexpected extraction: %v", got)
	}
}

func TestInvoiceLayoutInlineAndWrappedName(t *testing.T) {
	got := checkLayout(t, []invoiceLine{
		box(10, 10, "Prestador de serviços"), box(10, 30, "CNPJ: 11222333000181"),
		box(10, 50, "Nome / Razão Social"), box(10, 63, "Clínica de Saúde"), box(10, 73, "Exemplo Ltda"),
		box(10, 90, "E-mail"), box(10, 103, "contato@example.com"),
		box(10, 130, "Tomador de serviços"), box(10, 150, "CPF: 529.982.247-25"), box(10, 170, "Nome: Maria da Silva"),
		box(10, 210, "Valor total da nota: R$ 450,00"),
	})
	if got["prestador"]["nome"] != "Clínica de Saúde Exemplo Ltda" || got["tomador"]["nome"] != "Maria da Silva" || got["pedido"]["valor"] != "450.00" {
		t.Fatalf("unexpected: %v", got)
	}
}

func TestInvoiceLayoutRejectsAmbiguousInvalidAndUnlabeled(t *testing.T) {
	got := checkLayout(t, []invoiceLine{
		box(10, 10, "Prestador"), box(10, 30, "CPF/CNPJ"), box(10, 43, "11.222.333/0001-80"),
		box(10, 70, "Tomador"), box(10, 90, "CPF: 529.982.247-25"), box(10, 110, "CPF: 01234567890"),
		box(10, 150, "Valor líquido: R$ 100,00"), box(10, 175, "Valor líquido: R$ 200,00"),
		box(10, 200, "Valor total: R$ 300,00"),
		box(10, 230, "R$ 999,00"),
	})
	if len(got["prestador"])+len(got["tomador"])+len(got["pedido"]) != 0 {
		t.Fatalf("unsafe suggestions: %v", got)
	}
	got = checkLayout(t, []invoiceLine{box(10, 10, "CPF: 529.982.247-25"), box(10, 30, "Nome: Alguém"), box(10, 50, "R$ 450,00")})
	if len(got["prestador"])+len(got["tomador"])+len(got["pedido"]) != 0 {
		t.Fatalf("guessed roles without headings: %v", got)
	}
}

func TestInvoiceLayoutMovesWithCoordinates(t *testing.T) {
	for _, scale := range []float64{0.8, 1, 1.5} {
		lines := []invoiceLine{box(10, 10, "Prestador dos serviços"), box(10, 30, "CNPJ"), box(10, 43, "11222333000181"), box(10, 70, "Tomador dos serviços"), box(10, 90, "CPF"), box(10, 103, "52998224725"), box(10, 140, "Valor dos serviços"), box(10, 153, "450,00")}
		for i := range lines {
			lines[i].X = lines[i].X*scale + 50
			lines[i].Right = lines[i].Right*scale + 50
			lines[i].Y = lines[i].Y*scale + 20
			lines[i].Bottom = lines[i].Bottom*scale + 20
		}
		got := checkLayout(t, lines)
		if got["prestador"]["documento"] != "11.222.333/0001-81" || got["tomador"]["documento"] != "529.982.247-25" || got["pedido"]["valor"] != "450.00" {
			t.Fatalf("scale %f: %v", scale, got)
		}
	}
}

func TestInvoiceLayoutDoesNotBorrowNeighboringAmount(t *testing.T) {
	got := checkLayout(t, []invoiceLine{box(10, 10, "Valor dos serviços"), box(210, 10, "ISSQN"), box(10, 23, "-"), box(210, 23, "R$ 25,00")})
	if got["pedido"]["valor"] != "" {
		t.Fatalf("tax was mistaken for total: %v", got)
	}
}

func TestInvoiceLayoutRejectsTwoDocumentsUnderOneLabel(t *testing.T) {
	got := checkLayout(t, []invoiceLine{box(10, 10, "Tomador"), box(10, 30, "CPF"), box(10, 43, "52998224725"), box(10, 53, "01234567890")})
	if got["tomador"]["documento"] != "" {
		t.Fatalf("chose one of two documents: %v", got)
	}
}

func TestInvoiceAmountsRejectFragmentsAndDoNotFallBackFromInvalidNet(t *testing.T) {
	for _, text := range []string{"-100,00", "R$ -100,00", "−100,00", "(100,00)", "1.23,45", "100,000", "100,00 lixo", "100,00 200,00", "0,00", "R$ 0,00"} {
		t.Run(text, func(t *testing.T) {
			got := checkLayout(t, []invoiceLine{box(10, 10, "Valor líquido: "+text), box(10, 40, "Valor dos serviços: R$ 450,00")})
			if got["pedido"]["valor"] != "" {
				t.Fatalf("unsafe value from %q: %v", text, got)
			}
		})
	}
	for _, text := range []string{"R$ 1.234,56", "1234,56", "001234,56"} {
		got := checkLayout(t, []invoiceLine{box(10, 10, "Valor líquido: "+text)})
		if got["pedido"]["valor"] != "1234.56" {
			t.Fatalf("valid value rejected: %v", got)
		}
	}
}
