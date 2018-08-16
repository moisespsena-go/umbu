package main

import "github.com/moisespsena/template/text/template"

func main() {
	t, err := template.New("teste").Parse(`
{{define "html"}}
{{if .}}
OK {{x}}: {{.}}
{{end}}
{{end}}
{{$v := (trim (template_exec "html" .))}}
||{{$v}}||
`)
	if err != nil {
		panic(err)
	}
	r, err := t.CreateExecutor().ExecuteString(100, template.FuncMap{"x": func() string {
		return "XXX!!"
	}})
	if err != nil {
		panic(err)
	}
	println(r)
}
