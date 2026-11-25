package report

import (
	"bytes"
	"html"
	"strings"
	"testing"
	"time"

	"mkvcheck/internal/findings"
)

const hostileFileName = "<script>alert(1)</script>[/code][url=http://example.com/x]a|b\nc"

func sampleReport(fileName string) Report {
	prescanError := "lecture impossible"
	return Report{
		ID:        "abcdefghijklmnopqrstuv",
		CreatedAt: time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC),
		Findings: findings.Findings{
			Mode:           "decode",
			StartedAt:      time.Date(2026, 11, 24, 9, 59, 0, 0, time.UTC),
			ElapsedSeconds: 12,
			Torrent: &findings.TorrentCheck{
				Name:          fileName,
				InfoHash:      "abcdef0123456789",
				PieceLength:   1048576,
				PieceCount:    10,
				TotalSize:     10485760,
				BadPieceCount: 1,
				PieceRanges: []findings.PieceRange{
					{Kind: "mismatch", FirstPiece: 2, LastPiece: 3, StartOffset: 10, EndOffset: 20, Files: []string{fileName}},
				},
				Files: []findings.TorrentFile{
					{Path: fileName, Size: 100, Status: "suspect", LocalPath: fileName},
				},
			},
			Checks: []findings.FileCheck{{
				FileName:     fileName,
				FileSize:     2048,
				PrescanError: &prescanError,
				Tracks:       []findings.Track{{Type: fileName, Number: 1, Codec: fileName, Language: "fra"}},
				Anomalies:    []findings.Anomaly{{Code: "x", Detail: fileName}},
				FfmpegRan:    true, FfmpegExitCode: 1, FfmpegTotalErrorLines: 2, FfmpegThreading: "st",
				FfmpegErrors: []findings.FfmpegError{{Message: fileName + "```", Count: 2}},
				TorrentMatch: &findings.TorrentMatch{TorrentPath: fileName, TorrentSize: 1, LocalSize: 2, MatchKind: "full_path"},
			}},
			UserAgent: fileName,
		},
	}
}

func TestMarkdown_ContainsVerdictDiskImageWarningAndSeal(t *testing.T) {
	valeurObtenue := Markdown(sampleReport("film.mkv"))
	for _, valeurAttendue := range []string{"## Verdict", DiskImageWarning, "Sceau", "Problèmes détectés : 1 pièce invalide"} {
		if !strings.Contains(valeurObtenue, valeurAttendue) {
			t.Fatalf("valeurAttendue %q absente de %q", valeurAttendue, valeurObtenue)
		}
	}
	valeurAttendue := markdownSeal(sampleReport("film.mkv"))
	if !strings.Contains(valeurObtenue, valeurAttendue) {
		t.Fatalf("sceau %q absent", valeurAttendue)
	}
}

func TestSeal_ChangesWhenAnyFindingChanges(t *testing.T) {
	original := sampleReport("film.mkv")
	modified := sampleReport("film.mkv")
	modified.Findings.Checks[0].FfmpegTotalErrorLines = 3
	valeurAttendue := markdownSeal(original)
	valeurObtenue := markdownSeal(modified)
	if valeurAttendue == valeurObtenue {
		t.Fatal("le sceau doit changer quand un constat change")
	}
}

func TestHTML_EscapesFileNameWithScriptTag(t *testing.T) {
	var buffer bytes.Buffer
	if err := HTML(&buffer, sampleReport(hostileFileName)); err != nil {
		t.Fatal(err)
	}
	valeurObtenue := buffer.String()
	if strings.Contains(valeurObtenue, "<script>") {
		t.Fatalf("balise script non neutralisée : %s", valeurObtenue)
	}
	if !strings.Contains(valeurObtenue, "&lt;script&gt;") {
		t.Fatal("nom de fichier échappé attendu")
	}
}

func TestBBCode_NeutralisesClosingTagsInFileNames(t *testing.T) {
	valeurObtenue := BBCode(sampleReport(hostileFileName))
	if strings.Contains(valeurObtenue, "[url=") {
		t.Fatal("[url= ne doit pas survivre")
	}
	valeurAttendue := strings.Count(BBCode(sampleReport("film.mkv")), "[/code]")
	if strings.Count(valeurObtenue, "[/code]") != valeurAttendue {
		t.Fatalf("[/code] injecté : %s", valeurObtenue)
	}
	if !strings.Contains(valeurObtenue, DiskImageWarning) {
		t.Fatal("avertissement images disque attendu")
	}
}

func TestMarkdown_EscapesPipesAndNewlinesInTableCells(t *testing.T) {
	valeurComplete := Markdown(sampleReport(hostileFileName))
	segments := strings.Split(valeurComplete, "```")
	valeurObtenue := strings.Join([]string{segments[0], segments[2]}, "")
	if strings.Contains(valeurObtenue, "a|b") || strings.Contains(valeurObtenue, "<script>") {
		t.Fatalf("pipe ou balise non neutralisés : %s", valeurObtenue)
	}
	if strings.Contains(valeurObtenue, "b\nc") {
		t.Fatal("saut de ligne dans une cellule")
	}
	if len(segments) != 3 {
		t.Fatalf("clôture de bloc de code cassée : %s", valeurComplete)
	}
}

func TestRender_EmptyFindingsStillRenders(t *testing.T) {
	empty := Report{ID: "x", CreatedAt: time.Now(), ExpiresAt: time.Now()}
	var buffer bytes.Buffer
	if err := HTML(&buffer, empty); err != nil {
		t.Fatal(err)
	}
	if Markdown(empty) == "" || BBCode(empty) == "" || buffer.Len() == 0 {
		t.Fatal("rendu vide")
	}
}

