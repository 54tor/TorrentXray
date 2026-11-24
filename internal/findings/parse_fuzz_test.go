package findings

import (
	"encoding/json"
	"testing"
	"unicode/utf8"
)

func FuzzParse(f *testing.F) {
	graines := []string{
		`{}`,
		`{"mode":"decode","elapsedSeconds":12,"userAgent":"Mozilla/5.0","checks":[{"name":"a.mkv"}],"versions":{"ffmpegWasm":"0.12"}}`,
		`{"checks":"x","torrent":[],"versions":7,"mode":{"a":1}}`,
		"{\"userAgent\":\"a\\u0000b\\u202ec\\u200b\"}",
		`{"elapsedSeconds":1e999,"startedAt":"not a date"}`,
		`{"checks":[null,1,"x",[],{"tracks":[{}],"anomalies":[[1,2]]}]}`,
		`{} garbage`,
		`[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[`,
	}
	for _, graine := range graines {
		f.Add([]byte(graine))
	}
	f.Fuzz(func(t *testing.T, corps []byte) {
		resultat, erreurObtenue := Parse(corps)
		if erreurObtenue != nil {
			return
		}
		verifierInvariants(t, resultat)
	})
}

func verifierInvariants(t *testing.T, resultat Findings) {
	t.Helper()
	if len(resultat.Checks) > maxChecks {
		t.Fatalf("checks : %d > %d", len(resultat.Checks), maxChecks)
	}
	if resultat.Torrent != nil && len(resultat.Torrent.Files) > maxTorrentFiles {
		t.Fatalf("fichiers du torrent : %d > %d", len(resultat.Torrent.Files), maxTorrentFiles)
	}
	if resultat.Mode != "structure" && resultat.Mode != "decode" {
		t.Fatalf("mode inattendu %q", resultat.Mode)
	}
	if resultat.ElapsedSeconds < 0 {
		t.Fatalf("durée négative %d", resultat.ElapsedSeconds)
	}
	serialise, erreurSerialisation := json.Marshal(resultat)
	if erreurSerialisation != nil {
		t.Fatalf("sérialisation : %v", erreurSerialisation)
	}
	var generique any
	if erreurDecodage := json.Unmarshal(serialise, &generique); erreurDecodage != nil {
		t.Fatalf("relecture : %v", erreurDecodage)
	}
	verifierTextes(t, generique)
}

func verifierTextes(t *testing.T, valeur any) {
	t.Helper()
	switch typee := valeur.(type) {
	case string:
		if utf8.RuneCountInString(typee) > maxTextRunes {
			t.Fatalf("texte de %d runes", utf8.RuneCountInString(typee))
		}
		if controlCharacters.MatchString(typee) {
			t.Fatalf("caractère de contrôle dans %q", typee)
		}
	case []any:
		for _, element := range typee {
			verifierTextes(t, element)
		}
	case map[string]any:
		for _, element := range typee {
			verifierTextes(t, element)
		}
	}
}
