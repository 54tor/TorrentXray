package server

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strconv"
)

const (
	minimumHMACKeyBytes = 32
	bytesPerMegabyte    = 1 << 20

	defaultAddr          = ":8080"
	defaultDataDir       = "data"
	defaultDifficulty    = 200000
	defaultRatePerMinute = 20
	defaultBurst         = 10
	defaultMaxStoreMB    = 512
)

type Config struct {
	Addr           string
	DataDir        string
	ClientIPHeader string
	HMACKey        []byte
	Difficulty     int64
	RatePerMinute  int
	Burst          int
	MaxStoreBytes  int64
}

func ConfigFromEnv() (Config, error) {
	key, err := hmacKeyFromEnv()
	if err != nil {
		return Config{}, err
	}
	difficulty, err := integerFromEnv("MKVCHECK_POW_DIFFICULTY", defaultDifficulty)
	if err != nil {
		return Config{}, err
	}
	ratePerMinute, err := integerFromEnv("MKVCHECK_RATE_PER_MINUTE", defaultRatePerMinute)
	if err != nil {
		return Config{}, err
	}
	burst, err := integerFromEnv("MKVCHECK_BURST", defaultBurst)
	if err != nil {
		return Config{}, err
	}
	maxStoreMB, err := integerFromEnv("MKVCHECK_MAX_STORE_MB", defaultMaxStoreMB)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Addr:           textFromEnv("MKVCHECK_ADDR", defaultAddr),
		DataDir:        textFromEnv("MKVCHECK_DATA_DIR", defaultDataDir),
		ClientIPHeader: os.Getenv("MKVCHECK_IP_HEADER"),
		HMACKey:        key,
		Difficulty:     difficulty,
		RatePerMinute:  int(ratePerMinute),
		Burst:          int(burst),
		MaxStoreBytes:  maxStoreMB * bytesPerMegabyte,
	}, nil
}

func hmacKeyFromEnv() ([]byte, error) {
	configured := os.Getenv("MKVCHECK_HMAC_KEY")
	if configured == "" {
		generated := make([]byte, minimumHMACKeyBytes)
		_, err := rand.Read(generated)
		return generated, err
	}
	if len(configured) < minimumHMACKeyBytes {
		return nil, errors.New("MKVCHECK_HMAC_KEY: au moins 32 octets requis")
	}
	return []byte(configured), nil
}

func textFromEnv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func integerFromEnv(name string, fallback int64) (int64, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s: entier strictement positif attendu", name)
	}
	return value, nil
}
