package template

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/moisespsena-go/tracederror"
	"github.com/moisespsena-go/umbu/funcs"
	"github.com/pkg/errors"
)

type ExecutorOptions struct {
	DotOverrideDisabled bool
}

type Executor struct {
	StateOptions
	parent         *Executor
	template       *Template
	funcs          funcs.FuncValues
	writeError     int
	Local          LocalData
	noCaptureError bool
	Context        context.Context
	super          *State
	rawData        func(dst io.Writer) error
}

func ExecutorOfRawData(rawData func(dst io.Writer) error) *Executor {
	return &Executor{rawData: rawData}
}

func (this *Executor) NoCaptureError() {
	this.noCaptureError = true
}

func (this *Executor) SetSuper(super *State) {
	this.super = super
	if super != nil {
		this.noCaptureError = true
	}
}

func (this *Executor) FullPath() (pth TemplatePath) {
	s := this.super
	for s != nil {
		var p = StateLocation{
			TemplateName: s.tmpl.name,
			TemplatePath: s.tmpl.Path,
		}

		if s.node != nil {
			p.Location, p.Context = s.tmpl.ErrorContext(s.node)
			p.Location = strings.TrimPrefix(p.Location, "'"+p.TemplateName+"':")
		}

		pth.pth = append(pth.pth, p)
		s = s.e.super
	}

	for i := 0; i < len(pth.pth)/2; i++ {
		j := len(pth.pth) - i - 1
		pth.pth[i], pth.pth[j] = pth.pth[j], pth.pth[i]
	}

	pth.pth = append(pth.pth, StateLocation{
		TemplateName: this.template.name,
		TemplatePath: this.template.Path,
	})
	return
}

func (this *Executor) Template() *Template {
	return this.template
}

func (this *Executor) Parent() *Executor {
	return this.parent
}

func (this *Executor) GetFuncs() funcs.FuncValues {
	return this.funcs
}

func (this *Executor) NewChild() *Executor {
	child := NewExecutor(this.template)
	child.parent = this
	child.StateOptions = this.StateOptions
	child.super = this.super
	return child
}

func (this *Executor) WriteError() *Executor {
	if this.writeError != 1 {
		this = this.NewChild()
		this.writeError = 1
	}
	return this
}

func (this *Executor) NotWriteError() *Executor {
	if this.writeError != 2 {
		this = this.NewChild()
		this.writeError = 2
	}
	return this
}

func (this *Executor) IsWriteError() bool {
	p := this
	for p != nil {
		if p.writeError == 1 {
			return true
		}
		p = p.parent
	}
	return false
}

func (this *Executor) FilterFuncs(names ...string) (funcs.FuncValues, error) {
	if len(names) == 0 {
		var items []funcs.FuncValues
		e := this
		for e != nil {
			items = append(items, e.funcs)
			e = e.parent
		}
		return funcs.NewValues(items...), nil
	}
	fvalues := funcs.NewValues()
	for _, name := range names {
		f := this.FindFunc(name)
		if f == nil {
			return nil, fmt.Errorf("Function %q doesn't exists.", name)
		}
		fvalues.SetValue(name, f)
	}
	return fvalues, nil
}

func (this *Executor) AppendFuncs(funcMaps ...funcs.FuncMap) error {
	return this.funcs.Append(funcMaps...)
}

func (this *Executor) AppendFuncsValues(funcValues ...funcs.FuncValues) *Executor {
	this.funcs.AppendValues(funcValues...)
	return this
}

func (this *Executor) Funcs(funcMaps ...funcs.FuncMap) *Executor {
	if len(funcMaps) > 0 {
		fv, err := funcs.CreateValuesFunc(funcMaps...)
		if err != nil {
			panic(err)
		}
		return this.NewChild().SetFuncs(fv)
	}
	return this
}

func (this *Executor) FuncsValues(funcValues ...funcs.FuncValues) *Executor {
	if len(funcValues) > 0 {
		return this.NewChild().SetFuncs(funcs.NewValues(funcValues...))
	}
	return this
}

func (this *Executor) SetFuncs(values funcs.FuncValues) *Executor {
	this.funcs = values
	return this
}

func (this *Executor) FindFunc(name string) *funcs.FuncValue {
	if fn := this.funcs.Get(name); fn != nil {
		return fn
	}
	if this.parent != nil {
		return this.parent.FindFunc(name)
	}
	return nil
}

