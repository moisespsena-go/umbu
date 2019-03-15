package main

import (
	"github.com/moisespsena/template/text/template"
	"time"
)

type z struct {
}

func (z) IsZero() bool {
	return false
}

func main() {
	t, err := template.New("teste").Parse(`{{timef . "YYYY-MM-dd"}}`)
	if err != nil {
		panic(err)
	}

	var d interface{}
	d = time.Now()
	r, err := t.CreateExecutor().ExecuteString(d)
	if err != nil {
		panic(err)
	}
	println(r)
}
