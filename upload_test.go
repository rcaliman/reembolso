package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func submitInvoice(t *testing.T, pdf []byte, data map[string]map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	raw, _ := json.Marshal(data)
	writer.WriteField("dados", string(raw))
	part, err := writer.CreateFormFile("nota", "nota fiscal.pdf")
	if err != nil {
		t.Fatal(err)
	}
	part.Write(pdf)
	writer.Close()
	req := httptest.NewRequest("POST", "/api/pedidos", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	authenticatedHandler(t).ServeHTTP(w, req)
	return w
}
func TestInvoicePersistsAndPrintPages(t *testing.T) {
	database = filepath.Join(t.TempDir(), "database.sqlite")
	pdf, err := os.ReadFile("testdata/nota.pdf")
	if err != nil {
		t.Fatal(err)
	}
	w := submitInvoice(t, pdf, sample())
	if w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	out, err := store("get", map[string]int{"id": 1})
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]map[string]string
	json.Unmarshal(out, &data)
	if data["nota"]["paginas"] != "2" {
		t.Fatalf("metadata: %s", out)
	}
	for page := 1; page <= 2; page++ {
		w = httptest.NewRecorder()
		authenticatedHandler(t).ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/api/pedidos/1/nota/paginas/%d", page), nil))
		if w.Code != 200 || !bytes.HasPrefix(w.Body.Bytes(), []byte("\x89PNG")) {
			t.Fatalf("page %d: %d", page, w.Code)
		}
	}
	w = httptest.NewRecorder()
	authenticatedHandler(t).ServeHTTP(w, httptest.NewRequest("GET", "/api/pedidos/1/nota", nil))
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), pdf) {
		t.Fatal("original PDF changed or missing")
	}
	w = httptest.NewRecorder()
	authenticatedHandler(t).ServeHTTP(w, httptest.NewRequest("GET", "/api/pedidos/1/nota/paginas/3", nil))
	if w.Code != 404 {
		t.Fatal("missing page must return 404")
	}
}
func TestInvoiceInvalidAndCleanup(t *testing.T) {
	database = filepath.Join(t.TempDir(), "database.sqlite")
	for _, pdf := range [][]byte{[]byte("not a PDF"), []byte("%PDF-1.4\ninvalid")} {
		w := submitInvoice(t, pdf, sample())
		if w.Code != 400 {
			t.Fatalf("invalid PDF accepted: %s", w.Body)
		}
	}
	pdf, err := os.ReadFile("testdata/nota.pdf")
	if err != nil {
		t.Fatal(err)
	}
	data := sample()
	data["pedido"]["valor"] = "0"
	w := submitInvoice(t, pdf, data)
	if w.Code != 400 {
		t.Fatalf("invalid request: %d", w.Code)
	}
	entries, _ := os.ReadDir(filepath.Join(filepath.Dir(database), "anexos"))
	if len(entries) != 0 {
		t.Fatal("orphan invoice files")
	}
	out, err := store("get", map[string]int{"id": 1})
	if err != nil || string(bytes.TrimSpace(out)) != "null" {
		t.Fatalf("invalid upload persisted: %s %v", out, err)
	}
}

func TestInvoiceCPFConflictCleanup(t *testing.T) {
	database = filepath.Join(t.TempDir(), "cpf-conflict.sqlite")
	savePeopleRequest(t, sample())
	pdf, err := os.ReadFile("testdata/nota.pdf")
	if err != nil {
		t.Fatal(err)
	}
	d := sample()
	d["solicitante"]["cpf"] = "012.345.678-90"
	w := submitInvoice(t, pdf, d)
	if w.Code != 409 {
		t.Fatalf("conflict status: %d %s", w.Code, w.Body)
	}
	entries, _ := os.ReadDir(filepath.Join(filepath.Dir(database), "anexos"))
	if len(entries) != 0 {
		t.Fatal("conflicting request left an orphan invoice")
	}
	out, err := store("get", map[string]int{"id": 2})
	if err != nil || string(bytes.TrimSpace(out)) != "null" {
		t.Fatalf("conflicting request persisted: %s", out)
	}
}

func TestMultipleAttachments(t *testing.T) {
	pdf, err := os.ReadFile("testdata/nota.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		count       int
		invalidLast bool
		status      int
	}{
		{"three", 3, false, 201}, {"too many", 4, false, 400}, {"invalid last", 3, true, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database = filepath.Join(t.TempDir(), "database.sqlite")
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			raw, _ := json.Marshal(sample())
			writer.WriteField("dados", string(raw))
			for i := 0; i < tc.count; i++ {
				part, e := writer.CreateFormFile("nota", fmt.Sprintf("documento-%d.pdf", i+1))
				if e != nil {
					t.Fatal(e)
				}
				if tc.invalidLast && i == tc.count-1 {
					part.Write([]byte("invalid"))
				} else {
					part.Write(pdf)
				}
			}
			writer.Close()
			req := httptest.NewRequest("POST", "/api/pedidos", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			h := authenticatedHandler(t)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if tc.status != 201 {
				entries, _ := os.ReadDir(filepath.Join(filepath.Dir(database), "anexos"))
				if len(entries) != 0 {
					t.Fatal("orphan attachments")
				}
				return
			}
			for i, key := range attachmentKeys {
				out, e := store("get", map[string]int{"id": 1})
				if e != nil {
					t.Fatal(e)
				}
				var data map[string]map[string]string
				json.Unmarshal(out, &data)
				if data[key]["nome"] != fmt.Sprintf("documento-%d.pdf", i+1) {
					t.Fatalf("missing metadata: %s", out)
				}
				for _, suffix := range []string{"", "/paginas/1"} {
					w = httptest.NewRecorder()
					h.ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/api/pedidos/1/documentos/%d%s", i+1, suffix), nil))
					if w.Code != 200 {
						t.Fatalf("document %d%s: %d", i+1, suffix, w.Code)
					}
				}
			}
			for _, index := range []string{"0", "4", "invalid"} {
				w = httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("GET", "/api/pedidos/1/documentos/"+index, nil))
				if w.Code != 404 {
					t.Fatalf("invalid index accepted: %s", index)
				}
			}
		})
	}
}

