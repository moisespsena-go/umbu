package main

import "github.com/moisespsena/template/text/template"

func main() {
	t := template.New("teste")
	t, err := t.Parse("{{contains . \"X\"}}")
	if err != nil {
		panic(err)
	}
	v, err := t.ExecuteString(map[string]string {"C": "Value"})
	if err != nil {
		println(err)
	} else {
		println(v)
	}
}
