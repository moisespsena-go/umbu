package render

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/moisespsena-go/umbu/html/template"
)

type TemplateRender struct {
	template   *Template
	funcValues template.FuncValues
	obj        interface{}
	lang       []string
}

func NewTemplateRender(tmpl *Template, obj interface{}, lang ...string) (r *TemplateRender) {
	r = &TemplateRender{template: tmpl, obj: obj, lang: lang}
	r.funcValues.AppendValues(tmpl.FuncValues...)
	// set default funcMaps
	r.funcValues.SetDefault("render", r.Require)
	r.funcValues.SetDefault("require", r.Require)
	r.funcValues.SetDefault("include", r.Include)
	return
}

func (this *TemplateRender) Render(state *template.State, w io.Writer, ctx context.Context, name string, require bool, objs ...interface{}) (err error) {
	var renderObj = this.obj

	for i, obj_ := range objs {
		if obj_ != nil {
			renderObj, objs = obj_, objs[i:]
			break
		}
	}

	var exectr *template.Executor

	if len(this.lang) == 0 {
		exectr, err = this.template.GetExecutor(name)
	} else {
		if extPos := strings.LastIndexByte(name, '.'); extPos > 0 {
			for _, lang := range this.lang {
				if lang == "_" {
					lang = "default"
				}
				name2 := path.Join(name[0:extPos], lang+name[extPos:])
				if exectr, err = this.template.GetExecutor(name2); err == nil {
					break
				}
			}
		} else {
			exectr, err = this.template.GetExecutor(name)
		}
	}

	if err == nil {
		exectr.SetSuper(state)
		exectr = exectr.FuncsValues(this.funcValues)
		if len(objs) > 0 {
			for i, max := 0, len(objs); i < max; i++ {
				switch ot := objs[i].(type) {
				case template.LocalData:
					if i == 0 {
						exectr.Local = ot
					} else {
						exectr.Local.Merge(ot)
					}
				case map[interface{}]interface{}:
					exectr.Local.Merge(ot)
				default:
					exectr.Local.Set(objs[i], objs[i+1])
					i++
				}
			}
		}
		exectr.Context = ctx
		err = exectr.Execute(w, renderObj)
	}

	return err
}

func (this *TemplateRender) renderC(state *template.State, ctx context.Context, name string, require bool, objs ...interface{}) (s template.HTML, err error) {
	var w bytes.Buffer
	if err = this.Render(state, &w, ctx, name, require, objs...); err != nil {
		return
	}
	return template.HTML(w.String()), nil
}

func (this *TemplateRender) RequireC(state *template.State, w io.Writer, ctx context.Context, name string, objs ...interface{}) (err error) {
	return this.Render(state, w, ctx, name, true, objs...)
}

func (this *TemplateRender) Require(state *template.State, name string, objs ...interface{}) (s template.HTML, err error) {
	var w bytes.Buffer
	if err = this.RequireC(state, &w, state.Context(), name, objs...); err != nil {
		return
	}
	return template.HTML(w.String()), nil
}

func (this *TemplateRender) IncludeC(state *template.State, w io.Writer, ctx context.Context, name string, objs ...interface{}) error {
	return this.Render(state, w, ctx, name, false, objs...)
}

func (this *TemplateRender) Include(state *template.State, name string, objs ...interface{}) (s template.HTML, err error) {
	var w bytes.Buffer
	if err = this.IncludeC(state, &w, state.Context(), name, objs...); err != nil {
		return
	}
	return template.HTML(w.String()), nil
}

func (this *TemplateRender) RenderC(state *template.State, w io.Writer, ctx context.Context, name string) (err error) {
	this.funcValues.SetDefault("yield", func(state *template.State) (template.HTML, error) {
		return this.Require(state, name)
	})

	layout := this.template.Layout
	usingDefaultLayout := false

	if layout == "" && this.template.UsingDefaultLayout {
		usingDefaultLayout = true
		layout = this.template.DefaultLayout
	}

	if layout != "" {
		name := filepath.Join("layouts", layout)

		if err = this.RequireC(state, w, ctx, name); err == nil {
			return
		} else if !usingDefaultLayout {
			err = fmt.Errorf("Failed to render layout: '%v.tmpl', got error: %v", filepath.Join("layouts", this.template.Layout), err)
			return
		} else {
			return
		}
	}

	return this.RequireC(state, w, ctx, name)
}
