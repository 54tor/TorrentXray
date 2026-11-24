package server

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	altcha "github.com/altcha-org/altcha-lib-go"

	"mkvcheck/internal/abuse"
	"mkvcheck/internal/findings"
	"mkvcheck/internal/store"
)

const (
	cleDeTest         = "cle-de-test-0123456789-0123456789"
	difficulteTest    = 50
	corpsMinimal      = `{"mode":"structure","userAgent":"test"}`
	enTeteAltcha      = "X-Altcha"
	typeJSON          = "application/json"
	identifiantAbsent = "AAAAAAAAAAAAAAAAAAAAAA"
)

type environnement struct {
	handler http.Handler
}

func nouvelEnvironnement(t *testing.T, tailleMax int64, rafale int) environnement {
	t.Helper()
	reserve, err := store.Open(t.TempDir(), tailleMax, 48*time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	garde := abuse.New(abuse.Config{
		HMACKey:       []byte(cleDeTest),
		Difficulty:    difficulteTest,
		RatePerMinute: 60,
		Burst:         rafale,
		ClientIP:      abuse.ClientIPFromHeader(""),
	}, time.Now)
	statique := fstest.MapFS{
		"index.html":              {Data: []byte("<html>accueil</html>")},
		"app.js":                  {Data: []byte("x")},
		"vendor/ffmpeg/core.wasm": {Data: []byte{0, 'a', 's', 'm'}},
	}
	return environnement{handler: New(Config{}, reserve, garde, statique)}
}

func (e environnement) servir(methode, chemin string, corps io.Reader, entetes map[string]string) *httptest.ResponseRecorder {
	requete := httptest.NewRequest(methode, chemin, corps)
	for nom, valeur := range entetes {
		requete.Header.Set(nom, valeur)
	}
	enregistrement := httptest.NewRecorder()
	e.handler.ServeHTTP(enregistrement, requete)
	return enregistrement
}

func (e environnement) preuve(t *testing.T) string {
	t.Helper()
	reponse := e.servir(http.MethodGet, "/api/challenge", nil, nil)
	var defi altcha.Challenge
	if err := json.Unmarshal(reponse.Body.Bytes(), &defi); err != nil {
		t.Fatalf("défi illisible : %v", err)
	}
	solution, err := altcha.SolveChallenge(defi.Challenge, defi.Salt, altcha.Algorithm(defi.Algorithm), int(defi.MaxNumber), 0, nil)
	if err != nil || solution == nil {
		t.Fatalf("défi non résolu : %v", err)
	}
	octets, _ := json.Marshal(altcha.Payload{
		Algorithm: defi.Algorithm, Challenge: defi.Challenge,
		Number: int64(solution.Number), Salt: defi.Salt, Signature: defi.Signature,
	})
	return base64.StdEncoding.EncodeToString(octets)
}

func (e environnement) publier(t *testing.T, corps string) *httptest.ResponseRecorder {
	t.Helper()
	return e.servir(http.MethodPost, "/api/reports", strings.NewReader(corps), map[string]string{enTeteAltcha: e.preuve(t)})
}

func TestPostReports_CreatesReportAndPermalinkServesAllThreeFormats(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 10)
	reponse := env.publier(t, corpsMinimal)
	if reponse.Code != http.StatusCreated {
		t.Fatalf("valeurAttendue=201, valeurObtenue=%d (%s)", reponse.Code, reponse.Body)
	}
	var cree struct{ ID, URL, ExpiresAt string }
	json.Unmarshal(reponse.Body.Bytes(), &cree)
	if len(cree.ID) != 22 || cree.URL != "/r/"+cree.ID || cree.ExpiresAt == "" {
		t.Fatalf("réponse inattendue : %+v", cree)
	}
	cas := []struct{ suffixe, typeAttendu string }{
		{"", "text/html; charset=utf-8"},
		{".md", "text/markdown; charset=utf-8"},
		{".bb", "text/plain; charset=utf-8"},
	}
	for _, cas := range cas {
		lecture := env.servir(http.MethodGet, "/r/"+cree.ID+cas.suffixe, nil, nil)
		if lecture.Code != http.StatusOK || lecture.Header().Get("Content-Type") != cas.typeAttendu {
			t.Fatalf("%q : code=%d type=%q", cas.suffixe, lecture.Code, lecture.Header().Get("Content-Type"))
		}
		if lecture.Header().Get("Cache-Control") != "no-store" || lecture.Header().Get("X-Robots-Tag") != "noindex" {
			t.Fatalf("%q : en-têtes de non-indexation absents", cas.suffixe)
		}
		if lecture.Body.Len() == 0 {
			t.Fatalf("%q : corps vide", cas.suffixe)
		}
	}
}