func TestClientCannotSupplyAttachmentMetadata(t *testing.T) {
	var body bytes.Buffer
	d := sample()
	for _, key := range attachmentKeys {
		d[key] = map[string]string{"arquivo": "nota-forged"}
	}
	json.NewEncoder(&body).Encode(d)
	data, cleanup, err := readSubmission(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/pedidos", &body))
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range attachmentKeys {
		if data[key] != nil {
			t.Fatalf("client metadata accepted: %s", key)
		}
	}
}

func TestSeparateInvoiceAndDocuments(t *testing.T) {
	pdf, err := os.ReadFile("testdata/nota.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		note   bool
		extras int
		status int
	}{
		{"optional", false, 0, 201}, {"two documents", true, 1, 201}, {"note and two reports", true, 2, 201}, {"three reports only", false, 3, 201}, {"combined limit", true, 3, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database = filepath.Join(t.TempDir(), "db.sqlite")
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			raw, _ := json.Marshal(sample())
			writer.WriteField("dados", string(raw))
			var names []string
			// Os laudos chegam antes da nota no multipart para verificar a ordem definida pelo servidor.
			for i := 0; i < tc.extras; i++ {
				name := fmt.Sprintf("laudo-%d.pdf", i+1)
				part, _ := writer.CreateFormFile("documentos", name)
				part.Write(pdf)
				names = append(names, name)
			}
			if tc.note {
				part, _ := writer.CreateFormFile("nota", "nota.pdf")
				part.Write(pdf)
				names = append([]string{"nota.pdf"}, names...)
			}
			writer.Close()
			req := httptest.NewRequest("POST", "/api/pedidos", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			w := httptest.NewRecorder()
			authenticatedHandler(t).ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if tc.status != 201 {
				entries, _ := os.ReadDir(filepath.Join(filepath.Dir(database), "anexos"))
				if len(entries) != 0 {
					t.Fatal("orphan attachments")
				}
				return
			}
			out, err := store("get", map[string]int{"id": 1})
			if err != nil {
				t.Fatal(err)
			}
			var data map[string]map[string]string
			json.Unmarshal(out, &data)
			for i, key := range attachmentKeys {
				if i < len(names) {
					if data[key]["nome"] != names[i] {
						t.Fatalf("wrong order: %s", out)
					}
				} else if data[key] != nil {
					t.Fatalf("unexpected attachment: %s", out)
				}
			}
		})
	}
}

func TestSubmissionDeadlineSharedAcrossDocuments(t *testing.T) {
	database = filepath.Join(t.TempDir(), "db.sqlite")
	bin := t.TempDir()
	// Processos simples simulam conversões lentas sem depender de PDFs grandes nem esperar 45 s.
	info := "#!/usr/bin/python3\nprint('Pages: 1')\n"
	convert := "#!/usr/bin/python3\nimport pathlib,sys,time\npathlib.Path(sys.argv[-1]+'-started').write_text('started')\ntime.sleep(1)\npathlib.Path(sys.argv[-1]+'-1.png').write_bytes(b'PNG')\n"
	for name, script := range map[string]string{"pdfinfo": info, "pdftoppm": convert} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	raw, _ := json.Marshal(sample())
	writer.WriteField("dados", string(raw))
	for i := 0; i < 3; i++ {
		part, _ := writer.CreateFormFile("documentos", fmt.Sprintf("laudo-%d.pdf", i))
		part.Write([]byte("%PDF-1.4\n"))
	}
	writer.Close()
	req := httptest.NewRequest("POST", "/api/pedidos", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	started := time.Now()
	_, cleanup, err := readSubmissionWithin(httptest.NewRecorder(), req, 2500*time.Millisecond)
	defer cleanup()
	if err == nil || !strings.Contains(err.Error(), "prazo") {
		t.Fatalf("expected shared timeout, got %v", err)
	}
	if time.Since(started) > 4*time.Second {
		t.Fatal("conversion did not stop at shared deadline")
	}
	entries, _ := os.ReadDir(filepath.Join(filepath.Dir(database), "anexos"))
	if len(entries) != 0 {
		t.Fatal("timeout left orphan attachments")
	}
	if _, err := os.Stat(database); !os.IsNotExist(err) {
		t.Fatal("timed out submission reached persistence")
	}
}
