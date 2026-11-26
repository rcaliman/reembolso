package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
)

var storeInitMu sync.Mutex
var initializedDatabases = map[string]bool{}

// Cada banco é migrado uma vez por processo; consultas não adquirem bloqueio de escrita.
func initializeStore() error {
	storeInitMu.Lock()
	defer storeInitMu.Unlock()
	if initializedDatabases[database] {
		return nil
	}
	out, err := runStore("init", map[string]string{"admin_password": os.Getenv("REEMBOLSO_ADMIN_PASSWORD")})
	if err != nil {
		return err
	}
	var initial struct {
		Password string `json:"senha_inicial"`
	}
	if err = json.Unmarshal(out, &initial); err != nil {
		return err
	}
	if initial.Password != "" {
		log.Printf("Administrador criado. Login: admin | Senha inicial: %s | Altere-a em Minha conta.", initial.Password)
	}
	initializedDatabases[database] = true
	return nil
}
func store(action string, data any) ([]byte, error) {
	if err := initializeStore(); err != nil {
		return nil, err
	}
	return runStore(action, data)
}
func runStore(action string, data any) ([]byte, error) {
	script, _ := assets.ReadFile("store.py")
	input, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("python3", "-c", string(script), database, action)
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("sqlite: %w: %s", err, stderr.String())
	}
	return out, nil
}
