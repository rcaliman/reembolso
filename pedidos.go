package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var pedidoYear = regexp.MustCompile(`^[0-9]{4}$`)

func pedidoRoutes(mux *http.ServeMux) {
	mux.HandleFunc("DELETE /api/pedidos/{id}", deletePedido)

	mux.HandleFunc("GET /api/pedidos", func(w http.ResponseWriter, r *http.Request) {
		year := r.URL.Query().Get("ano")
		if year != "" && !pedidoYear.MatchString(year) {
			reply(w, 400, map[string]string{"error": "Ano inválido."})
			return
		}
		authStore(w, "list_pedidos", map[string]string{"ano": year})
	})
	mux.HandleFunc("GET /api/pedidos/filtros", func(w http.ResponseWriter, r *http.Request) {
		authStore(w, "pedido_filters", nil)
	})
	mux.HandleFunc("PATCH /api/pedidos/{id}/reembolso", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || id < 1 {
			http.NotFound(w, r)
			return
		}
		var d struct {
			Observacoes *string `json:"observacoes"`
			Valor       *string `json:"valor_reembolsado"`
			Versao      *int64  `json:"versao"`
		}
		// Até 2000 caracteres, incluindo escapes JSON e texto Unicode.
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&d) != nil {
			reply(w, 400, map[string]string{"error": "Dados inválidos."})
			return
		}
		if d.Valor == nil {
			reply(w, 400, map[string]string{"error": "Informe valor_reembolsado; use uma string vazia para limpar."})
			return
		}
		if d.Versao == nil || *d.Versao < 0 {
			reply(w, 400, map[string]string{"error": "Informe a versão do reembolso exibido na listagem."})
			return
		}
		value := strings.TrimSpace(*d.Valor)
		var cents any
		if value != "" {
			parsed, err := moneyCents(value)
			if err != nil {
				reply(w, 400, map[string]string{"error": "Informe um valor de 0 a 999999999.99, com até duas casas decimais."})
				return
			}
			cents = parsed
		}
		payload := map[string]any{"id": id, "centavos": cents, "versao": *d.Versao}
		if d.Observacoes != nil {
			if utf8.RuneCountInString(*d.Observacoes) > 2000 {
				reply(w, 400, map[string]string{"error": "As observações devem ter até 2000 caracteres."})
				return
			}
			payload["observacoes"] = *d.Observacoes
		}
		authStore(w, "reembolso", payload)
	})
}

func deletePedido(w http.ResponseWriter, r *http.Request) {
	if currentUser(r).Perfil != "admin" {
		reply(w, http.StatusForbidden, map[string]string{"error": "Apenas administradores podem excluir pedidos."})
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	var data struct {
		Versao *int64 `json:"versao"`
	}
	if !authJSON(w, r, &data) {
		return
	}
	if data.Versao == nil || *data.Versao < 0 {
		reply(w, 400, map[string]string{"error": "Informe a versão do pedido exibido na listagem."})
		return
	}
	out, err := store("delete_pedido", map[string]any{"id": id, "versao": *data.Versao})
	if err != nil {
		log.Printf("excluir pedido %d: %v", id, err)
		reply(w, 500, map[string]string{"error": "Não foi possível excluir o pedido."})
		return
	}
	var result struct {
		Error   string   `json:"error"`
		Status  int      `json:"status"`
		Folders []string `json:"pastas"`
	}
	if err = json.Unmarshal(out, &result); err != nil {
		reply(w, 500, map[string]string{"error": "Resposta inválida da persistência."})
		return
	}
	if result.Error != "" {
		reply(w, result.Status, map[string]string{"error": result.Error})
		return
	}
	response := map[string]any{"ok": true}
	for _, folder := range result.Folders {
		if folder == "" {
			continue
		}
		if dir, valid := invoiceDirectory(folder); valid {
			if err = os.RemoveAll(dir); err != nil {
				log.Printf("pedido %d excluído; falha ao remover anexo: %v", id, err)
				response["warning"] = "Não foi possível remover os arquivos dos documentos. Verifique as permissões da pasta de anexos no servidor."
			}
		} else {
			log.Printf("pedido %d excluído; caminho inválido de anexo ignorado", id)
			response["warning"] = "Um documento tinha um caminho inválido e seus arquivos não foram removidos. Verifique os anexos no servidor."
		}
	}
	reply(w, 200, response)
}
