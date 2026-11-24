package findings

import "strings"

const (
	maxPieceRanges    = 200
	maxFilesPerRange  = 20
	maxTorrentFiles   = 1000
	healthyFileStatus = "healthy"
)

var (
	allowedRangeKinds   = []string{"zero", "mismatch"}
	allowedFileStatuses = []string{"healthy", "suspect", "missing", "size_mismatch", "unverifiable"}
)

type PieceRange struct {
	Kind        string   `json:"kind"`
	FirstPiece  int      `json:"firstPiece"`
	LastPiece   int      `json:"lastPiece"`
	StartOffset int      `json:"startOffset"`
	EndOffset   int      `json:"endOffset"`
	Files       []string `json:"files"`
}

type TorrentFile struct {
	Path      string `json:"path"`
	Size      int    `json:"size"`
	Status    string `json:"status"`
	LocalPath string `json:"localPath"`
}

type TorrentCheck struct {
	Name               string        `json:"name"`
	InfoHash           string        `json:"infoHash"`
	PieceLength        int           `json:"pieceLength"`
	PieceCount         int           `json:"pieceCount"`
	TotalSize          int           `json:"totalSize"`
	UnverifiablePieces int           `json:"unverifiablePieces"`
	BadPieceCount      int           `json:"badPieceCount"`
	ZeroPieceCount     int           `json:"zeroPieceCount"`
	PieceRanges        []PieceRange  `json:"pieceRanges"`
	Files              []TorrentFile `json:"files"`
}

func torrentCheckFromPayload(payload map[string]any) *TorrentCheck {
	if len(payload) == 0 {
		return nil
	}
	return &TorrentCheck{
		Name:               textOf(payload["name"]),
		InfoHash:           hexDigitsOf(payload["infoHash"]),
		PieceLength:        nonNegativeIntegerOf(payload["pieceLength"]),
		PieceCount:         nonNegativeIntegerOf(payload["pieceCount"]),
		TotalSize:          nonNegativeIntegerOf(payload["totalSize"]),
		UnverifiablePieces: nonNegativeIntegerOf(payload["unverifiablePieces"]),
		BadPieceCount:      nonNegativeIntegerOf(payload["badPieceCount"]),
		ZeroPieceCount:     nonNegativeIntegerOf(payload["zeroPieceCount"]),
		PieceRanges:        pieceRangesFromPayload(payload["pieceRanges"]),
		Files:              torrentFilesFromPayload(payload["files"]),
	}
}

func hexDigitsOf(value any) string {
	isNotHexDigit := func(character rune) bool {
		return !strings.ContainsRune("0123456789abcdefABCDEF", character)
	}
	return strings.ToLower(strings.Map(func(character rune) rune {
		if isNotHexDigit(character) {
			return -1
		}
		return character
	}, textOf(value)))
}

func pieceRangesFromPayload(rawRanges any) []PieceRange {
	ranges := []PieceRange{}
	for _, rawRange := range limitedList(rawRanges, maxPieceRanges) {
		pieceRange := mapOf(rawRange)
		firstPiece := nonNegativeIntegerOf(pieceRange["firstPiece"])
		startOffset := nonNegativeIntegerOf(pieceRange["startOffset"])
		ranges = append(ranges, PieceRange{
			Kind:        enumOf(pieceRange["kind"], allowedRangeKinds, "mismatch"),
			FirstPiece:  firstPiece,
			LastPiece:   max(firstPiece, rangeBound(pieceRange, "lastPiece", firstPiece)),
			StartOffset: startOffset,
			EndOffset:   max(startOffset, rangeBound(pieceRange, "endOffset", startOffset)),
			Files:       textsOf(pieceRange["files"], maxFilesPerRange),
		})
	}
	return ranges
}

func rangeBound(pieceRange map[string]any, key string, fallback int) int {
	if _, present := pieceRange[key]; !present {
		return fallback
	}
	return integerOf(pieceRange[key])
}

func torrentFilesFromPayload(rawFiles any) []TorrentFile {
	files := []TorrentFile{}
	for _, rawFile := range limitedList(rawFiles, maxTorrentFiles) {
		file := mapOf(rawFile)
		files = append(files, TorrentFile{
			Path:      textOf(file["path"]),
			Size:      nonNegativeIntegerOf(file["size"]),
			Status:    enumOf(file["status"], allowedFileStatuses, "unverifiable"),
			LocalPath: textOf(file["localPath"]),
		})
	}
	return files
}
