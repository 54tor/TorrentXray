package report

import (
	_ "embed"
	"html/template"
	"io"
)

//go:embed templates/report.html.tmpl
var htmlSource string

var htmlTemplate = template.Must(template.New("html").Funcs(template.FuncMap(sharedFuncs())).Parse(htmlSource))

func HTML(writer io.Writer, report Report) error {
	return htmlTemplate.Execute(writer, newSealedView(report))
}
