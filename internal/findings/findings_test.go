package findings

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func parseOrFail(t *testing.T, raw string) Findings {
	t.Helper()
	valeurObtenue, erreurObtenue := Parse([]byte(raw))
	if erreurObtenue != nil {
		t.Fatalf("erreur inattendue : %v", erreurObtenue)
	}
	return valeurObtenue
}

func TestParse_EmptyObjectGivesValidFindingsWithoutChecks(t *testing.T) {
	valeurObtenue := parseOrFail(t, `{}`)
	if len(valeurObtenue.Checks) != 0 || valeurObtenue.Torrent != nil || valeurObtenue.Mode != "structure" {
		t.Fatalf("obtenu %+v", valeurObtenue)
	}
	if valeurObtenue.StartedAt.IsZero() {
		t.Fatal("StartedAt devrait avoir une valeur par défaut")
	}
}

func TestParse_RejectsBodyOverLimit(t *testing.T) {
	corps := `{"userAgent":"` + strings.Repeat("a", MaxBodyBytes) + `"}`
	_, erreurObtenue := Parse([]byte(corps[:MaxBodyBytes+1]))
	if !errors.Is(erreurObtenue, ErrTooLarge) {
		t.Fatalf("attendu ErrTooLarge, obtenu %v", erreurObtenue)
	}
	_, erreurObtenue = Parse([]byte(corps[:MaxBodyBytes]))
	if errors.Is(erreurObtenue, ErrTooLarge) {
		t.Fatal("la limite exacte doit être acceptée")
	}
}

func nested(depth int) string {
	return strings.Repeat(`{"a":`, depth-1) + `{}` + strings.Repeat(`}`, depth-1)
}

func TestParse_RejectsNestingAboveThirtyTwo(t *testing.T) {
	if _, erreurObtenue := Parse([]byte(nested(33))); !errors.Is(erreurObtenue, ErrInvalidJSON) {
		t.Fatalf("attendu ErrInvalidJSON à 33, obtenu %v", erreurObtenue)
	}
	if _, erreurObtenue := Parse([]byte(nested(32))); erreurObtenue != nil {
		t.Fatalf("32 niveaux doivent passer : %v", erreurObtenue)
	}
}

func TestParse_RejectsMalformedAndNonObjectJSON(t *testing.T) {
	for _, corps := range []string{``, `{`, `[]`, `"x"`, `null`} {
		if _, erreurObtenue := Parse([]byte(corps)); !errors.Is(erreurObtenue, ErrInvalidJSON) {
			t.Errorf("%q : attendu ErrInvalidJSON, obtenu %v", corps, erreurObtenue)
		}
	}
}

func TestParse_CoercesWrongTypesToZeroValues(t *testing.T) {
	valeurObtenue := parseOrFail(t, `{"elapsedSeconds":"abc","checks":"nope","torrent":[1],"versions":5,"userAgent":{"x":1},
		"startedAt":12,"mode":["decode"]}`)
	if valeurObtenue.ElapsedSeconds != 0 || len(valeurObtenue.Checks) != 0 || valeurObtenue.Torrent != nil ||
		valeurObtenue.Versions != (Versions{}) || valeurObtenue.UserAgent != "" || valeurObtenue.Mode != "structure" {
		t.Fatalf("obtenu %+v", valeurObtenue)
	}
	fichier := parseOrFail(t, `{"checks":[{"file":{"name":7,"size":"12"},"prescan":{"durationSeconds":"x","tracks":{"a":1}},"ffmpeg":{"exitCode":"3"}}]}`).Checks[0]
	if fichier.FileName != "7" || fichier.FileSize != 12 || fichier.DurationSeconds != 0 || len(fichier.Tracks) != 0 || fichier.FfmpegExitCode != 3 {
		t.Fatalf("obtenu %+v", fichier)
	}
}

func TestParse_ClampsNegativeNumbersToZero(t *testing.T) {
	fichier := parseOrFail(t, `{"elapsedSeconds":-4,"checks":[{"file":{"size":-1},"prescan":{"durationSeconds":-2.5}}]}`)
	if fichier.ElapsedSeconds != 0 || fichier.Checks[0].FileSize != 0 || fichier.Checks[0].DurationSeconds != 0 {
		t.Fatalf("obtenu %+v", fichier)
	}
}

func TestParse_CutsTextAtThreeHundredCharsAndStripsControlCharacters(t *testing.T) {
	nom := strings.Repeat("é", 1000) + "\n\x00evil"
	corps := fmt.Sprintf(`{"checks":[{"file":{"name":%q}},{"file":{"name":"a\nb\u0000c"}}]}`, nom)
	valeurObtenue := parseOrFail(t, strings.ReplaceAll(corps, `\x00`, `\u0000`))
	if longueur := len([]rune(valeurObtenue.Checks[0].FileName)); longueur != 300 {
		t.Fatalf("attendu 300 caractères, obtenu %d", longueur)
	}
	if valeurObtenue.Checks[1].FileName != "a b c" {
		t.Fatalf("obtenu %q", valeurObtenue.Checks[1].FileName)
	}
}

