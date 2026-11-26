package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func passkeyHTTP(t *testing.T, h http.Handler, path string, data any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "http://localhost"+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
func findCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %s missing: %v", name, w.Header())
	return nil
}
func credentialOptions(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("options: %d %s", w.Code, w.Body)
	}
	var data struct {
		PublicKey map[string]any `json:"publicKey"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	return data.PublicKey
}
func base64url(data []byte) string { return base64.RawURLEncoding.EncodeToString(data) }
func fixtureClient(t *testing.T, kind, challenge, origin string) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{"type": kind, "challenge": challenge, "origin": origin, "crossOrigin": false})
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func fixtureAuthData(rp string, flags byte, count uint32) []byte {
	hash := sha256.Sum256([]byte(rp))
	data := append([]byte{}, hash[:]...)
	data = append(data, flags, 0, 0, 0, 0)
	binary.BigEndian.PutUint32(data[33:], count)
	return data
}

type authenticatorFixture struct {
	key        *ecdsa.PrivateKey
	id, handle []byte
}

func registerFixture(t *testing.T, h http.Handler, session *http.Cookie) authenticatorFixture {
	t.Helper()
	begin := passkeyHTTP(t, h, "/api/passkeys/register/begin", map[string]string{"nome": "Meu celular", "senha": "admin-test-password"}, session)
	options := credentialOptions(t, begin)
	selection := options["authenticatorSelection"].(map[string]any)
	if selection["userVerification"] != "required" || selection["residentKey"] != "required" {
		t.Fatal("registration must require user verification and resident credential")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := base64.RawURLEncoding.DecodeString(options["user"].(map[string]any)["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	if _, err = rand.Read(id); err != nil {
		t.Fatal(err)
	}
	cose, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: key.X.FillBytes(make([]byte, 32)), -3: key.Y.FillBytes(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	authData := fixtureAuthData("localhost", 0x45, 0)
	authData = append(authData, make([]byte, 16)...)
	authData = append(authData, 0, byte(len(id)))
	authData = append(authData, id...)
	authData = append(authData, cose...)
	attestation, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": authData})
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"id": base64url(id), "rawId": base64url(id), "type": "public-key", "clientExtensionResults": map[string]any{}, "response": map[string]any{
		"clientDataJSON": base64url(fixtureClient(t, "webauthn.create", options["challenge"].(string), "http://localhost")), "attestationObject": base64url(attestation), "transports": []string{"internal"},
	}}
	done := passkeyHTTP(t, h, "/api/passkeys/register/finish", body, session, findCookie(t, begin, challengeCookieName("register")))
	if done.Code != 200 {
		t.Fatalf("registration: %d %s", done.Code, done.Body)
	}
	return authenticatorFixture{key: key, id: id, handle: handle}
}
func (a authenticatorFixture) assertion(t *testing.T, challenge, origin, rp string, flags byte, count uint32) map[string]any {
	t.Helper()
	client := fixtureClient(t, "webauthn.get", challenge, origin)
	authData := fixtureAuthData(rp, flags, count)
	clientHash := sha256.Sum256(client)
	signed := append(append([]byte{}, authData...), clientHash[:]...)
	digest := sha256.Sum256(signed)
	signature, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{"id": base64url(a.id), "rawId": base64url(a.id), "type": "public-key", "clientExtensionResults": map[string]any{}, "response": map[string]any{
		"clientDataJSON": base64url(client), "authenticatorData": base64url(authData), "signature": base64url(signature), "userHandle": base64url(a.handle),
	}}
}
func TestPasskeyRegistrationLoginAndReplay(t *testing.T) {
	database = filepath.Join(t.TempDir(), "passkeys.sqlite")
	passkeyOrigin = ""
	t.Cleanup(func() { passkeyOrigin = "" })
	h := routes()
	session := loginTest(t, h, "admin", "admin-test-password")
	fixture := registerFixture(t, h, session)
	begin := passkeyHTTP(t, h, "/api/passkeys/login/begin", map[string]any{})
	options := credentialOptions(t, begin)
	if options["userVerification"] != "required" || options["rpId"] != "localhost" {
		t.Fatal("login must require verification for configured rp")
	}
	response := fixture.assertion(t, options["challenge"].(string), "http://localhost", "localhost", 0x05, 1)
	challenge := findCookie(t, begin, challengeCookieName("login"))
	done := passkeyHTTP(t, h, "/api/passkeys/login/finish", response, challenge)
	if done.Code != 200 {
		t.Fatalf("passkey login: %d %s", done.Code, done.Body)
	}
	authCookie := findCookie(t, done, "reembolso_session")
	if !authCookie.HttpOnly || authCookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("session cookie protections")
	}
	w := request(t, h, "GET", "/api/me", nil, authCookie)
	if w.Code != 200 {
		t.Fatal("passkey session not accepted")
	}
	replay := passkeyHTTP(t, h, "/api/passkeys/login/finish", response, challenge)
	if replay.Code != 400 {
		t.Fatal("replayed challenge accepted")
	}
}
func TestPasskeysRejectInvalidAssertions(t *testing.T) {
	database = filepath.Join(t.TempDir(), "invalid-passkeys.sqlite")
	passkeyOrigin = ""
	t.Cleanup(func() { passkeyOrigin = "" })
	h := routes()
	fixture := registerFixture(t, h, loginTest(t, h, "admin", "admin-test-password"))
	for _, scenario := range []string{"challenge", "origin", "rp", "user-verification", "signature", "handle"} {
		t.Run(scenario, func(t *testing.T) {
			begin := passkeyHTTP(t, h, "/api/passkeys/login/begin", map[string]any{})
			options := credentialOptions(t, begin)
			challenge, origin, rp, flags := options["challenge"].(string), "http://localhost", "localhost", byte(0x05)
			switch scenario {
			case "challenge":
				challenge = base64url(make([]byte, 32))
			case "origin":
				origin = "https://evil.example"
			case "rp":
				rp = "evil.example"
			case "user-verification":
				flags = 0x01
			}
			response := fixture.assertion(t, challenge, origin, rp, flags, 1)
			payload := response["response"].(map[string]any)
			if scenario == "signature" {
				payload["signature"] = base64url([]byte("invalid"))
			}
			if scenario == "handle" {
				payload["userHandle"] = base64url(make([]byte, 32))
			}
			done := passkeyHTTP(t, h, "/api/passkeys/login/finish", response, findCookie(t, begin, challengeCookieName("login")))
			if done.Code != 401 {
				t.Fatalf("%s accepted: %d %s", scenario, done.Code, done.Body)
			}
			for _, cookie := range done.Result().Cookies() {
				if cookie.Name == "reembolso_session" {
					t.Fatal("invalid login minted session")
				}
			}
		})
	}
}
func TestPasskeyOriginAndAccessPolicy(t *testing.T) {
	for _, origin := range []string{"http://example.com", "https://example.com/path", "https://example.com?x=1", "https://user@example.com", "https://127.0.0.1", "https://example.com:99999"} {
		if _, err := parsePasskeyOrigin(origin); err == nil {
			t.Fatalf("invalid origin accepted: %s", origin)
		}
	}
	database = filepath.Join(t.TempDir(), "policy.sqlite")
	passkeyOrigin = "https://reembolso.example"
	t.Cleanup(func() { passkeyOrigin = "" })
	h := routes()
	for _, path := range []string{"/api/passkeys", "/api/passkeys/register/begin", "/api/passkeys/register/finish"} {
		w := request(t, h, "POST", path, map[string]any{}, nil)
		if w.Code != 401 {
			t.Fatalf("anonymous registration/list: %s %d", path, w.Code)
		}
	}
	w := passkeyHTTP(t, h, "/api/passkeys/login/begin", map[string]any{})
	if w.Code != 400 {
		t.Fatal("wrong Host accepted")
	}
	req := httptest.NewRequest("POST", "https://reembolso.example/api/passkeys/login/begin", nil)
	req.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, req)
	if recorder.Code != 403 {
		t.Fatal("cross-origin initiation accepted")
	}
}
