package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"mkvcheck/internal/findings"
	"mkvcheck/internal/report"
)

const (
	identifiantOctets = 16
	extensionRapport  = ".json"
	prefixTemporaire  = "tmp-"
	suffixTemporaire  = ".tmp"
	permissionDossier = 0o700
	permissionFichier = 0o600
)

var (
	ErrNotFound = errors.New("rapport introuvable")
	ErrFull     = errors.New("stockage plein")

	motifIdentifiant = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)
)

type Store struct {
	dir      string
	maxBytes int64
	ttl      time.Duration
	now      func() time.Time
}

func Open(dir string, maxBytes int64, ttl time.Duration, now func() time.Time) (*Store, error) {
	if erreurCreation := os.MkdirAll(dir, permissionDossier); erreurCreation != nil {
		return nil, erreurCreation
	}
	return &Store{dir: dir, maxBytes: maxBytes, ttl: ttl, now: now}, nil
}

func (s *Store) Save(f findings.Findings) (report.Report, error) {
	tailleCourante, erreurTaille := s.tailleRepertoire()
	if erreurTaille != nil {
		return report.Report{}, erreurTaille
	}
	if tailleCourante > s.maxBytes {
		return report.Report{}, ErrFull
	}
	identifiant, erreurIdentifiant := nouvelIdentifiant()
	if erreurIdentifiant != nil {
		return report.Report{}, erreurIdentifiant
	}
	creation := s.now().UTC()
	rapport := report.Report{ID: identifiant, CreatedAt: creation, ExpiresAt: creation.Add(s.ttl), Findings: f}
	return rapport, s.ecrireAtomiquement(rapport)
}

func (s *Store) Get(id string) (report.Report, error) {
	if !motifIdentifiant.MatchString(id) {
		return report.Report{}, ErrNotFound
	}
	rapport, erreurLecture := lireRapport(filepath.Join(s.dir, id+extensionRapport))
	if erreurLecture != nil {
		return report.Report{}, ErrNotFound
	}
	if s.estExpire(rapport) {
		return report.Report{}, ErrNotFound
	}
	return rapport, nil
}

func (s *Store) Purge() int {
	chemins, _ := filepath.Glob(filepath.Join(s.dir, "*"+extensionRapport))
	nombreSupprimes := 0
	for _, chemin := range chemins {
		rapport, erreurLecture := lireRapport(chemin)
		if erreurLecture != nil || !s.estExpire(rapport) {
			continue
		}
		if os.Remove(chemin) == nil {
			nombreSupprimes++
		}
	}
	return nombreSupprimes
}

func (s *Store) RunPurger(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Purge()
		}
	}
}

func (s *Store) estExpire(rapport report.Report) bool {
	return !s.now().Before(rapport.ExpiresAt)
}

func (s *Store) tailleRepertoire() (int64, error) {
	entrees, erreurLecture := os.ReadDir(s.dir)
	if erreurLecture != nil {
		return 0, erreurLecture
	}
	var total int64
	for _, entree := range entrees {
		informations, erreurInformations := entree.Info()
		if erreurInformations != nil {
			continue
		}
		total += informations.Size()
	}
	return total, nil
}

func (s *Store) ecrireAtomiquement(rapport report.Report) error {
	contenu, erreurEncodage := json.Marshal(rapport)
	if erreurEncodage != nil {
		return erreurEncodage
	}
	temporaire, erreurCreation := os.CreateTemp(s.dir, prefixTemporaire+"*"+suffixTemporaire)
	if erreurCreation != nil {
		return erreurCreation
	}
	if erreurEcriture := ecrireEtFermer(temporaire, contenu); erreurEcriture != nil {
		os.Remove(temporaire.Name())
		return erreurEcriture
	}
	if erreurRenommage := os.Rename(temporaire.Name(), filepath.Join(s.dir, rapport.ID+extensionRapport)); erreurRenommage != nil {
		os.Remove(temporaire.Name())
		return erreurRenommage
	}
	return nil
}

func ecrireEtFermer(fichier *os.File, contenu []byte) error {
	if _, erreurEcriture := fichier.Write(contenu); erreurEcriture != nil {
		fichier.Close()
		return erreurEcriture
	}
	if erreurPermission := fichier.Chmod(permissionFichier); erreurPermission != nil {
		fichier.Close()
		return erreurPermission
	}
	return fichier.Close()
}

func lireRapport(chemin string) (report.Report, error) {
	contenu, erreurLecture := os.ReadFile(chemin)
	if erreurLecture != nil {
		return report.Report{}, erreurLecture
	}
	var rapport report.Report
	if erreurDecodage := json.Unmarshal(contenu, &rapport); erreurDecodage != nil {
		return report.Report{}, erreurDecodage
	}
	return rapport, nil
}

func nouvelIdentifiant() (string, error) {
	octets := make([]byte, identifiantOctets)
	if _, erreurAleatoire := rand.Read(octets); erreurAleatoire != nil {
		return "", erreurAleatoire
	}
	return base64.RawURLEncoding.EncodeToString(octets), nil
}
