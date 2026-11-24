package web

import (
	"io/fs"
	"regexp"
	"testing"
)

func TestIndex_ReferencesOnlyLocalAssets(t *testing.T) {
	static, err := Static()
	if err != nil {
		t.Fatal(err)
	}
	page, err := fs.ReadFile(static, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	interdits := map[string]*regexp.Regexp{
		"URL distante":        regexp.MustCompile(`(?i)https?://`),
		"script en ligne":     regexp.MustCompile(`(?is)<script\b[^>]*>\s*[^<\s]`),
		"attribut style":      regexp.MustCompile(`(?i)\sstyle\s*=`),
		"bloc style":          regexp.MustCompile(`(?i)<style\b`),
		"gestionnaire inline": regexp.MustCompile(`(?i)\son[a-z]+\s*=`),
	}
	for nature, motif := range interdits {
		valeurAttendue := false
		valeurObtenue := motif.Match(page)
		if valeurObtenue != valeurAttendue {
			t.Errorf("index.html contient un %s : %q", nature, motif.FindString(string(page)))
		}
	}
}

func TestStatic_EmbedsReportStylesheet(t *testing.T) {
	static, err := Static()
	if err != nil {
		t.Fatal(err)
	}
	_, err = fs.Stat(static, "report.css")
	valeurAttendue := true
	valeurObtenue := err == nil
	if valeurObtenue != valeurAttendue {
		t.Errorf("report.css absent de l'embed : %v", err)
	}
}
