package main

import (
	"bytes"
	"context"
	"embed"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// O catálogo público acompanha o executável e não é exposto como arquivo HTTP.
//
//go:embed rede.py data/banescaixa.sqlite3
var redeAssets embed.FS

func consultarRede(ctx context.Context, cidade, especialidade string) ([]byte, error) {
	script, err := redeAssets.ReadFile("rede.py")
	if err != nil {
		return nil, err
	}
	catalogo, err := redeAssets.ReadFile("data/banescaixa.sqlite3")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "-c", string(script), cidade, especialidade)
	cmd.Stdin = bytes.NewReader(catalogo)
	return cmd.Output()
}

func redeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/rede-credenciada", func(w http.ResponseWriter, r *http.Request) {
		cidade := strings.TrimSpace(r.URL.Query().Get("cidade"))
		especialidade := strings.TrimSpace(r.URL.Query().Get("especialidade"))
		if len(cidade) > 200 || len(especialidade) > 300 {
			reply(w, http.StatusBadRequest, map[string]string{"error": "Filtro inválido."})
			return
		}
		out, err := consultarRede(r.Context(), cidade, especialidade)
		if err != nil {
			log.Printf("rede credenciada: %v", err)
			reply(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível consultar a rede credenciada. Tente novamente."})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write(out)
	})
}
