package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvoiceParserLimitsAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := parseInvoiceLayoutContext(ctx, []byte("<html/>")); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled parser: %v", err)
	}
	if _, err := parseInvoiceLayout(bytes.Repeat([]byte(" "), maxInvoiceTextBytes+1)); !errors.Is(err, errInvoiceComplexity) {
		t.Fatalf("output limit: %v", err)
	}
	for _, tc := range []struct {
		name  string
		count int
		text  string
	}{{"lines", maxInvoiceLines + 1, "text"}, {"regions", maxInvoiceRegions + 1, "Prestador"}, {"fields", maxInvoiceFields + 1, "Nome"}} {
		t.Run(tc.name, func(t *testing.T) {
			lines := make([]invoiceLine, tc.count)
			for i := range lines {
				lines[i] = box(10, float64(i)*12, tc.text)
			}
			if _, err := parseInvoiceLayout(layoutFixture(lines)); !errors.Is(err, errInvoiceComplexity) {
				t.Fatalf("complexity limit: %v", err)
			}
		})
	}
	var words strings.Builder
	words.WriteString("<html><line>")
	for i := 0; i < maxInvoiceWords+1; i++ {
		fmt.Fprintf(&words, `<word xMin="%d" yMin="1" xMax="%d" yMax="9">a</word>`, i, i+1)
	}
	words.WriteString("</line></html>")
	if _, err := parseInvoiceLayout([]byte(words.String())); !errors.Is(err, errInvoiceComplexity) {
		t.Fatalf("word limit: %v", err)
	}
}

func TestInvoiceOutputStopsAtLimit(t *testing.T) {
	canceled := false
	out := &invoiceOutput{limit: 4, cancel: func() { canceled = true }}
	if _, err := out.Write([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	if _, err := out.Write([]byte("5")); !errors.Is(err, errInvoiceComplexity) || !canceled || out.Len() != 4 {
		t.Fatalf("unbounded output: %v", err)
	}
}

func TestInvoiceOutputLimitAlsoAppliesToStreamCopies(t *testing.T) {
	canceled := false
	out := &invoiceOutput{limit: 4, cancel: func() { canceled = true }}
	_, err := io.Copy(out, io.LimitReader(strings.NewReader("12345"), 5))
	if !errors.Is(err, errInvoiceComplexity) || !canceled || out.Len() > 4 {
		t.Fatalf("stream copy bypassed limit: %v bytes=%d", err, out.Len())
	}
}

func TestPDFProcessingCapacityAndRelease(t *testing.T) {
	for i := 0; i < cap(pdfWorkSlots); i++ {
		pdfWorkSlots <- struct{}{}
	}
	handler := limitPDFProcessing(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest("POST", "/", nil))
	for i := 0; i < cap(pdfWorkSlots); i++ {
		<-pdfWorkSlots
	}
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("capacity ignored: %d", w.Code)
	}
	for i := 0; i < cap(pdfWorkSlots)+1; i++ {
		w = httptest.NewRecorder()
		handler(w, httptest.NewRequest("POST", "/", nil))
		if w.Code != 204 {
			t.Fatal("processing slot not released")
		}
	}
}
