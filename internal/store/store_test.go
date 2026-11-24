package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mkvcheck/internal/findings"
)

const (
	testTtl      = 48 * time.Hour
	testMaxBytes = 1 << 20
)

type horlogeFixe struct{ instant time.Time }

func (horloge *horlogeFixe) now() time.Time { return horloge.instant }

func ouvrirStoreDeTest(t *testing.T, maxBytes int64) (*Store, *horlogeFixe, string) {
	t.Helper()
	repertoire := t.TempDir()
	horloge := &horlogeFixe{instant: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	store, erreurOuverture := Open(repertoire, maxBytes, testTtl, horloge.now)
	if erreurOuverture != nil {
		t.Fatalf("ouverture: %v", erreurOuverture)
	}
	return store, horloge, repertoire
}

func TestGet_ReturnsSavedReportWithinTtl(t *testing.T) {
	store, horloge, _ := ouvrirStoreDeTest(t, testMaxBytes)
	sauvegarde, erreurSauvegarde := store.Save(findings.Findings{})
	if erreurSauvegarde != nil {
		t.Fatalf("save: %v", erreurSauvegarde)
	}
	horloge.instant = horloge.instant.Add(testTtl - time.Second)
	rapportObtenu, erreurLecture := store.Get(sauvegarde.ID)
	if erreurLecture != nil {
		t.Fatalf("get: %v", erreurLecture)
	}
	if rapportObtenu.ID != sauvegarde.ID {
		t.Errorf("valeurAttendue %q, valeurObtenue %q", sauvegarde.ID, rapportObtenu.ID)
	}
	valeurAttendue := sauvegarde.CreatedAt.Add(testTtl)
	if !rapportObtenu.ExpiresAt.Equal(valeurAttendue) {
		t.Errorf("valeurAttendue %v, valeurObtenue %v", valeurAttendue, rapportObtenu.ExpiresAt)
	}
}

func TestGet_ReturnsNotFoundAfterTtlEvenBeforePurge(t *testing.T) {
	store, horloge, repertoire := ouvrirStoreDeTest(t, testMaxBytes)
	sauvegarde, _ := store.Save(findings.Findings{})
	horloge.instant = horloge.instant.Add(testTtl)
	_, erreurObtenue := store.Get(sauvegarde.ID)
	if !errors.Is(erreurObtenue, ErrNotFound) {
		t.Errorf("valeurAttendue ErrNotFound, valeurObtenue %v", erreurObtenue)
	}
	if _, erreurStat := os.Stat(filepath.Join(repertoire, sauvegarde.ID+".json")); erreurStat != nil {
		t.Errorf("le fichier doit rester tant que le purge n'a pas tourné: %v", erreurStat)
	}
}

func TestGet_RejectsMalformedIds(t *testing.T) {
	store, _, repertoire := ouvrirStoreDeTest(t, testMaxBytes)
	racineParent := filepath.Dir(repertoire)
	if erreurEcriture := os.WriteFile(filepath.Join(racineParent, "x.json"), []byte("{}"), 0o600); erreurEcriture != nil {
		t.Fatal(erreurEcriture)
	}
	identifiantsInvalides := []string{"../x", "", strings.Repeat("a", 21), strings.Repeat("a", 23), "a/b", strings.Repeat("a", 21) + "/", "..\\" + strings.Repeat("a", 19)}
	for _, identifiant := range identifiantsInvalides {
		_, erreurObtenue := store.Get(identifiant)
		if !errors.Is(erreurObtenue, ErrNotFound) {
			t.Errorf("id %q: valeurAttendue ErrNotFound, valeurObtenue %v", identifiant, erreurObtenue)
		}
	}
}

func TestPurge_RemovesOnlyExpiredFiles(t *testing.T) {
	store, horloge, repertoire := ouvrirStoreDeTest(t, testMaxBytes)
	ancien, _ := store.Save(findings.Findings{})
	horloge.instant = horloge.instant.Add(testTtl / 2)
	recent, _ := store.Save(findings.Findings{})
	horloge.instant = horloge.instant.Add(testTtl/2 + time.Second)

	nombreSupprimes := store.Purge()

	if nombreSupprimes != 1 {
		t.Errorf("valeurAttendue 1, valeurObtenue %d", nombreSupprimes)
	}
	if _, erreurStat := os.Stat(filepath.Join(repertoire, ancien.ID+".json")); !os.IsNotExist(erreurStat) {
		t.Errorf("le rapport expiré doit être supprimé: %v", erreurStat)
	}
	if _, erreurStat := os.Stat(filepath.Join(repertoire, recent.ID+".json")); erreurStat != nil {
		t.Errorf("le rapport valide doit rester: %v", erreurStat)
	}
}

func TestSave_ReturnsErrFullWhenDirectoryExceedsLimit(t *testing.T) {
	store, _, repertoire := ouvrirStoreDeTest(t, 100)
	if erreurEcriture := os.WriteFile(filepath.Join(repertoire, "remplissage.json"), make([]byte, 200), 0o600); erreurEcriture != nil {
		t.Fatal(erreurEcriture)
	}
	_, erreurObtenue := store.Save(findings.Findings{})
	if !errors.Is(erreurObtenue, ErrFull) {
		t.Errorf("valeurAttendue ErrFull, valeurObtenue %v", erreurObtenue)
	}
}

func TestSave_WritesAtomicallyWithoutPartialFiles(t *testing.T) {
	store, _, repertoire := ouvrirStoreDeTest(t, testMaxBytes)
	sauvegarde, _ := store.Save(findings.Findings{})
	entrees, _ := os.ReadDir(repertoire)
	if len(entrees) != 1 || entrees[0].Name() != sauvegarde.ID+".json" {
		t.Fatalf("valeurAttendue un seul fichier %s.json, valeurObtenue %v", sauvegarde.ID, entrees)
	}
	for _, entree := range entrees {
		if strings.HasSuffix(entree.Name(), ".tmp") {
			t.Errorf("fichier temporaire restant: %s", entree.Name())
		}
	}
}

func TestSave_GeneratesDistinctTwentyTwoCharIds(t *testing.T) {
	store, _, _ := ouvrirStoreDeTest(t, testMaxBytes)
	identifiantsVus := map[string]bool{}
	for indexEssai := 0; indexEssai < 50; indexEssai++ {
		sauvegarde, erreurSauvegarde := store.Save(findings.Findings{})
		if erreurSauvegarde != nil {
			t.Fatal(erreurSauvegarde)
		}
		if len(sauvegarde.ID) != 22 {
			t.Fatalf("valeurAttendue 22, valeurObtenue %d", len(sauvegarde.ID))
		}
		if identifiantsVus[sauvegarde.ID] {
			t.Fatalf("id dupliqué %s", sauvegarde.ID)
		}
		identifiantsVus[sauvegarde.ID] = true
	}
}