func TestParse_CapsListsAtSpecLimits(t *testing.T) {
	checks := strings.TrimSuffix(strings.Repeat(`{},`, 51), ",")
	files := strings.TrimSuffix(strings.Repeat(`{"path":"a"},`, 1001), ",")
	tracks := strings.TrimSuffix(strings.Repeat(`{},`, 65), ",")
	anomalies := strings.TrimSuffix(strings.Repeat(`{},`, 101), ",")
	errorsList := strings.TrimSuffix(strings.Repeat(`["m",1],`, 501), ",")
	versionLines := strings.TrimSuffix(strings.Repeat(`"v",`, 41), ",")
	ranges := strings.TrimSuffix(strings.Repeat(`{"files":["a","a","a","a","a","a","a","a","a","a","a","a","a","a","a","a","a","a","a","a","a"]},`, 201), ",")
	corps := fmt.Sprintf(`{"checks":[%s,{"prescan":{"tracks":[%s],"anomalies":[%s]},"ffmpeg":{"errors":[%s],"versionLines":[%s]}}],
		"torrent":{"files":[%s],"pieceRanges":[%s]}}`, checks, tracks, anomalies, errorsList, versionLines, files, ranges)
	valeurObtenue := parseOrFail(t, corps)
	if len(valeurObtenue.Checks) != 50 || len(valeurObtenue.Torrent.Files) != 1000 || len(valeurObtenue.Torrent.PieceRanges) != 200 ||
		len(valeurObtenue.Torrent.PieceRanges[0].Files) != 20 {
		t.Fatalf("plafonds torrent/contrôles : %d %d %d", len(valeurObtenue.Checks), len(valeurObtenue.Torrent.Files), len(valeurObtenue.Torrent.PieceRanges))
	}
	corps = fmt.Sprintf(`{"checks":[{"prescan":{"tracks":[%s],"anomalies":[%s]},"ffmpeg":{"errors":[%s],"versionLines":[%s]}}]}`, tracks, anomalies, errorsList, versionLines)
	fichier := parseOrFail(t, corps).Checks[0]
	if len(fichier.Tracks) != 64 || len(fichier.Anomalies) != 100 || len(fichier.FfmpegErrors) != 500 || len(fichier.FfmpegVersionLines) != 40 {
		t.Fatalf("plafonds fichier : %d %d %d %d", len(fichier.Tracks), len(fichier.Anomalies), len(fichier.FfmpegErrors), len(fichier.FfmpegVersionLines))
	}
}

func TestParse_UnknownModeFallsBackToStructure(t *testing.T) {
	if valeurObtenue := parseOrFail(t, `{"mode":"rm -rf"}`).Mode; valeurObtenue != "structure" {
		t.Fatalf("obtenu %q", valeurObtenue)
	}
	if valeurObtenue := parseOrFail(t, `{"mode":"decode"}`).Mode; valeurObtenue != "decode" {
		t.Fatalf("obtenu %q", valeurObtenue)
	}
}

func TestParse_ClosedEnumerationsFallBackToDefaults(t *testing.T) {
	valeurObtenue := parseOrFail(t, `{"checks":[{"torrentMatch":{"matchKind":"x"},"ffmpeg":{"exitCode":0,"threading":"zz"}},{"ffmpeg":{"threading":"mt"}}],
		"torrent":{"pieceRanges":[{"kind":"x","firstPiece":5,"lastPiece":2,"startOffset":9,"endOffset":1}],"files":[{"status":"x"}]}}`)
	if valeurObtenue.Checks[0].TorrentMatch.MatchKind != "name_and_size" || valeurObtenue.Checks[0].FfmpegThreading != "st" || valeurObtenue.Checks[1].FfmpegThreading != "mt" {
		t.Fatalf("obtenu %+v", valeurObtenue.Checks)
	}
	plage := valeurObtenue.Torrent.PieceRanges[0]
	if plage.Kind != "mismatch" || plage.LastPiece != 5 || plage.EndOffset != 9 || valeurObtenue.Torrent.Files[0].Status != "unverifiable" {
		t.Fatalf("obtenu %+v", valeurObtenue.Torrent)
	}
}

func TestParse_ReadsFullFileCheckAndNormalisesInfoHash(t *testing.T) {
	valeurObtenue := parseOrFail(t, `{"startedAt":"2026-01-02T03:04:05Z","torrent":{"infoHash":"AB-cd ZZ12"},
		"checks":[{"file":{"name":"a.mkv","size":9},"prescan":{"containerName":"Matroska","readError":"boom","tracks":[{"type":"video","number":1}]},
		"ffmpeg":{"exitCode":1,"totalErrorLines":3,"errors":[["m",2],["n"]]}}]}`)
	fichier := valeurObtenue.Checks[0]
	if valeurObtenue.Torrent.InfoHash != "abcd12" || valeurObtenue.StartedAt.Year() != 2026 ||
		*fichier.ContainerName != "Matroska" || *fichier.PrescanError != "boom" || !fichier.FfmpegRan ||
		fichier.FfmpegErrors[0].Count != 2 || fichier.FfmpegErrors[1].Count != 1 || fichier.Tracks[0].Type != "video" {
		t.Fatalf("obtenu %+v", valeurObtenue)
	}
}

