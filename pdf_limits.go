package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os/exec"
)

const maxInvoiceTextBytes = 4 << 20
const maxInvoiceWords = 12000
const maxInvoiceLines = 2000
const maxInvoiceRegions = 64
const maxInvoiceFields = 256

var errInvoiceComplexity = errors.New("PDF muito complexo para preenchimento automático")
var pdfWorkSlots = make(chan struct{}, 2)

func limitPDFProcessing(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case pdfWorkSlots <- struct{}{}:
			defer func() { <-pdfWorkSlots }()
			next(w, r)
		default:
			w.Header().Set("Retry-After", "5")
			reply(w, http.StatusTooManyRequests, map[string]string{"error": "O servidor está processando outras notas. Tente novamente em alguns segundos."})
		}
	}
}

type invoiceOutput struct {
	buffer bytes.Buffer
	limit  int
	cancel context.CancelFunc
}

func (out *invoiceOutput) Len() int      { return out.buffer.Len() }
func (out *invoiceOutput) Bytes() []byte { return out.buffer.Bytes() }

func (out *invoiceOutput) Write(p []byte) (int, error) {
	if len(p) > out.limit-out.Len() {
		out.cancel()
		return 0, errInvoiceComplexity
	}
	return out.buffer.Write(p)
}
func invoiceText(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := &invoiceOutput{limit: maxInvoiceTextBytes, cancel: cancel}
	cmd := exec.CommandContext(ctx, "pdftotext", "-f", "1", "-l", "1", "-bbox-layout", path, "-")
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
