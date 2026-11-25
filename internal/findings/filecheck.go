package findings

const (
	maxTracks       = 64
	maxAnomalies    = 100
	maxErrorLines   = 500
	maxVersionLines = 40
)

var (
	allowedMatchKinds = []string{"full_path", "relative_path", "name_and_size"}
	allowedThreading  = []string{"mt", "st"}
)

type Track struct {
	Type                 string  `json:"type"`
	Number               int     `json:"number"`
	Codec                string  `json:"codec"`
	Language             string  `json:"language"`
	PacketCount          int     `json:"packetCount"`
	ZeroSizePackets      int     `json:"zeroSizePackets"`
	LastTimestampSeconds float64 `json:"lastTimestampSeconds"`
}

type Anomaly struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

type FfmpegError struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

type TorrentMatch struct {
	TorrentPath string `json:"torrentPath"`
	TorrentSize int    `json:"torrentSize"`
	LocalPath   string `json:"localPath"`
	LocalSize   int    `json:"localSize"`
	MatchKind   string `json:"matchKind"`
}

type FileCheck struct {
	FileName              string        `json:"fileName"`
	FileSize              int           `json:"fileSize"`
	TorrentMatch          *TorrentMatch `json:"torrentMatch"`
	ContainerName         *string       `json:"containerName"`
	DurationSeconds       float64       `json:"durationSeconds"`
	PrescanMilliseconds   int           `json:"prescanMilliseconds"`
	PrescanError          *string       `json:"prescanError"`
	Tracks                []Track       `json:"tracks"`
	Anomalies             []Anomaly     `json:"anomalies"`
	FfmpegRan             bool          `json:"ffmpegRan"`
	FfmpegExitCode        int           `json:"ffmpegExitCode"`
	FfmpegThreading       string        `json:"ffmpegThreading"`
	FfmpegTotalErrorLines int           `json:"ffmpegTotalErrorLines"`
	FfmpegErrors          []FfmpegError `json:"ffmpegErrors"`
	FfmpegVersionLines    []string      `json:"ffmpegVersionLines"`
	MediaInfo             string        `json:"mediaInfo"`
}

func fileCheckFromPayload(payload map[string]any) FileCheck {
	file := mapOf(payload["file"])
	prescan := mapOf(payload["prescan"])
	ffmpeg := mapOf(payload["ffmpeg"])
	return FileCheck{
		FileName:              textOf(file["name"]),
		FileSize:              nonNegativeIntegerOf(file["size"]),
		TorrentMatch:          torrentMatchFromPayload(mapOf(payload["torrentMatch"])),
		ContainerName:         optionalTextOf(prescan, "containerName"),
		DurationSeconds:       nonNegativeDecimalOf(prescan["durationSeconds"]),
		PrescanMilliseconds:   nonNegativeIntegerOf(prescan["elapsedMilliseconds"]),
		PrescanError:          optionalTextOf(prescan, "readError"),
		Tracks:                tracksFromPayload(prescan["tracks"]),
		Anomalies:             anomaliesFromPayload(prescan["anomalies"]),
		FfmpegRan:             len(ffmpeg) > 0,
		FfmpegExitCode:        integerOf(ffmpeg["exitCode"]),
		FfmpegThreading:       enumOf(ffmpeg["threading"], allowedThreading, "st"),
		FfmpegTotalErrorLines: nonNegativeIntegerOf(ffmpeg["totalErrorLines"]),
		FfmpegErrors:          ffmpegErrorsFromPayload(ffmpeg["errors"]),
		FfmpegVersionLines:    textsOf(ffmpeg["versionLines"], maxVersionLines),
		MediaInfo:             multilineTextOf(payload["mediaInfo"]),
	}
}

func torrentMatchFromPayload(match map[string]any) *TorrentMatch {
	if len(match) == 0 {
		return nil
	}
	return &TorrentMatch{
		TorrentPath: textOf(match["torrentPath"]),
		TorrentSize: nonNegativeIntegerOf(match["torrentSize"]),
		LocalPath:   textOf(match["localPath"]),
		LocalSize:   nonNegativeIntegerOf(match["localSize"]),
		MatchKind:   enumOf(match["matchKind"], allowedMatchKinds, "name_and_size"),
	}
}

func tracksFromPayload(rawTracks any) []Track {
	tracks := []Track{}
	for _, rawTrack := range limitedList(rawTracks, maxTracks) {
		track := mapOf(rawTrack)
		tracks = append(tracks, Track{
			Type:                 textOf(track["type"]),
			Number:               integerOf(track["number"]),
			Codec:                textOf(track["codec"]),
			Language:             textOf(track["language"]),
			PacketCount:          nonNegativeIntegerOf(track["packetCount"]),
			ZeroSizePackets:      nonNegativeIntegerOf(track["zeroSizePackets"]),
			LastTimestampSeconds: nonNegativeDecimalOf(track["lastTimestampSeconds"]),
		})
	}
	return tracks
}

func anomaliesFromPayload(rawAnomalies any) []Anomaly {
	anomalies := []Anomaly{}
	for _, rawAnomaly := range limitedList(rawAnomalies, maxAnomalies) {
		anomaly := mapOf(rawAnomaly)
		anomalies = append(anomalies, Anomaly{Code: textOf(anomaly["code"]), Detail: textOf(anomaly["detail"])})
	}
	return anomalies
}

func ffmpegErrorsFromPayload(rawErrors any) []FfmpegError {
	ffmpegErrors := []FfmpegError{}
	for _, rawError := range limitedList(rawErrors, maxErrorLines) {
		pair := listOf(rawError)
		count := 1
		if len(pair) > 1 {
			count = max(1, integerOf(pair[1]))
		}
		message := ""
		if len(pair) > 0 {
			message = textOf(pair[0])
		}
		ffmpegErrors = append(ffmpegErrors, FfmpegError{Message: message, Count: count})
	}
	return ffmpegErrors
}