func TestPostReports_RejectsMissingProofOfWork(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 10)
	reponse := env.servir(http.MethodPost, "/api/reports", strings.NewReader(corpsMinimal), nil)
	verifierErreur(t, reponse, http.StatusForbidden, "proof_of_work")
}

func TestPostReports_RejectsOversizedBody(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 10)
	gros := `{"userAgent":"` + strings.Repeat("a", findings.MaxBodyBytes) + `"}`
	verifierErreur(t, env.publier(t, gros), http.StatusRequestEntityTooLarge, "too_large")
}

func TestPostReports_RejectsInvalidJSON(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 10)
	verifierErreur(t, env.publier(t, "{pas du json"), http.StatusBadRequest, "invalid_json")
}

func TestPostReports_RejectsOversizedProofHeader(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 10)
	reponse := env.servir(http.MethodPost, "/api/reports", strings.NewReader(corpsMinimal), map[string]string{enTeteAltcha: strings.Repeat("a", 5000)})
	verifierErreur(t, reponse, http.StatusBadRequest, "invalid_request")
}

func TestPostReports_DoesNotConsumeProofWhenBodyIsInvalid(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 10)
	preuve := env.preuve(t)
	rejet := env.servir(http.MethodPost, "/api/reports", strings.NewReader("{pas du json"), map[string]string{enTeteAltcha: preuve})
	verifierErreur(t, rejet, http.StatusBadRequest, "invalid_json")
	succes := env.servir(http.MethodPost, "/api/reports", strings.NewReader(corpsMinimal), map[string]string{enTeteAltcha: preuve})
	if succes.Code != http.StatusCreated {
		t.Fatalf("valeurAttendue=201, valeurObtenue=%d", succes.Code)
	}
}

func TestPostReports_ReturnsRateLimitedAfterBurst(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 2)
	for range 2 {
		env.servir(http.MethodPost, "/api/reports", strings.NewReader(corpsMinimal), nil)
	}
	verifierErreur(t, env.servir(http.MethodPost, "/api/reports", strings.NewReader(corpsMinimal), nil), http.StatusTooManyRequests, "rate_limited")
}

func TestGetChallenge_IsRateLimited(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 1)
	env.servir(http.MethodGet, "/api/challenge", nil, nil)
	verifierErreur(t, env.servir(http.MethodGet, "/api/challenge", nil, nil), http.StatusTooManyRequests, "rate_limited")
}

func TestPostReports_ReturnsStorageFullWhenStoreFull(t *testing.T) {
	env := nouvelEnvironnement(t, 1, 10)
	if premier := env.publier(t, corpsMinimal); premier.Code != http.StatusCreated {
		t.Fatalf("premier dépôt : %d", premier.Code)
	}
	verifierErreur(t, env.publier(t, corpsMinimal), http.StatusServiceUnavailable, "storage_full")
}

func TestGetReport_UnknownOrExpiredIdReturns404(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 10)
	for _, chemin := range []string{"/r/" + identifiantAbsent, "/r/" + identifiantAbsent + ".md", "/r/" + identifiantAbsent + ".bb", "/r/court", "/r/..%2f..%2fetc", "/r/" + identifiantAbsent + ".txt"} {
		if code := env.servir(http.MethodGet, chemin, nil, nil).Code; code != http.StatusNotFound {
			t.Fatalf("%s : valeurAttendue=404, valeurObtenue=%d", chemin, code)
		}
	}
}

