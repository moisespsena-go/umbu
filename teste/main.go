package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/moisespsena-go/tracederror"

	"github.com/moisespsena/template/text/template"
)


func main() {
	var c = map[string]string{"A":"a","B":"b"}

	render := func() string {
		t, err := template.New("-").Parse(`{{range &$el := .}}{{$el}}{{end}}`)
		if err != nil {
			panic(err)
		}
		e := t.CreateExecutor()
		r, err := e.ExecuteString(c)
		if err != nil {
			panic(err)
		}
		return r
	}

	defer func() {
		if r := recover(); r != nil {
			if t, ok := r.(tracederror.TracedError); ok {
				os.Stderr.WriteString(t.Error() + "\n")
				os.Stderr.Write(t.Trace())
			} else {
				os.Stderr.WriteString(fmt.Sprint(r))
				os.Stderr.Write(debug.Stack())
			}
		}
	}()

	fmt.Println(render())
}
