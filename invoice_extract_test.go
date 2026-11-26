package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
)

func TestExtractInvoiceEndpoint(t *testing.T) {
	for _, pdf := range [][]byte{[]byte("invalid"), []byte("%PDF-1.4\ninvalid")} {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, _ := writer.CreateFormFile("nota", "nota.pdf")
		part.Write(pdf)
		writer.Close()
		req := httptest.NewRequest("POST", "/api/notas/extrair", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		extractInvoice(w, req)
		if w.Code != 400 && w.Code != 422 {
			t.Fatalf("accepted invalid PDF: %d", w.Code)
		}
	}
}

func TestExtractUploadedExample(t *testing.T) {
	const path = "32015062205271221000118000000000000126019660646816.pdf"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("local example not present")
	}
	out, err := exec.Command("pdftotext", "-bbox-layout", path, "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseInvoiceLayout(out)
	if err != nil {
		t.Fatal(err)
	}
	if got["prestador"]["nome"] == "" || got["tomador"]["nome"] == "" || !validCNPJ(got["prestador"]["documento"]) || !validCPF(got["tomador"]["documento"]) || got["pedido"]["valor"] != "450.00" {
		raw, _ := json.Marshal(got)
		t.Fatalf("example extraction incomplete: %s", raw)
	}
}
