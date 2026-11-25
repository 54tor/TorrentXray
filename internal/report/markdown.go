package report

import (
	_ "embed"
	"strings"
	"text/template"
)

//go:embed templates/report.md.tmpl
var markdownSource string

var markdownTemplate = template.Must(template.New("markdown").Funcs(sharedFuncs()).Funcs(template.FuncMap{
	"esc":    escapeMarkdown,
	"code":   escapeMarkdownCode,
	"fenced": neutraliseFences,
}).Parse(markdownSource))

var (
	markdownLineBreaks = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", " ", " ", " ", " ")
	markdownSpecials   = strings.NewReplacer(
		"\\", "\\\\", "|", "\\|", "`", "\\`", "*", "\\*", "_", "\\_",
		"[", "\\[", "]", "\\]", "#", "\\#", "!", "\\!", "<", "&lt;", ">", "&gt;", "&", "&amp;",
	)
	markdownCodeFence = strings.NewReplacer("`", "'")
)

func escapeMarkdown(value string) string {
	return markdownSpecials.Replace(markdownLineBreaks.Replace(value))
}

func neutraliseFences(value string) string {
	return markdownCodeFence.Replace(value)
}

func escapeMarkdownCode(value string) string {
	return markdownCodeFence.Replace(markdownLineBreaks.Replace(value))
}

func markdownBody(report Report) string {
	var output strings.Builder
	if err := markdownTemplate.Execute(&output, newView(report)); err != nil {
		panic(err)
	}
	return strings.TrimSpace(output.String())
}

func markdownSeal(report Report) string {
	return Seal(markdownBody(report))
}

func Markdown(report Report) string {
	return markdownBody(report) + "\n\n---\n\nSceau SHA-256 du contenu ci-dessus : `" + markdownSeal(report) + "`\n"
}
