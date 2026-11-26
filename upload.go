package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxInvoiceSize = 10 << 20
const maxSubmissionDuration = 45 * time.Second

var attachmentKeys = []string{"nota", "nota2", "nota3"}

func clearAttachments(data map[string]map[string]string) {
	for _, key := range attachmentKeys {
		delete(data, key)
	}
}

// A nota é persistida junto do banco; as imagens permitem imprimir suas páginas
// no mesmo trabalho de impressão do formulário, sem depender do visualizador PDF.
func readSubmission(w http.ResponseWriter, r *http.Request) (map[string]map[string]string, func(), error) {
	return readSubmissionWithin(w, r, maxSubmissionDuration)
}

// O prazo inclui a leitura do envio e é compartilhado por todos os documentos.
// Assim, sobra tempo para persistir e responder antes do limite HTTP de 90 s.
func readSubmissionWithin(w http.ResponseWriter, r *http.Request, timeout time.Duration) (map[string]map[string]string, func(), error) {
	var data map[string]map[string]string
	noop := func() {}
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "multipart/form-data" {
		err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&data)
		clearAttachments(data)
		return data, noop, err
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	r = r.WithContext(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, 3*maxInvoiceSize+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return nil, noop, fmt.Errorf("Envie até 3 PDFs de até 10 MB cada.")
	}
	defer r.MultipartForm.RemoveAll()
	raw := r.FormValue("dados")
	if len(raw) > 16384 {
		return nil, noop, fmt.Errorf("Dados do pedido muito grandes.")
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil || data == nil {
		return nil, noop, fmt.Errorf("Dados do pedido inválidos.")
	}
	// Não aceite caminhos ou metadados de anexos enviados pelo cliente.
	clearAttachments(data)
	headers := append([]*multipart.FileHeader{}, r.MultipartForm.File["nota"]...)
	headers = append(headers, r.MultipartForm.File["documentos"]...)
	count := 0
	for _, files := range r.MultipartForm.File {
		count += len(files)
	}
	if count > 3 || count != len(headers) {
		return nil, noop, fmt.Errorf("Envie até 3 documentos nos campos de anexos.")
	}
	var cleanups []func()
	cleanup := func() {
		for _, remove := range cleanups {
			remove()
		}
	}
	for i, header := range headers {
		if ctx.Err() != nil {
			cleanup()
			return nil, noop, fmt.Errorf("O prazo para enviar e preparar os documentos foi excedido. Tente novamente com PDFs menores.")
		}
		note, remove, err := prepareAttachment(r, header)
		if err != nil {
			cleanup()
			if ctx.Err() != nil {
				return nil, noop, fmt.Errorf("O prazo para enviar e preparar os documentos foi excedido. Tente novamente com PDFs menores.")
			}
			return nil, noop, err
		}
		cleanups = append(cleanups, remove)
		data[attachmentKeys[i]] = note
	}
	if ctx.Err() != nil {
		cleanup()
		return nil, noop, fmt.Errorf("O prazo para enviar e preparar os documentos foi excedido. Tente novamente com PDFs menores.")
	}
	return data, cleanup, nil
}

func prepareAttachment(r *http.Request, header *multipart.FileHeader) (map[string]string, func(), error) {
	noop := func() {}
	if header.Size > maxInvoiceSize {
		return nil, noop, fmt.Errorf("Cada documento deve ter até 10 MB.")
	}
	file, err := header.Open()
	if err != nil {
		return nil, noop, fmt.Errorf("Não foi possível ler o documento.")
	}
	defer file.Close()
	root := filepath.Join(filepath.Dir(database), "anexos")
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, noop, fmt.Errorf("Não foi possível criar a pasta de anexos.")
	}
	dir, err := os.MkdirTemp(root, "nota-")
	if err != nil {
		return nil, noop, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	pdf, err := io.ReadAll(io.LimitReader(file, maxInvoiceSize+1))
	if err != nil || len(pdf) > maxInvoiceSize || !strings.HasPrefix(string(pdf), "%PDF-") {
		cleanup()
		return nil, noop, fmt.Errorf("Selecione um arquivo PDF válido de até 10 MB.")
	}
	path := filepath.Join(dir, "original.pdf")
	if err = os.WriteFile(path, pdf, 0600); err != nil {
		cleanup()
		return nil, noop, err
	}
	ctx := r.Context()
	info, err := exec.CommandContext(ctx, "pdfinfo", path).Output()
	if err != nil {
		cleanup()
		return nil, noop, fmt.Errorf("Não foi possível abrir o PDF. Use um PDF válido, sem senha; verifique se o Poppler está instalado.")
	}
	pages := 0
	for _, line := range strings.Split(string(info), "\n") {
		if strings.HasPrefix(line, "Pages:") {
			pages, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Pages:")))
		}
	}
	if pages < 1 || pages > 20 {
		cleanup()
		return nil, noop, fmt.Errorf("Cada documento deve conter entre 1 e 20 páginas.")
	}
	err = exec.CommandContext(ctx, "pdftoppm", "-scale-to", "2000", "-png", path, filepath.Join(dir, "pagina")).Run()
	if err != nil {
		cleanup()
		return nil, noop, fmt.Errorf("Não foi possível preparar o documento para impressão. Verifique o PDF e a instalação do Poppler.")
	}
	// Normalize nomes: o Poppler usa zeros à esquerda conforme o total de páginas.
	for i := 1; i <= pages; i++ {
		source := filepath.Join(dir, fmt.Sprintf("pagina-%0*d.png", len(strconv.Itoa(pages)), i))
		if err = os.Rename(source, filepath.Join(dir, fmt.Sprintf("page-%d.png", i))); err != nil {
			cleanup()
			return nil, noop, fmt.Errorf("Não foi possível preparar as páginas do documento.")
		}
	}
	return map[string]string{"arquivo": filepath.Base(dir), "paginas": strconv.Itoa(pages), "nome": filepath.Base(header.Filename)}, cleanup, nil
}

func invoiceDirectory(folder string) (string, bool) {
	if !strings.HasPrefix(folder, "nota-") || filepath.Base(folder) != folder || strings.ContainsAny(folder, `/\`) {
		return "", false
	}
	return filepath.Join(filepath.Dir(database), "anexos", folder), true
}

func invoiceFile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	out, err := store("get", map[string]int{"id": id})
	if err != nil {
		http.Error(w, "Erro ao consultar nota fiscal.", 500)
		return
	}
	key := "nota"
	if index := r.PathValue("document"); index != "" {
		n, err := strconv.Atoi(index)
		if err != nil || n < 1 || n > 3 {
			http.NotFound(w, r)
			return
		}
		key = attachmentKeys[n-1]
	}
	var data map[string]map[string]string
	if json.Unmarshal(out, &data) != nil || data[key] == nil {
		http.NotFound(w, r)
		return
	}
	note := data[key]
	folder := note["arquivo"]
	dir, valid := invoiceDirectory(folder)
	if !valid {
		http.NotFound(w, r)
		return
	}
	filename := "original.pdf"
	if p := r.PathValue("page"); p != "" {
		page, e := strconv.Atoi(p)
		total, _ := strconv.Atoi(note["paginas"])
		if e != nil || page < 1 || page > total {
			http.NotFound(w, r)
			return
		}
		filename = fmt.Sprintf("page-%d.png", page)
	} else {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": note["nome"]}))
	}
	http.ServeFile(w, r, filepath.Join(dir, filename))
}
