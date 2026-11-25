package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"text/template"
	"time"

	"mkvcheck/internal/findings"
)

const DiskImageWarning = "Les images disque (ISO, BDMV, DVD) peuvent produire de faux positifs : un constat n'est pas forcément une corruption."

const (
	emptyValue       = "—"
	timeLayout       = "2006-01-02 15:04:05 MST"
	byteUnitStep     = 1024
	thousandsGroup   = 3
	noBreakSeparator = " "
)

type Report struct {
	ID        string            `json:"id"`
	CreatedAt time.Time         `json:"createdAt"`
	ExpiresAt time.Time         `json:"expiresAt"`
	Findings  findings.Findings `json:"findings"`
}

type rangeRow struct {
	Kind, Pieces, Bytes, File string
}

type view struct {
	Report
	Warning string
}

type sealedView struct {
	view
	Seal string
}

func newSealedView(report Report) sealedView {
	return sealedView{view: newView(report), Seal: markdownSeal(report)}
}

func Seal(body string) string {
	digest := sha256.Sum256([]byte(body))
	return hex.EncodeToString(digest[:])
}

func newView(report Report) view {
	return view{Report: report, Warning: DiskImageWarning}
}

func sharedFuncs() template.FuncMap {
	return template.FuncMap{
		"size":         humanSize,
		"num":          formatNumber,
		"seconds":      func(value float64) string { return fmt.Sprintf("%.1f", value) },
		"dash":         dashIfEmpty,
		"when":         func(moment time.Time) string { return moment.UTC().Format(timeLayout) },
		"verdict":      verdictLabel,
		"readStatus":   readStatus,
		"decodeStatus": decodeStatus,
		"mode":         modeLabel,
		"status":       statusLabel,
		"kind":         kindLabel,
		"threading":    threadingLabel,
		"matchKind":    matchKindLabel,
		"anomaly":      anomalyLabel,
		"matchSummary": matchSummary,
		"rangeRows":    rangeRows,
		"repeat":       repeatPrefix,
		"firstVersion": firstVersionLine,
	}
}

func humanSize(bytes int) string {
	value := float64(bytes)
	units := []string{"o", "Kio", "Mio", "Gio", "Tio"}
	unitIndex := 0
	for value >= byteUnitStep && unitIndex < len(units)-1 {
		value /= byteUnitStep
		unitIndex++
	}
	if unitIndex == 0 {
		return fmt.Sprintf("%d o", bytes)
	}
	return strings.Replace(fmt.Sprintf("%.1f %s", value, units[unitIndex]), ".", ",", 1)
}

func formatNumber(value int) string {
	digits := fmt.Sprintf("%d", value)
	sign := ""
	if value < 0 {
		sign, digits = "-", digits[1:]
	}
	var groups []string
	for len(digits) > thousandsGroup {
		groups = append([]string{digits[len(digits)-thousandsGroup:]}, groups...)
		digits = digits[:len(digits)-thousandsGroup]
	}
	groups = append([]string{digits}, groups...)
	return sign + strings.Join(groups, noBreakSeparator)
}

func dashIfEmpty(value string) string {
	if value == "" {
		return emptyValue
	}
	return value
}

func verdictLabel(current findings.Findings) string {
	if current.HasProblems() {
		return "Problèmes détectés : " + strings.Join(problemSummary(current), ", ") + "."
	}
	if current.Torrent != nil && current.Torrent.ContentMatching() {
		return "Le contenu est conforme au torrent et aucun problème n'a été détecté."
	}
	return "Aucun problème détecté."
}

func problemSummary(current findings.Findings) []string {
	var parts []string
	if torrent := current.Torrent; torrent != nil {
		parts = appendCount(parts, torrent.BadPieceCount, "pièce invalide", "pièces invalides")
		parts = appendCount(parts, torrent.ZeroPieceCount, "pièce à zéro", "pièces à zéro")
		parts = appendCount(parts, torrent.UnverifiablePieces, "pièce invérifiable", "pièces invérifiables")
		parts = appendCount(parts, len(torrent.Files)-torrent.CountFiles("healthy"), "fichier du torrent non conforme", "fichiers du torrent non conformes")
	}
	var unreadable, malformed, decodeErrors int
	for _, check := range current.Checks {
		if check.PrescanError != nil {
			unreadable++
		}
		if len(check.Anomalies) > 0 {
			malformed++
		}
		if check.FfmpegRan && (check.FfmpegExitCode != 0 || check.FfmpegTotalErrorLines > 0) {
			decodeErrors++
		}
	}
	parts = appendCount(parts, unreadable, "fichier illisible", "fichiers illisibles")
	parts = appendCount(parts, malformed, "fichier avec anomalies de structure", "fichiers avec anomalies de structure")
	return appendCount(parts, decodeErrors, "fichier avec erreurs de décodage", "fichiers avec erreurs de décodage")
}

