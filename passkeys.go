package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

var passkeyOrigin string

// A origem é uma configuração do servidor, nunca derivada de cabeçalhos de proxy.
func parsePasskeyOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !(u.Scheme == "http" && u.Hostname() == "localhost")) || net.ParseIP(u.Hostname()) != nil {
		return nil, errors.New("use uma origem HTTPS com nome de domínio, como https://reembolso.exemplo.com:5002; localhost HTTP é permitido para desenvolvimento")
	}
	if port := u.Port(); port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return nil, errors.New("porta inválida na origem WebAuthn")
		}
	}
	u.Path = ""
	u.RawPath = ""
	u.Host = strings.ToLower(u.Host)
	if (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80") {
		u.Host = u.Hostname()
	}
	return u, nil
}
func passkeyServer(r *http.Request) (*webauthn.WebAuthn, *url.URL, error) {
	raw := passkeyOrigin
	if raw == "" {
		// O único padrão automático é localhost. Produção exige uma origem explícita.
		candidate, err := url.Parse("http://" + r.Host)
		if err != nil || candidate.Hostname() != "localhost" {
			return nil, nil, errors.New("O login por biometria precisa ser habilitado no endereço HTTPS da aplicação.")
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		raw = scheme + "://" + r.Host
	}
	origin, err := parsePasskeyOrigin(raw)
	if err != nil {
		return nil, nil, err
	}
	requested, err := parsePasskeyOrigin(origin.Scheme + "://" + r.Host)
	if err != nil || requested.String() != origin.String() || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin.String()) {
		return nil, nil, errors.New("Acesse o endereço configurado para o login por biometria.")
	}
	required := true
	server, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "Reembolso", RPID: origin.Hostname(), RPOrigins: []string{origin.String()},
		AttestationPreference:  protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, RequireResidentKey: &required, UserVerification: protocol.VerificationRequired},
	})
	return server, origin, err
}
func passkeyRequestServer(w http.ResponseWriter, r *http.Request) (*webauthn.WebAuthn, *url.URL, bool) {
	server, origin, err := passkeyServer(r)
	if err != nil {
		reply(w, 400, map[string]string{"error": err.Error()})
		return nil, nil, false
	}
	return server, origin, true
}

type passkeyUser struct {
	user
	Handle      []byte                `json:"handle"`
	Credentials []webauthn.Credential `json:"credentials"`
	Versions    map[string]int        `json:"versions"`
}

func (u *passkeyUser) WebAuthnID() []byte                         { return u.Handle }
func (u *passkeyUser) WebAuthnName() string                       { return u.Login }
func (u *passkeyUser) WebAuthnDisplayName() string                { return u.Login }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

type passkeyFailure struct {
	Error  string `json:"error"`
	Status int    `json:"status"`
}

func passkeyStore(w http.ResponseWriter, action string, data any, result any) bool {
	out, err := store(action, data)
	if err != nil {
		log.Printf("passkey %s: %v", action, err)
		reply(w, 500, map[string]string{"error": "Não foi possível concluir a operação."})
		return false
	}
	var failure passkeyFailure
	_ = json.Unmarshal(out, &failure)
	if failure.Error != "" {
		reply(w, failure.Status, map[string]string{"error": failure.Error})
		return false
	}
	if err = json.Unmarshal(out, result); err != nil {
		reply(w, 500, map[string]string{"error": "Resposta inválida da persistência."})
		return false
	}
	return true
}
func secretDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func challengeCookieName(kind string) string { return "reembolso_passkey_" + kind }
func challengeCookie(w http.ResponseWriter, r *http.Request, kind, token string, age int, origin *url.URL) {
	http.SetCookie(w, &http.Cookie{Name: challengeCookieName(kind), Value: token, Path: "/api/passkeys", HttpOnly: true, Secure: origin.Scheme == "https", SameSite: http.SameSiteStrictMode, MaxAge: age})
}
func challengeBinding(r *http.Request, kind string, origin *url.URL) map[string]any {
	data := map[string]any{"tipo": kind, "origem": origin.String()}
	if kind == "register" {
		data["id"] = currentUser(r).ID
		if cookie, err := r.Cookie("reembolso_session"); err == nil {
			data["sessao"] = secretDigest(cookie.Value)
		}
	}
	return data
}
func savePasskeyChallenge(w http.ResponseWriter, r *http.Request, kind string, origin *url.URL, payload any) bool {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		reply(w, 500, map[string]string{"error": "Não foi possível iniciar a confirmação."})
		return false
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	data := challengeBinding(r, kind, origin)
	data["token"] = secretDigest(token)
	data["payload"] = payload
	data["cliente"] = secretDigest(clientAddress(r))
	if old, err := r.Cookie(challengeCookieName(kind)); err == nil && len(old.Value) <= 128 {
		data["previous"] = secretDigest(old.Value)
	}
	var result map[string]any
	if !passkeyStore(w, "passkey_challenge_save", data, &result) {
		return false
	}
	challengeCookie(w, r, kind, token, 300, origin)
	return true
}

