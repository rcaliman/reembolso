package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func extractInvoice(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxInvoiceSize+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		reply(w, 400, map[string]string{"error": "Envie um PDF de até 10 MB."})
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("nota")
	if err != nil {
		reply(w, 400, map[string]string{"error": "Selecione a nota fiscal em PDF."})
		return
	}
	defer file.Close()
	pdf, err := io.ReadAll(io.LimitReader(file, maxInvoiceSize+1))
	if err != nil || header.Size > maxInvoiceSize || len(pdf) > maxInvoiceSize || !strings.HasPrefix(string(pdf), "%PDF-") {
		reply(w, 400, map[string]string{"error": "Selecione um PDF válido de até 10 MB."})
		return
	}
	dir, err := os.MkdirTemp("", "reembolso-extrair-")
	if err != nil {
		reply(w, 500, map[string]string{"error": "Não foi possível analisar o PDF."})
		return
	}
	defer os.RemoveAll(dir)
	path := dir + "/nota.pdf"
	if err = os.WriteFile(path, pdf, 0600); err != nil {
		reply(w, 500, map[string]string{"error": "Não foi possível analisar o PDF."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	// Process only the first page with bounded output and a shared time budget.
	out, err := invoiceText(ctx, path)
	if err != nil {
		reply(w, 422, map[string]string{"error": "Não foi possível ler o PDF. Confira se está sem senha e se o Poppler está instalado. Você pode preencher manualmente."})
		return
	}
	data, err := parseInvoiceLayoutContext(ctx, out)
	if err != nil {
		reply(w, 422, map[string]string{"error": "Não foi possível interpretar os campos do PDF. Você pode preencher manualmente."})
		return
	}
	reply(w, 200, data)
}
