package findings

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const (
	MaxBodyBytes = 262144
	maxJSONDepth = 32
	maxChecks    = 50
)

var (
	ErrTooLarge    = errors.New("findings: body too large")
	ErrInvalidJSON = errors.New("findings: invalid JSON")
)

var allowedModes = []string{"structure", "decode"}

type Versions struct {
	FfmpegWasm string `json:"ffmpegWasm"`
	FfmpegCore string `json:"ffmpegCore"`
	Mediabunny string `json:"mediabunny"`
}

type Findings struct {
	Mode           string        `json:"mode"`
	StartedAt      time.Time     `json:"startedAt"`
	ElapsedSeconds int           `json:"elapsedSeconds"`
	Torrent        *TorrentCheck `json:"torrent"`
	Checks         []FileCheck   `json:"checks"`
	Versions       Versions      `json:"versions"`
	UserAgent      string        `json:"userAgent"`
}

func Parse(raw []byte) (Findings, error) {
	if len(raw) > MaxBodyBytes {
		return Findings{}, ErrTooLarge
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&decoded); err != nil {
		return Findings{}, ErrInvalidJSON
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Findings{}, ErrInvalidJSON
	}
	payload, isObject := decoded.(map[string]any)
	if !isObject || nestingDepth(payload) > maxJSONDepth {
		return Findings{}, ErrInvalidJSON
	}
	return findingsFromPayload(payload), nil
}

func findingsFromPayload(payload map[string]any) Findings {
	versions := mapOf(payload["versions"])
	return Findings{
		Mode:           enumOf(payload["mode"], allowedModes, "structure"),
		StartedAt:      dateOf(payload["startedAt"]),
		ElapsedSeconds: nonNegativeIntegerOf(payload["elapsedSeconds"]),
		Torrent:        torrentCheckFromPayload(mapOf(payload["torrent"])),
		Checks:         fileChecksFromPayload(payload["checks"]),
		Versions: Versions{
			FfmpegWasm: textOf(versions["ffmpegWasm"]),
			FfmpegCore: textOf(versions["ffmpegCore"]),
			Mediabunny: textOf(versions["mediabunny"]),
		},
		UserAgent: textOf(payload["userAgent"]),
	}
}

func fileChecksFromPayload(rawChecks any) []FileCheck {
	checks := []FileCheck{}
	for _, rawCheck := range limitedList(rawChecks, maxChecks) {
		checks = append(checks, fileCheckFromPayload(mapOf(rawCheck)))
	}
	return checks
}