type passkeyChallenge struct {
	Session webauthn.SessionData `json:"session"`
	Name    string               `json:"nome"`
}

func takePasskeyChallenge(w http.ResponseWriter, r *http.Request, kind string, origin *url.URL) (*passkeyChallenge, bool) {
	cookie, err := r.Cookie(challengeCookieName(kind))
	if err != nil || len(cookie.Value) > 128 {
		reply(w, 400, map[string]string{"error": "Inicie novamente a confirmação no aparelho."})
		return nil, false
	}
	challengeCookie(w, r, kind, "", -1, origin)
	data := challengeBinding(r, kind, origin)
	data["token"] = secretDigest(cookie.Value)
	var result *passkeyChallenge
	if !passkeyStore(w, "passkey_challenge_take", data, &result) {
		return nil, false
	}
	if result == nil {
		reply(w, 400, map[string]string{"error": "A confirmação expirou ou já foi utilizada. Tente novamente."})
		return nil, false
	}
	return result, true
}
func passkeyName(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > 60 {
		return "", false
	}
	for _, ch := range value {
		if unicode.IsControl(ch) {
			return "", false
		}
	}
	return value, true
}
func passkeyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/passkeys/config", func(w http.ResponseWriter, r *http.Request) {
		_, _, err := passkeyServer(r)
		if err != nil {
			reply(w, 200, map[string]any{"enabled": false, "message": err.Error()})
			return
		}
		reply(w, 200, map[string]bool{"enabled": true})
	})
	mux.HandleFunc("GET /api/passkeys", func(w http.ResponseWriter, r *http.Request) {
		authStore(w, "passkey_list", map[string]int{"id": currentUser(r).ID})
	})
	mux.HandleFunc("DELETE /api/passkeys/{id}", func(w http.ResponseWriter, r *http.Request) {
		keyID, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || keyID < 1 {
			http.NotFound(w, r)
			return
		}
		var data struct {
			Password string `json:"senha"`
		}
		if !authJSON(w, r, &data) {
			return
		}
		if len(data.Password) > 512 {
			reply(w, 400, map[string]string{"error": "Senha inválida."})
			return
		}
		authStore(w, "passkey_delete", map[string]any{"id": currentUser(r).ID, "key_id": keyID, "senha": data.Password})
	})
	mux.HandleFunc("POST /api/passkeys/register/begin", func(w http.ResponseWriter, r *http.Request) {
		server, origin, ok := passkeyRequestServer(w, r)
		if !ok {
			return
		}
		var data struct {
			Password string `json:"senha"`
			Name     string `json:"nome"`
		}
		if !authJSON(w, r, &data) {
			return
		}
		name, ok := passkeyName(data.Name)
		if !ok || len(data.Password) > 512 {
			reply(w, 400, map[string]string{"error": "Informe a senha atual e um nome de até 60 caracteres para a passkey."})
			return
		}
		var u *passkeyUser
		if !passkeyStore(w, "passkey_user", map[string]any{"id": currentUser(r).ID, "rp_id": origin.Hostname(), "create": true, "senha": data.Password}, &u) {
			return
		}
		if u == nil || len(u.Handle) < 32 {
			reply(w, 500, map[string]string{"error": "Não foi possível preparar o cadastro."})
			return
		}
		exclusions := make([]protocol.CredentialDescriptor, 0, len(u.Credentials))
		for _, credential := range u.Credentials {
			exclusions = append(exclusions, credential.Descriptor())
		}
		options, session, err := server.BeginRegistration(u, webauthn.WithExclusions(exclusions))
		if err != nil {
			log.Printf("begin registration: %v", err)
			reply(w, 500, map[string]string{"error": "Não foi possível iniciar o cadastro da passkey."})
			return
		}
		session.Expires = time.Now().Add(5 * time.Minute)
		if !savePasskeyChallenge(w, r, "register", origin, passkeyChallenge{Session: *session, Name: name}) {
			return
		}
		reply(w, 200, options)
	})
	mux.HandleFunc("POST /api/passkeys/register/finish", func(w http.ResponseWriter, r *http.Request) {
		server, origin, ok := passkeyRequestServer(w, r)
		if !ok {
			return
		}
		challenge, ok := takePasskeyChallenge(w, r, "register", origin)
		if !ok {
			return
		}
		var u *passkeyUser
		if !passkeyStore(w, "passkey_user", map[string]any{"id": currentUser(r).ID, "rp_id": origin.Hostname()}, &u) {
			return
		}
		if u == nil {
			reply(w, 400, map[string]string{"error": "Inicie novamente o cadastro da passkey."})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
		credential, err := server.FinishRegistration(u, challenge.Session, r)
		if err != nil {
			reply(w, 400, map[string]string{"error": "Não foi possível confirmar o cadastro no aparelho. Tente novamente."})
			return
		}
		authStore(w, "passkey_save", map[string]any{"id": u.ID, "rp_id": origin.Hostname(), "cred_id": base64.RawURLEncoding.EncodeToString(credential.ID), "credential": credential, "nome": challenge.Name, "session": challengeBinding(r, "register", origin)["sessao"]})
	})
	mux.HandleFunc("POST /api/passkeys/login/begin", func(w http.ResponseWriter, r *http.Request) {
		server, origin, ok := passkeyRequestServer(w, r)
		if !ok {
			return
		}
		options, session, err := server.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			reply(w, 500, map[string]string{"error": "Não foi possível iniciar o login por passkey."})
			return
		}
		session.Expires = time.Now().Add(5 * time.Minute)
		if !savePasskeyChallenge(w, r, "login", origin, passkeyChallenge{Session: *session}) {
			return
		}
		reply(w, 200, options)
	})
	mux.HandleFunc("POST /api/passkeys/login/finish", func(w http.ResponseWriter, r *http.Request) {
		server, origin, ok := passkeyRequestServer(w, r)
		if !ok {
			return
		}
		challenge, ok := takePasskeyChallenge(w, r, "login", origin)
		if !ok {
			return
		}
		lookup := func(rawID, handle []byte) (webauthn.User, error) {
			out, err := store("passkey_lookup", map[string]any{"cred_id": base64.RawURLEncoding.EncodeToString(rawID), "handle": handle, "rp_id": origin.Hostname()})
			if err != nil {
				return nil, err
			}
			var u *passkeyUser
			if err = json.Unmarshal(out, &u); err != nil {
				return nil, err
			}
			if u == nil {
				return nil, errors.New("credencial não encontrada")
			}
			return u, nil
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
		owner, credential, err := server.FinishPasskeyLogin(lookup, challenge.Session, r)
		if err != nil || credential == nil || credential.Authenticator.CloneWarning {
			reply(w, 401, map[string]string{"error": "Não foi possível entrar com esta passkey. Tente novamente ou use sua senha."})
			return
		}
		u, ok := owner.(*passkeyUser)
		if !ok {
			reply(w, 500, map[string]string{"error": "Não foi possível confirmar o usuário."})
			return
		}
		id := base64.RawURLEncoding.EncodeToString(credential.ID)
		version, ok := u.Versions[id]
		if !ok {
			reply(w, 401, map[string]string{"error": "Inicie novamente o login por passkey."})
			return
		}
		var result struct {
			Token string `json:"token"`
			User  user   `json:"usuario"`
		}
		if !passkeyStore(w, "passkey_login", map[string]any{"id": u.ID, "rp_id": origin.Hostname(), "cred_id": id, "credential": credential, "version": version}, &result) {
			return
		}
		if result.Token == "" || result.User.ID == 0 {
			reply(w, 500, map[string]string{"error": "Não foi possível criar a sessão."})
			return
		}
		sessionCookie(w, r, result.Token, 43200)
		reply(w, 200, result.User)
	})
}

// É possível terminar TLS em um proxy, desde que a origem pública tenha sido fixada.
func configuredHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if passkeyOrigin == "" {
		return false
	}
	origin, err := parsePasskeyOrigin(passkeyOrigin)
	if err != nil || origin.Scheme != "https" {
		return false
	}
	requested, err := parsePasskeyOrigin(origin.Scheme + "://" + r.Host)
	return err == nil && requested.String() == origin.String()
}

var _ webauthn.User = (*passkeyUser)(nil)

func publicPasskeyPath(path string) bool {
	switch path {
	case "/passkeys.js", "/api/passkeys/config", "/api/passkeys/login/begin", "/api/passkeys/login/finish":
		return true
	}
	return false
}
