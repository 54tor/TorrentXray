package abuse

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	altcha "github.com/altcha-org/altcha-lib-go"
	"golang.org/x/time/rate"
)

const (
	testDifficulty = 50
	cleDeTest      = "cle-de-test-0123456789-0123456789"
	autreCleDeTest = "autre-cle-0123456789-0123456789-ab"
)

var instantInitial = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

type horlogeMobile struct{ instant time.Time }

func (horloge *horlogeMobile) Now() time.Time { return horloge.instant }

func nouveauGarde(horloge *horlogeMobile, ratePerMinute, burst int) *Guard {
	return New(Config{
		HMACKey:       []byte(cleDeTest),
		Difficulty:    testDifficulty,
		RatePerMinute: ratePerMinute,
		Burst:         burst,
		ClientIP:      ClientIPFromHeader(""),
	}, horloge.Now)
}

func resoudreDefi(t *testing.T, garde *Guard) string {
	t.Helper()
	enregistrement := httptest.NewRecorder()
	garde.ChallengeHandler().ServeHTTP(enregistrement, httptest.NewRequest(http.MethodGet, "/challenge", nil))
	var defi altcha.Challenge
	if err := json.Unmarshal(enregistrement.Body.Bytes(), &defi); err != nil {
		t.Fatalf("défi illisible : %v", err)
	}
	solution, err := altcha.SolveChallenge(defi.Challenge, defi.Salt, altcha.Algorithm(defi.Algorithm), int(defi.MaxNumber), 0, nil)
	if err != nil || solution == nil {
		t.Fatalf("défi non résolu : %v", err)
	}
	return encoderCharge(t, altcha.Payload{
		Algorithm: defi.Algorithm,
		Challenge: defi.Challenge,
		Number:    int64(solution.Number),
		Salt:      defi.Salt,
		Signature: defi.Signature,
	})
}

func encoderCharge(t *testing.T, charge altcha.Payload) string {
	t.Helper()
	octets, err := json.Marshal(charge)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(octets)
}

func TestVerifySolution_AcceptsValidSolutionOnce(t *testing.T) {
	garde := nouveauGarde(&horlogeMobile{instantInitial}, 60, 5)
	valeurObtenue := garde.VerifySolution(resoudreDefi(t, garde))
	if !valeurObtenue {
		t.Fatal("valeurAttendue=true, valeurObtenue=false")
	}
}

func TestVerifySolution_RejectsReplayedSolution(t *testing.T) {
	garde := nouveauGarde(&horlogeMobile{instantInitial}, 60, 5)
	charge := resoudreDefi(t, garde)
	garde.VerifySolution(charge)
	if valeurObtenue := garde.VerifySolution(charge); valeurObtenue {
		t.Fatal("valeurAttendue=false, valeurObtenue=true")
	}
}

func TestVerifySolution_RejectsTamperedPayload(t *testing.T) {
	garde := nouveauGarde(&horlogeMobile{instantInitial}, 60, 5)
	octets, _ := base64.StdEncoding.DecodeString(resoudreDefi(t, garde))
	var charge altcha.Payload
	_ = json.Unmarshal(octets, &charge)
	charge.Number++
	if valeurObtenue := garde.VerifySolution(encoderCharge(t, charge)); valeurObtenue {
		t.Fatal("valeurAttendue=false, valeurObtenue=true")
	}
	if valeurObtenue := garde.VerifySolution("pas-du-base64"); valeurObtenue {
		t.Fatal("charge illisible acceptée")
	}
}

func TestVerifySolution_RejectsExpiredChallenge(t *testing.T) {
	horloge := &horlogeMobile{instantInitial}
	garde := nouveauGarde(horloge, 60, 5)
	charge := resoudreDefi(t, garde)
	horloge.instant = instantInitial.Add(challengeLifetime + time.Second)
	if valeurObtenue := garde.VerifySolution(charge); valeurObtenue {
		t.Fatal("valeurAttendue=false, valeurObtenue=true")
	}
}

