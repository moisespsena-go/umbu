package template

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"runtime/debug"

	"github.com/moisespsena/template/funcs"
)

type Executor struct {
	parent          *Executor
	template        *Template
	funcs           *funcs.FuncValues
	writeError      int
	Local           *LocalData
	notCaptureError bool
}

func (te *Executor) Template() *Template {
	return te.template
}

func (te *Executor) Parent() *Executor {
	return te.parent
}

func (te *Executor) GetFuncs() *funcs.FuncValues {
	return te.funcs
}

func (te *Executor) NewChild() *Executor {
	child := NewExecutor(te.template)
	child.parent = te
	return child
}

func (te *Executor) WriteError() *Executor {
	if te.writeError != 1 {
		te = te.NewChild()
		te.writeError = 1
	}
	return te
}

func (te *Executor) NotWriteError() *Executor {
	if te.writeError != 2 {
		te = te.NewChild()
		te.writeError = 2
	}
	return te
}

func (te *Executor) IsWriteError() bool {
	p := te
	for p != nil {
		if p.writeError == 1 {
			return true
		}
		p = p.parent
	}
	return false
}

func (te *Executor) FilterFuncs(names ...string) *funcs.FuncValues {
	if len(names) == 0 {
		var items []*funcs.FuncValues
		e := te
		for e != nil {
			items = append(items, e.funcs)
			e = e.parent
		}
		return funcs.NewValues(items...)
	}
	fvalues := funcs.NewValues()
	for _, name := range names {
		f := te.FindFunc(name)
		if f == nil {
			panic(fmt.Errorf("Function %q doesn't exists.", name))
		}
		fvalues.SetValue(name, f)
	}
	return fvalues
}

func (te *Executor) AppendFuncs(funcMaps ...funcs.FuncMap) error {
	return te.funcs.Append(funcMaps...)
}

func (te *Executor) AppendFuncsValues(funcValues ...*funcs.FuncValues) *Executor {
	te.funcs.AppendValues(funcValues...)
	return te
}

func (te *Executor) Funcs(funcMaps ...funcs.FuncMap) *Executor {
	if len(funcMaps) > 0 {
		fv, err := funcs.CreateValuesFunc(funcMaps...)
		if err != nil {
			panic(err)
		}
		return te.NewChild().SetFuncs(fv)
	}
	return te
}

func (te *Executor) FuncsValues(funcValues ...*funcs.FuncValues) *Executor {
	if len(funcValues) > 0 {
		return te.NewChild().SetFuncs(funcs.NewValues(funcValues...))
	}
	return te
}

func (te *Executor) SetFuncs(values *funcs.FuncValues) *Executor {
	te.funcs = values
	return te
}

func (te *Executor) FindFunc(name string) *funcs.FuncValue {
	if fn := te.funcs.Get(name); fn != nil {
		return fn
	}
	if te.parent != nil {
		return te.parent.FindFunc(name)
	}
	return nil
}

type ErrorWithTrace struct {
	Err        string
	StackTrace []byte
}

func (et *ErrorWithTrace) Error() string {
	return et.Err
}

func (et *ErrorWithTrace) Trace() []byte {
	return et.StackTrace
}

func (e *Executor) execute(wr io.Writer, data interface{}) (err error) {
	if !e.notCaptureError {
		defer func() {
			if r := recover(); r != nil {
				if st, ok := r.(*ErrorWithTrace); ok {
					err = st
				} else {
					name := e.template.name
					if e.template.Path != "" {
						name = e.template.Path + "[" + name + "]"
					}
					err = &ErrorWithTrace{fmt.Sprintf("Get error when render %v: %v", name, r), debug.Stack()}
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

	t := e.template

	state := &state{
		e:            e,
		tmpl:         t,
		wr:           wr,
		vars:         []variable{{"$", value}},
		funcsValue:   make(map[string]*funcs.FuncValue),
		contextValue: funcs.NewContextValue(e.funcs),
		local:        e.Local,
	}

	if t.Tree == nil || t.Root == nil {
		state.errorf("%q is an incomplete or empty template", t.Name())
	}

	for fname, fun := range DefaultFuncMap {
		state.funcsValue[fname] = funcs.NewFuncValue(fun, nil)
	}
	state.funcsValue["_tpl_funcs"] = funcs.NewFuncValue(state.getFuncs, nil)
	state.funcsValue["_tpl_data_funcs"] = funcs.NewFuncValue(state.dataFuncs, nil)
	state.funcsValue["set"] = funcs.NewFuncValue(state.local.Set, nil)
	state.funcsValue["get"] = funcs.NewFuncValue(state.local.Get, nil)
	state.funcsValue["template_exec"] = funcs.NewFuncValue(state.templateExec, nil)
	state.funcsValue["trim"] = funcs.NewFuncValue(state.trim, nil)
	state.walk(value, t.Root)
	return
}

func (e *Executor) Execute(wr io.Writer, data interface{}, funcs_ ...interface{}) (err error) {
	ee := e

	if len(funcs_) > 0 {
		ee = e.NewChild()
		for i, fns := range funcs_ {
			if funcMaps, ok := fns.(funcs.FuncMap); ok {
				err = ee.AppendFuncs(funcMaps)
				if err != nil {
					if e.IsWriteError() {
						wr.Write([]byte(fmt.Sprint(err)))
					}
					return err
				}
			} else if funcValues, ok := fns.(*funcs.FuncValues); ok {
				ee.AppendFuncsValues(funcValues)
			} else {
				err = fmt.Errorf("Invalid func #%v of %v type", i, reflect.TypeOf(fns).String())
				if e.IsWriteError() {
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
		} else if funcsValues, ok := data.(*FuncValues); ok {
			return ee.FuncsValues(funcsValues).execute(wr, nil)
		}
	}
	return ee.execute(wr, data)
}

func (e *Executor) ExecuteString(data interface{}, funcs ...interface{}) (string, error) {
	var out bytes.Buffer
	err := e.Execute(&out, data, funcs...)
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
	data := make(LocalData)
	return &Executor{nil, t, fv, 0, &data, false}
}
