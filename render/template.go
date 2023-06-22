package render

import (
	"context"
	"io"

	"github.com/moisespsena-go/umbu/html/template"
)

// Template template struct
type Template struct {
	DefaultLayout      string
	UsingDefaultLayout bool
	GetExecutor        func(name string) (excr *template.Executor, err error)
	Layout             string
	Funcs              template.FuncMapSlice
	FuncValues         template.FuncValuesSlice
}

func (this Template) SetLayout(layout string) *Template {
	this.Layout = layout
	return &this
}

func (this Template) SetFuncValues(fv ...template.FuncValues) *Template {
	this.FuncValues.Append(fv...)
	return &this
}

func (this Template) SetFuncs(fv ...template.FuncMap) *Template {
	this.Funcs.Append(fv...)
	return &this
}

// Render render tmpl
func (this *Template) Render(state *template.State, w io.Writer, ctx context.Context, templateName string, obj interface{}, lang ...string) error {
	r := NewTemplateRender(this, obj, lang...)
	return r.RenderC(state, w, ctx, templateName)
}