func (this *Executor) execute(wr io.Writer, data interface{}) (err error) {
	if this.rawData != nil {
		return this.rawData(wr)
	}
	if !this.noCaptureError {
		defer func() {
			if r := recover(); r != nil {
				if r == errExit {
					return
				}
				if err2, ok := r.(error); ok && IsFatal(err2) {
					panic(err2)
				}
				if st, ok := r.(tracederror.TracedError); ok {
					err = st
				} else {
					name := this.FullPath()
					switch ee := r.(type) {
					case error:
						err = tracederror.New(errors.Wrapf(ee, "template %q", name))
					default:
						err = tracederror.New(fmt.Errorf("template %q: %v", name, r))
					}
				}
			}
		}()
	}
	var (
		value reflect.Value
		ok    bool
	)
	if data != nil {
		if value, ok = data.(reflect.Value); !ok {
			value = reflect.ValueOf(data)
		}
	}

	t := this.template

	state := &State{
		e:            this,
		tmpl:         t,
		wr:           wr,
		vars:         []variable{{"$", value}},
		global:       this.StateOptions.Global,
		funcsValue:   make(map[string]*funcs.FuncValue),
		contextValue: funcs.NewContextValue(this.funcs),
		local:        this.Local,
		context:      this.Context,
		data:         data,
		dataValue:    value,
	}

	if this.StateOptions.OnNoField == nil {
		this.StateOptions.OnNoField = func(recorde interface{}, fieldName string) (r interface{}, ok bool) {
			return
		}
	}

	if t.Tree == nil || t.Root == nil {
		state.errorf("'%s' is an incomplete or empty template", t.Name())
	}

	for fname, fun := range DefaultFuncMap {
		state.funcsValue[fname] = funcs.NewFuncValue(fun, nil)
	}

	stateValue := reflect.ValueOf(state)
	state.funcsValue["_tpl_state"] = funcs.NewFuncValue(func() reflect.Value {
		return stateValue
	}, nil)
	state.funcsValue["_tpl_funcs"] = funcs.NewFuncValue(state.getFuncs, nil)
	state.funcsValue["_tpl_data_funcs"] = funcs.NewFuncValue(state.dataFuncs, nil)
	state.funcsValue["set"] = funcs.NewFuncValue(state.local.Set, nil)
	state.funcsValue["get"] = funcs.NewFuncValue(state.local.Get, nil)
	state.funcsValue["template_exec"] = funcs.NewFuncValue(state.templateExec, nil)
	state.funcsValue["tpl_render"] = state.funcsValue["template_exec"]
	state.funcsValue["tpl_yield"] = funcs.NewFuncValue(state.templateYield, nil)
	state.funcsValue["trim"] = funcs.NewFuncValue(state.trim, nil)
	state.funcsValue["join"] = funcs.NewFuncValue(state.join, nil)
	state.walk(value, t.Root)
	return
}

func (this *Executor) Execute(wr io.Writer, data interface{}, funcs_ ...interface{}) (err error) {
	ee := this

	if len(funcs_) > 0 {
		ee = this.NewChild()
		for i, fns := range funcs_ {
			switch t := fns.(type) {
			case map[string]interface{}:
				err = ee.AppendFuncs(t)
				if err != nil {
					if this.IsWriteError() {
						wr.Write([]byte(fmt.Sprint(err)))
					}
					return err
				}
			case funcs.FuncMap:
				err = ee.AppendFuncs(t)
				if err != nil {
					if this.IsWriteError() {
						wr.Write([]byte(fmt.Sprint(err)))
					}
					return err
				}
			case funcs.FuncValues:
				ee.AppendFuncsValues(t)
			default:
				err = fmt.Errorf("Invalid func #%v of %v type", i, reflect.TypeOf(fns).String())
				if this.IsWriteError() {
					wr.Write([]byte(fmt.Sprint(err)))
				}
				return err
			}
		}
	}

	if data != nil {
		if dataHaveFuncs, ok := data.(*funcs.DataFuncs); ok {
			return ee.FuncsValues(dataHaveFuncs.GetFuncValues()).execute(wr, dataHaveFuncs.Data())
		} else if funcs, ok := data.(FuncMap); ok {
			return ee.Funcs(funcs).execute(wr, nil)
		} else if funcsValues, ok := data.(FuncValues); ok {
			return ee.FuncsValues(funcsValues).execute(wr, nil)
		}
	}
	err = ee.execute(wr, data)
	return
}

func (this *Executor) ExecuteString(data interface{}, funcs ...interface{}) (string, error) {
	var out bytes.Buffer
	err := this.Execute(&out, data, funcs...)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

func NewExecutor(t *Template, funcMaps ...funcs.FuncMap) *Executor {
	fv, err := funcs.CreateValuesFunc(funcMaps...)
	if err != nil {
		panic(err)
	}
	return &Executor{
		template:   t,
		funcs:      fv,
		writeError: 0,
		Local:      LocalData{},
		Context:    context.Background(),
	}
}