func TestHTML_ContainsExpiryDateAndNoIndexMeta(t *testing.T) {
	var buffer bytes.Buffer
	if err := HTML(&buffer, sampleReport("film.mkv")); err != nil {
		t.Fatal(err)
	}
	valeurObtenue := buffer.String()
	for _, valeurAttendue := range []string{"2026-11-26", `<meta name="robots" content="noindex">`, "/static/report.css", "color-scheme", html.EscapeString(DiskImageWarning)} {
		if !strings.Contains(valeurObtenue, valeurAttendue) {
			t.Fatalf("valeurAttendue %q absente", valeurAttendue)
		}
	}
}

func TestAnomalyLabel_TranslatesEveryPrescanCode(t *testing.T) {
	valeursAttendues := map[string]string{
		"read_error":        "Erreur de lecture du conteneur",
		"zero_size_packets": "Paquets vides",
		"track_ends_early":  "La piste s'arrête avant la fin annoncée",
		"empty_track":       "Piste sans aucune donnée",
		"inconnu":           "inconnu",
		"":                  "—",
	}
	for code, valeurAttendue := range valeursAttendues {
		if valeurObtenue := anomalyLabel(code); valeurObtenue != valeurAttendue {
			t.Errorf("%q : valeurAttendue %q, valeurObtenue %q", code, valeurAttendue, valeurObtenue)
		}
	}
}

func TestHTML_LinksToMarkdownAndBBCodeExports(t *testing.T) {
	var buffer bytes.Buffer
	if err := HTML(&buffer, sampleReport("film.mkv")); err != nil {
		t.Fatal(err)
	}
	for _, valeurAttendue := range []string{`href="/r/abcdefghijklmnopqrstuv.md"`, `href="/r/abcdefghijklmnopqrstuv.bb"`} {
		if !strings.Contains(buffer.String(), valeurAttendue) {
			t.Errorf("valeurAttendue %q absente", valeurAttendue)
		}
	}
}

func TestRender_KeepsZeroWidthNonJoinerInFileName(t *testing.T) {
	nomPersan := "می\u200Cخواهم.mkv"
	rapport := sampleReport(nomPersan)
	var sortieHTML bytes.Buffer
	if err := HTML(&sortieHTML, rapport); err != nil {
		t.Fatal(err)
	}
	sorties := map[string]string{
		"html":     sortieHTML.String(),
		"markdown": Markdown(rapport),
		"bbcode":   BBCode(rapport),
	}
	for format, valeurObtenue := range sorties {
		if !strings.Contains(valeurObtenue, nomPersan) {
			t.Errorf("%s : valeurAttendue contenant %q, valeurObtenue absente", format, nomPersan)
		}
	}
}

func TestMediaInfo_RenderedInEveryFormatOnlyWhenPresent(t *testing.T) {
	avecFiche := sampleReport("a.mkv")
	avecFiche.Findings.Checks[0].MediaInfo = "General\nFormat : Matroska [x] ```"
	sansFiche := sampleReport("a.mkv")

	var page bytes.Buffer
	if err := HTML(&page, avecFiche); err != nil {
		t.Fatal(err)
	}
	for format, valeurObtenue := range map[string]string{"html": page.String(), "md": Markdown(avecFiche), "bb": BBCode(avecFiche)} {
		if !strings.Contains(valeurObtenue, "MediaInfo") || !strings.Contains(valeurObtenue, "Format : Matroska") {
			t.Fatalf("%s : fiche absente", format)
		}
	}
	if strings.Contains(Markdown(avecFiche), "```\nGeneral\nFormat : Matroska [x] ```") {
		t.Fatalf("les barrières de code doivent être neutralisées")
	}
	if strings.Contains(Markdown(sansFiche), "MediaInfo") || strings.Contains(BBCode(sansFiche), "MediaInfo") {
		t.Fatalf("un rapport sans fiche ne doit pas changer")
	}
}

func TestHTML_EverySectionHasAnAnchor(t *testing.T) {
	var page bytes.Buffer
	if err := HTML(&page, sampleReport("a.mkv")); err != nil {
		t.Fatal(err)
	}
	for _, ancre := range []string{"verdict", "torrent", "torrent-pieces", "torrent-files", "checks", "file-0", "file-0-prescan", "file-0-ffmpeg", "tools"} {
		if !strings.Contains(page.String(), `id="`+ancre+`"`) || !strings.Contains(page.String(), `href="#`+ancre+`"`) && ancre != "verdict" {
			t.Fatalf("ancre %q absente", ancre)
		}
	}
}

func TestVerdictLabel_NamesEachProblem(t *testing.T) {
	message := "boom"
	current := findings.Findings{Checks: []findings.FileCheck{
		{PrescanError: &message},
		{Anomalies: []findings.Anomaly{{Code: "empty_track"}}},
	}}
	valeurAttendue := "Problèmes détectés : 1 fichier illisible, 1 fichier avec anomalies de structure."
	if valeurObtenue := verdictLabel(current); valeurObtenue != valeurAttendue {
		t.Fatalf("valeurAttendue=%q valeurObtenue=%q", valeurAttendue, valeurObtenue)
	}
}

func TestReadStatus_FlagsFailureExplicitly(t *testing.T) {
	message := "boom"
	if valeurObtenue := readStatus(findings.FileCheck{PrescanError: &message}); valeurObtenue != "ÉCHEC : boom" {
		t.Fatalf("valeurAttendue=%q valeurObtenue=%q", "ÉCHEC : boom", valeurObtenue)
	}
	if valeurObtenue := readStatus(findings.FileCheck{}); valeurObtenue != "Lu sans erreur" {
		t.Fatalf("valeurAttendue=%q valeurObtenue=%q", "Lu sans erreur", valeurObtenue)
	}
}