func TestVerifySolution_RejectsChallengeSignedWithOtherKey(t *testing.T) {
	autreGarde := New(Config{HMACKey: []byte(autreCleDeTest), Difficulty: testDifficulty, RatePerMinute: 60, Burst: 5}, (&horlogeMobile{instantInitial}).Now)
	garde := nouveauGarde(&horlogeMobile{instantInitial}, 60, 5)
	if valeurObtenue := garde.VerifySolution(resoudreDefi(t, autreGarde)); valeurObtenue {
		t.Fatal("valeurAttendue=false, valeurObtenue=true")
	}
}

func TestVerifySolution_PurgesConsumedEntriesAfterExpiry(t *testing.T) {
	horloge := &horlogeMobile{instantInitial}
	garde := nouveauGarde(horloge, 60, 5)
	garde.VerifySolution(resoudreDefi(t, garde))
	horloge.instant = instantInitial.Add(challengeLifetime + time.Minute)
	garde.VerifySolution("x")
	if valeurObtenue := len(garde.consumed); valeurObtenue != 0 {
		t.Fatalf("valeurAttendue=0, valeurObtenue=%d", valeurObtenue)
	}
}

func requeteDepuis(adresse string) *http.Request {
	requete := httptest.NewRequest(http.MethodPost, "/reports", nil)
	requete.RemoteAddr = adresse
	return requete
}

func TestAllowRequest_BlocksAfterBurstForSameIP(t *testing.T) {
	garde := nouveauGarde(&horlogeMobile{instantInitial}, 60, 2)
	garde.AllowRequest(requeteDepuis("192.0.2.1:1000"))
	garde.AllowRequest(requeteDepuis("192.0.2.1:1001"))
	if valeurObtenue := garde.AllowRequest(requeteDepuis("192.0.2.1:1002")); valeurObtenue {
		t.Fatal("valeurAttendue=false, valeurObtenue=true")
	}
}

func TestAllowRequest_RefillsOverTime(t *testing.T) {
	horloge := &horlogeMobile{instantInitial}
	garde := nouveauGarde(horloge, 60, 1)
	garde.AllowRequest(requeteDepuis("192.0.2.1:1"))
	horloge.instant = instantInitial.Add(2 * time.Second)
	if valeurObtenue := garde.AllowRequest(requeteDepuis("192.0.2.1:1")); !valeurObtenue {
		t.Fatal("valeurAttendue=true, valeurObtenue=false")
	}
}

func TestAllowRequest_IsolatesDistinctIPs(t *testing.T) {
	garde := nouveauGarde(&horlogeMobile{instantInitial}, 60, 1)
	garde.AllowRequest(requeteDepuis("192.0.2.1:1"))
	if valeurObtenue := garde.AllowRequest(requeteDepuis("192.0.2.2:1")); !valeurObtenue {
		t.Fatal("valeurAttendue=true, valeurObtenue=false")
	}
}

func TestAllowRequest_DropsIdleLimiters(t *testing.T) {
	horloge := &horlogeMobile{instantInitial}
	garde := nouveauGarde(horloge, 60, 1)
	garde.AllowRequest(requeteDepuis("192.0.2.1:1"))
	horloge.instant = instantInitial.Add(limiterIdleLifetime + 2*time.Minute)
	garde.AllowRequest(requeteDepuis("192.0.2.2:1"))
	if valeurObtenue := len(garde.limiters); valeurObtenue != 1 {
		t.Fatalf("valeurAttendue=1, valeurObtenue=%d", valeurObtenue)
	}
}

func TestClientIPFromHeader_IgnoresHeaderWhenNotConfigured(t *testing.T) {
	requete := requeteDepuis("192.0.2.9:4000")
	requete.Header.Set("X-Forwarded-For", "203.0.113.5")
	valeurObtenue := ClientIPFromHeader("")(requete)
	if valeurAttendue := "192.0.2.9"; valeurObtenue != valeurAttendue {
		t.Fatalf("valeurAttendue=%s, valeurObtenue=%s", valeurAttendue, valeurObtenue)
	}
}

func TestClientIPFromHeader_UsesRightmostEntryWhenConfigured(t *testing.T) {
	requete := requeteDepuis("192.0.2.9:4000")
	requete.Header.Set("X-Forwarded-For", "198.51.100.7, 203.0.113.5")
	valeurObtenue := ClientIPFromHeader("X-Forwarded-For")(requete)
	if valeurAttendue := "203.0.113.5"; valeurObtenue != valeurAttendue {
		t.Fatalf("valeurAttendue=%s, valeurObtenue=%s", valeurAttendue, valeurObtenue)
	}
}

