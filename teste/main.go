package main

import (
	"fmt"
	"os"

	"github.com/moisespsena-go/umbu/funcs"
	"github.com/moisespsena-go/umbu/text/template"
)

func main() {
	t, err := template.New("teste").Parse(`{{define "mixin"}}{{fn}}{{end}}{{fn}}{{template "mixin"}}{{fn}}`)
	fmt.Println(err)
	t.Funcs(funcs.FuncMap{"fn": func() string {
		return "a"
	}})
	t.Template("mixin").Funcs(funcs.FuncMap{"fn": func() string {
		return "b"
	}})
	fmt.Println(t.CreateExecutor().Execute(os.Stdout, nil))
}
