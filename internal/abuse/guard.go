package abuse

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	altcha "github.com/altcha-org/altcha-lib-go"
	"golang.org/x/time/rate"
)

const (
	challengeLifetime   = 5 * time.Minute
	limiterIdleLifetime = 10 * time.Minute
	sweepInterval       = time.Minute
	secondsPerMinute    = 60
	maxLimiters         = 100000
	minimumHMACKeyBytes = 32
	ipv6PrefixBits      = 64
)

type Config struct {
	HMACKey       []byte
	Difficulty    int64
	RatePerMinute int
	Burst         int
	ClientIP      func(*http.Request) string
}

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type Guard struct {
	config Config
	now    func() time.Time

	mutex     sync.Mutex
	consumed  map[[sha256.Size]byte]time.Time
	limiters  map[string]*ipLimiter
	lastSweep time.Time
}

func New(cfg Config, now func() time.Time) *Guard {
	if len(cfg.HMACKey) < minimumHMACKeyBytes {
		panic("abuse: HMACKey must be at least " + strconv.Itoa(minimumHMACKeyBytes) + " bytes")
	}
	if cfg.ClientIP == nil {
		cfg.ClientIP = ClientIPFromHeader("")
	}
	return &Guard{
		config:   cfg,
		now:      now,
		consumed: map[[sha256.Size]byte]time.Time{},
		limiters: map[string]*ipLimiter{},
	}
}

func (g *Guard) ChallengeHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		expires := g.now().Add(challengeLifetime)
		challenge, err := altcha.CreateChallenge(altcha.ChallengeOptions{
			HMACKey:   string(g.config.HMACKey),
			MaxNumber: g.config.Difficulty,
			Expires:   &expires,
		})
		if err != nil {
			http.Error(writer, "challenge unavailable", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(writer).Encode(challenge)
	})
}

func (g *Guard) VerifySolution(payload string) bool {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.purgeConsumed()
	charge, expiresAt, ok := g.decodeSignedPayload(payload)
	if !ok {
		return false
	}
	key := sha256.Sum256([]byte(charge.Signature))
	if _, alreadyUsed := g.consumed[key]; alreadyUsed {
		return false
	}
	g.consumed[key] = expiresAt
	return true
}

func (g *Guard) decodeSignedPayload(payload string) (altcha.Payload, time.Time, bool) {
	var charge altcha.Payload
	octets, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || json.Unmarshal(octets, &charge) != nil {
		return charge, time.Time{}, false
	}
	valid, err := altcha.VerifySolutionSafe(charge, string(g.config.HMACKey), false)
	if err != nil || !valid {
		return charge, time.Time{}, false
	}
	expiresAt, hasExpiry := signedExpiry(charge)
	if !hasExpiry || !g.now().Before(expiresAt) {
		return charge, time.Time{}, false
	}
	return charge, expiresAt, true
}

func signedExpiry(charge altcha.Payload) (time.Time, bool) {
	seconds, err := strconv.ParseInt(altcha.ExtractParams(charge).Get("expires"), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0), true
}

func (g *Guard) purgeConsumed() {
	currentTime := g.now()
	for key, expiresAt := range g.consumed {
		if !currentTime.Before(expiresAt) {
			delete(g.consumed, key)
		}
	}
}

func (g *Guard) AllowRequest(request *http.Request) bool {
	currentTime := g.now()
	address := limiterKey(g.config.ClientIP(request))
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.sweepIdleLimiters(currentTime)
	entry, exists := g.limiters[address]
	if !exists {
		if len(g.limiters) >= maxLimiters {
			return false
		}
		entry = &ipLimiter{limiter: rate.NewLimiter(rate.Limit(float64(g.config.RatePerMinute)/secondsPerMinute), g.config.Burst)}
		g.limiters[address] = entry
	}
	entry.lastSeen = currentTime
	return entry.limiter.AllowN(currentTime, 1)
}

func (g *Guard) sweepIdleLimiters(currentTime time.Time) {
	if currentTime.Sub(g.lastSweep) < sweepInterval {
		return
	}
	g.lastSweep = currentTime
	for address, entry := range g.limiters {
		if currentTime.Sub(entry.lastSeen) > limiterIdleLifetime {
			delete(g.limiters, address)
		}
	}
}

func ClientIPFromHeader(header string) func(*http.Request) string {
	return func(request *http.Request) string {
		if header != "" {
			if address := rightmostValidIP(strings.Join(request.Header.Values(header), ",")); address != "" {
				return address
			}
		}
		host, _, err := net.SplitHostPort(request.RemoteAddr)
		if err != nil {
			return request.RemoteAddr
		}
		return host
	}
}

func rightmostValidIP(headerValue string) string {
	entries := strings.Split(headerValue, ",")
	candidate := strings.TrimSpace(entries[len(entries)-1])
	if net.ParseIP(candidate) == nil {
		return ""
	}
	return candidate
}

func limiterKey(address string) string {
	parsed := net.ParseIP(address)
	if parsed == nil || parsed.To4() != nil {
		return address
	}
	return parsed.Mask(net.CIDRMask(ipv6PrefixBits, 128)).String()
}