func TestClientIPFromHeader_FallsBackOnMissingOrInvalidHeader(t *testing.T) {
	requete := requeteDepuis("192.0.2.9:4000")
	requete.Header.Set("X-Forwarded-For", "pas-une-ip")
	if valeurObtenue := ClientIPFromHeader("X-Forwarded-For")(requete); valeurObtenue != "192.0.2.9" {
		t.Fatalf("valeurObtenue=%s", valeurObtenue)
	}
	requete.Header.Del("X-Forwarded-For")
	if valeurObtenue := ClientIPFromHeader("X-Forwarded-For")(requete); valeurObtenue != "192.0.2.9" {
		t.Fatalf("valeurObtenue=%s", valeurObtenue)
	}
}

func TestAllowRequest_GroupsIPv6AddressesByPrefix(t *testing.T) {
	garde := nouveauGarde(&horlogeMobile{instantInitial}, 60, 1)
	garde.AllowRequest(requeteDepuis("[2001:db8:1:2::1]:1"))
	if valeurObtenue := garde.AllowRequest(requeteDepuis("[2001:db8:1:2:ffff::9]:1")); valeurObtenue {
		t.Fatal("valeurAttendue=false, valeurObtenue=true")
	}
}

func TestAllowRequest_RejectsNewIPsWhenLimiterTableIsFull(t *testing.T) {
	garde := nouveauGarde(&horlogeMobile{instantInitial}, 60, 1)
	for index := 0; index < maxLimiters; index++ {
		garde.limiters[strconv.Itoa(index)] = &ipLimiter{limiter: rate.NewLimiter(1, 1), lastSeen: instantInitial}
	}
	if valeurObtenue := garde.AllowRequest(requeteDepuis("192.0.2.1:1")); valeurObtenue {
		t.Fatal("valeurAttendue=false, valeurObtenue=true")
	}
}

func TestNew_PanicsWhenHMACKeyIsTooShort(t *testing.T) {
	for _, longueur := range []int{0, 31} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("valeurAttendue=panique pour %d octets, valeurObtenue=aucune", longueur)
				}
			}()
			New(Config{HMACKey: make([]byte, longueur)}, time.Now)
		}()
	}
}

func TestNew_AcceptsHMACKeyOf32Bytes(t *testing.T) {
	if New(Config{HMACKey: make([]byte, 32)}, time.Now) == nil {
		t.Fatal("valeurAttendue=garde, valeurObtenue=nil")
	}
}

func TestClientIPFromHeader_UsesLastLineOfRepeatedHeader(t *testing.T) {
	requete := requeteDepuis("192.0.2.9:4000")
	requete.Header.Add("X-Forwarded-For", "198.51.100.7")
	requete.Header.Add("X-Forwarded-For", "203.0.113.5")
	valeurObtenue := ClientIPFromHeader("X-Forwarded-For")(requete)
	if valeurAttendue := "203.0.113.5"; valeurObtenue != valeurAttendue {
		t.Fatalf("valeurAttendue=%s, valeurObtenue=%s", valeurAttendue, valeurObtenue)
	}
}

func TestVerifySolution_ConcurrentReplayAcceptedExactlyOnce(t *testing.T) {
	const nombreDeGoroutines = 50
	garde := nouveauGarde(&horlogeMobile{instantInitial}, 60, 5)
	charge := resoudreDefi(t, garde)
	var groupe sync.WaitGroup
	var verrou sync.Mutex
	acceptees := 0
	depart := make(chan struct{})
	for range nombreDeGoroutines {
		groupe.Add(1)
		go func() {
			defer groupe.Done()
			<-depart
			if garde.VerifySolution(charge) {
				verrou.Lock()
				acceptees++
				verrou.Unlock()
			}
		}()
	}
	close(depart)
	groupe.Wait()
	if valeurObtenue := acceptees; valeurObtenue != 1 {
		t.Fatalf("valeurAttendue=1, valeurObtenue=%d", valeurObtenue)
	}
}
