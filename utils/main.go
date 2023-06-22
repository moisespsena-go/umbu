package main

import "github.com/moisespsena-go/umbu/text/template"

func main() {
	t := template.New("teste")
	t, err := t.Parse("{{contains . \"X\"}}")
	if err != nil {
		panic(err)
	}
	v, err := t.ExecuteString(map[string]string{"X": "Value"})
	if err != nil {
		println(err)
	} else {
		println(v)
	}
}