func TestResponses_CarryStrictSecurityHeaders(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 10)
	attendus := map[string]string{
		"Content-Security-Policy": "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
		"Permissions-Policy":      "camera=(), microphone=(), geolocation=()",
	}
	for _, chemin := range []string{"/", "/healthz", "/inconnu", "/r/" + identifiantAbsent} {
		reponse := env.servir(http.MethodGet, chemin, nil, nil)
		for nom, valeurAttendue := range attendus {
			if valeurObtenue := reponse.Header().Get(nom); valeurObtenue != valeurAttendue {
				t.Fatalf("%s %s : valeurAttendue=%q, valeurObtenue=%q", chemin, nom, valeurAttendue, valeurObtenue)
			}
		}
	}
}

func TestStatic_ServesWasmWithApplicationWasmContentType(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 10)
	reponse := env.servir(http.MethodGet, "/static/vendor/ffmpeg/core.wasm", nil, nil)
	if reponse.Code != http.StatusOK || reponse.Header().Get("Content-Type") != "application/wasm" {
		t.Fatalf("code=%d type=%q", reponse.Code, reponse.Header().Get("Content-Type"))
	}
	if reponse.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("cache vendor : %q", reponse.Header().Get("Cache-Control"))
	}
}

func TestStatic_ServesIndexAtRootWithShortCache(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 10)
	racine := env.servir(http.MethodGet, "/", nil, nil)
	if racine.Code != http.StatusOK || !strings.Contains(racine.Body.String(), "accueil") {
		t.Fatalf("racine : code=%d", racine.Code)
	}
	script := env.servir(http.MethodGet, "/static/app.js", nil, nil)
	if script.Code != http.StatusOK || strings.Contains(script.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("script : code=%d cache=%q", script.Code, script.Header().Get("Cache-Control"))
	}
}

func TestHealthz_ReturnsOK(t *testing.T) {
	env := nouvelEnvironnement(t, 1<<20, 10)
	if code := env.servir(http.MethodGet, "/healthz", nil, nil).Code; code != http.StatusOK {
		t.Fatalf("valeurAttendue=200, valeurObtenue=%d", code)
	}
}

func verifierErreur(t *testing.T, reponse *httptest.ResponseRecorder, codeAttendu int, erreurAttendue string) {
	t.Helper()
	var corps struct{ Error string }
	json.Unmarshal(reponse.Body.Bytes(), &corps)
	if reponse.Code != codeAttendu || corps.Error != erreurAttendue {
		t.Fatalf("valeurAttendue=%d/%s, valeurObtenue=%d/%s", codeAttendu, erreurAttendue, reponse.Code, corps.Error)
	}
}

func TestConfigFromEnv_RejectsShortHMACKey(t *testing.T) {
	t.Setenv("MKVCHECK_HMAC_KEY", "trop-court")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("clé courte acceptée")
	}
}

func TestConfigFromEnv_GeneratesKeyWhenAbsent(t *testing.T) {
	t.Setenv("MKVCHECK_HMAC_KEY", "")
	cfg, err := ConfigFromEnv()
	if err != nil || len(cfg.HMACKey) < 32 {
		t.Fatalf("err=%v len=%d", err, len(cfg.HMACKey))
	}
}

func TestConfigFromEnv_ReadsNumericVariablesAndRejectsGarbage(t *testing.T) {
	t.Setenv("MKVCHECK_HMAC_KEY", cleDeTest)
	t.Setenv("MKVCHECK_MAX_STORE_MB", "2")
	cfg, err := ConfigFromEnv()
	if err != nil || cfg.MaxStoreBytes != 2<<20 {
		t.Fatalf("err=%v max=%d", err, cfg.MaxStoreBytes)
	}
	t.Setenv("MKVCHECK_BURST", "abc")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("valeur non numérique acceptée")
	}
}