func TestFileCheckHasProblems_FalseWhenCleanAndFfmpegNotRun(t *testing.T) {
	if (FileCheck{FfmpegExitCode: 1, FfmpegTotalErrorLines: 4}).HasProblems() {
		t.Fatal("ffmpeg non exécuté : pas de problème")
	}
}

func TestFileCheckHasProblems_FalseWhenFfmpegRanCleanly(t *testing.T) {
	if (FileCheck{FfmpegRan: true}).HasProblems() {
		t.Fatal("ffmpeg propre : pas de problème")
	}
}

func TestFileCheckHasProblems_TrueOnPrescanError(t *testing.T) {
	message := ""
	if !(FileCheck{PrescanError: &message}).HasProblems() {
		t.Fatal("erreur de pré-analyse, même vide, est un problème")
	}
}

func TestFileCheckHasProblems_TrueOnAnomaly(t *testing.T) {
	if !(FileCheck{Anomalies: []Anomaly{{Code: "x"}}}).HasProblems() {
		t.Fatal("anomalie attendue comme problème")
	}
}

func TestFileCheckHasProblems_TrueOnFfmpegNonZeroExit(t *testing.T) {
	if !(FileCheck{FfmpegRan: true, FfmpegExitCode: 1}).HasProblems() {
		t.Fatal("code de sortie non nul attendu comme problème")
	}
}

func TestFileCheckHasProblems_TrueOnFfmpegErrorLines(t *testing.T) {
	if !(FileCheck{FfmpegRan: true, FfmpegTotalErrorLines: 1}).HasProblems() {
		t.Fatal("lignes d'erreur attendues comme problème")
	}
}

func healthyFile() TorrentFile { return TorrentFile{Status: "healthy"} }

func TestTorrentCheckContentMatching_TrueWhenAllHealthy(t *testing.T) {
	if !(TorrentCheck{Files: []TorrentFile{healthyFile()}}).ContentMatching() {
		t.Fatal("torrent conforme attendu")
	}
}

func TestTorrentCheckContentMatching_FalseOnBadPieces(t *testing.T) {
	if (TorrentCheck{BadPieceCount: 1, Files: []TorrentFile{healthyFile()}}).ContentMatching() {
		t.Fatal("pièce invalide : non conforme")
	}
}

func TestTorrentCheckContentMatching_FalseOnZeroPieces(t *testing.T) {
	if (TorrentCheck{ZeroPieceCount: 1, Files: []TorrentFile{healthyFile()}}).ContentMatching() {
		t.Fatal("pièce à zéro : non conforme")
	}
}

func TestTorrentCheckContentMatching_FalseOnUnverifiablePieces(t *testing.T) {
	if (TorrentCheck{UnverifiablePieces: 1, Files: []TorrentFile{healthyFile()}}).ContentMatching() {
		t.Fatal("pièce invérifiable : non conforme")
	}
}

func TestTorrentCheckContentMatching_FalseWhenAFileIsNotHealthy(t *testing.T) {
	if (TorrentCheck{Files: []TorrentFile{healthyFile(), {Status: "missing"}}}).ContentMatching() {
		t.Fatal("fichier manquant : non conforme")
	}
}

func TestTorrentCheckCountFiles_CountsByStatus(t *testing.T) {
	torrent := TorrentCheck{Files: []TorrentFile{healthyFile(), {Status: "missing"}, healthyFile()}}
	if torrent.CountFiles("healthy") != 2 || torrent.CountFiles("missing") != 1 || torrent.CountFiles("suspect") != 0 {
		t.Fatal("comptage incorrect")
	}
}

func TestFindingsHasProblems_TorrentMismatchWithHealthyChecks(t *testing.T) {
	valeurObtenue := Findings{
		Torrent: &TorrentCheck{BadPieceCount: 3, Files: []TorrentFile{{Status: "suspect"}}},
		Checks:  []FileCheck{{FfmpegRan: true}},
	}
	if !valeurObtenue.HasProblems() {
		t.Fatal("torrent non conforme = problèmes")
	}
}

func TestFindingsHasProblems_FalseWhenEverythingIsClean(t *testing.T) {
	valeurObtenue := Findings{Torrent: &TorrentCheck{Files: []TorrentFile{healthyFile()}}, Checks: []FileCheck{{FfmpegRan: true}}}
	if valeurObtenue.HasProblems() {
		t.Fatal("aucun problème attendu")
	}
}

func TestFindingsHasProblems_TrueWhenOneCheckHasProblems(t *testing.T) {
	valeurObtenue := Findings{Checks: []FileCheck{{}, {Anomalies: []Anomaly{{Code: "x"}}}}}
	if !valeurObtenue.HasProblems() {
		t.Fatal("un contrôle avec problème suffit")
	}
}