func appendCount(parts []string, count int, singular, plural string) []string {
	switch {
	case count <= 0:
		return parts
	case count == 1:
		return append(parts, "1 "+singular)
	}
	return append(parts, formatNumber(count)+" "+plural)
}

func readStatus(check findings.FileCheck) string {
	if check.PrescanError != nil {
		return "ÉCHEC : " + *check.PrescanError
	}
	return "Lu sans erreur"
}

func decodeStatus(check findings.FileCheck) string {
	if check.FfmpegExitCode != 0 || check.FfmpegTotalErrorLines > 0 {
		return fmt.Sprintf("ÉCHEC : code de sortie %d, %s lignes d'erreur", check.FfmpegExitCode, formatNumber(check.FfmpegTotalErrorLines))
	}
	return "Décodé sans erreur"
}

func modeLabel(mode string) string {
	if mode == "decode" {
		return "Décodage complet"
	}
	return "Structure"
}

func statusLabel(status string) string {
	labels := map[string]string{
		"healthy":       "Conforme",
		"suspect":       "Suspect",
		"missing":       "Absent",
		"size_mismatch": "Taille différente",
		"unverifiable":  "Invérifiable",
	}
	return labelOrCode(labels, status)
}

func kindLabel(kind string) string {
	return labelOrCode(map[string]string{"zero": "Pièces à zéro", "mismatch": "Pièces invalides"}, kind)
}

func threadingLabel(threading string) string {
	if threading == "mt" {
		return "Multi-thread"
	}
	return "Mono-thread"
}

func matchKindLabel(kind string) string {
	labels := map[string]string{
		"full_path":     "chemin complet",
		"relative_path": "chemin relatif",
		"name_and_size": "nom et taille",
	}
	return labelOrCode(labels, kind)
}

func anomalyLabel(code string) string {
	labels := map[string]string{
		"read_error":        "Erreur de lecture du conteneur",
		"zero_size_packets": "Paquets vides",
		"track_ends_early":  "La piste s'arrête avant la fin annoncée",
		"empty_track":       "Piste sans aucune donnée",
	}
	return dashIfEmpty(labelOrCode(labels, code))
}

func labelOrCode(labels map[string]string, code string) string {
	if label, known := labels[code]; known {
		return label
	}
	return code
}

func matchSummary(match findings.TorrentMatch) string {
	if match.TorrentSize == match.LocalSize {
		return fmt.Sprintf("Correspond à %s (%s)", match.TorrentPath, matchKindLabel(match.MatchKind))
	}
	return fmt.Sprintf("Taille différente pour %s : %s attendus, %s obtenus",
		match.TorrentPath, formatNumber(match.TorrentSize), formatNumber(match.LocalSize))
}

func rangeRows(pieceRange findings.PieceRange) []rangeRow {
	files := pieceRange.Files
	if len(files) == 0 {
		files = []string{emptyValue}
	}
	rows := make([]rangeRow, 0, len(files))
	for fileIndex, file := range files {
		row := rangeRow{File: file}
		if fileIndex == 0 {
			row.Kind = kindLabel(pieceRange.Kind)
			row.Pieces = pieceSpan(pieceRange)
			row.Bytes = formatNumber(pieceRange.StartOffset) + "–" + formatNumber(pieceRange.EndOffset)
		}
		rows = append(rows, row)
	}
	return rows
}

func pieceSpan(pieceRange findings.PieceRange) string {
	pieceTotal := formatNumber(pieceRange.LastPiece - pieceRange.FirstPiece + 1)
	if pieceRange.FirstPiece == pieceRange.LastPiece {
		return fmt.Sprintf("%d (%s)", pieceRange.FirstPiece, pieceTotal)
	}
	return fmt.Sprintf("%d–%d (%s)", pieceRange.FirstPiece, pieceRange.LastPiece, pieceTotal)
}

func repeatPrefix(count int) string {
	if count > 1 {
		return fmt.Sprintf("[×%d] ", count)
	}
	return ""
}

func firstVersionLine(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}
