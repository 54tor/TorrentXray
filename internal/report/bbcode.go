package report

import (
	_ "embed"
	"strings"
	"text/template"
)

//go:embed templates/report.bbcode.tmpl
var bbcodeSource string

var bbcodeTemplate = template.Must(template.New("bbcode").Funcs(sharedFuncs()).Funcs(template.FuncMap{
	"esc":    escapeBBCode,
	"fenced": neutraliseBBCodeBlock,
}).Parse(bbcodeSource))

var bbcodeNeutraliser = strings.NewReplacer(
	"\r\n", " ", "\r", " ", "\n", " ", " ", " ", " ", " ",
	"[", "［", "]", "］",
)

var bbcodeBracketsOnly = strings.NewReplacer("[", "［", "]", "］")

func neutraliseBBCodeBlock(value string) string {
	return bbcodeBracketsOnly.Replace(value)
}

func escapeBBCode(value string) string {
	return bbcodeNeutraliser.Replace(value)
}

func BBCode(report Report) string {
	var output strings.Builder
	if err := bbcodeTemplate.Execute(&output, newSealedView(report)); err != nil {
		panic(err)
	}
	return strings.TrimSpace(output.String()) + "\n"
}
