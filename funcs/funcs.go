package funcs

import (
	"fmt"
	"reflect"
	"unicode"
)

// FuncMap is the type of the map defining the mapping from names to functions.
// Each function must have either a single return value, or two return values of
// which the second has type error. In that case, if the second (error)
// return value evaluates to non-nil during execution, execution terminates and
// Execute returns that error.
//
// When template execution invokes a function with an argument list, that list
// must be assignable to the function's parameter types. Functions meant to
// apply to arguments of arbitrary type can use parameters of type interface{} or
// of type reflect.Value. Similarly, functions meant to return a result of arbitrary
// type can return interface{} or reflect.Value.
type FuncMap map[string]interface{}

type FuncMapSlice []FuncMap

type FuncValue struct {
	f   interface{}
	v   reflect.Value
	ctx *FuncValue
}

func NewFuncValue(f interface{}, v *reflect.Value) (fv *FuncValue) {
	if v == nil {
		vv := reflect.ValueOf(f)
		v = &vv
	}

	if (*v).Kind() == reflect.Interface {
		*v = (*v).Elem()
	}

	fv = &FuncValue{f: f, v: *v}
	typ := v.Type()

	if typ.NumIn() == 1 && typ.In(0) == ContextType && typ.NumOut() == 1 {
		fv = &FuncValue{ctx: fv}
	}
	return
}

func (fv *FuncValue) F() interface{} {
	return fv.f
}

func (fv *FuncValue) V() reflect.Value {
	return fv.v
}

func (fv *FuncValue) Context() *FuncValue {
	return fv.ctx
}

func (fv *FuncValue) Value(context *Context) reflect.Value {
	return fv.ContextualValue(reflect.ValueOf(context))
}

func (fv *FuncValue) ContextualValue(context reflect.Value) reflect.Value {
	if fv.ctx != nil {
		return fv.ctx.v.Call([]reflect.Value{context})[0]
	}
	return fv.v
}

func (fv *FuncValue) Caller(context *Context) *ContextCaller {
	return &ContextCaller{f: fv.ContextualValue(context.Value)}
}

type FuncValuesSlice []FuncValues

func (this *FuncValuesSlice) Append(m ...FuncValues) {
	*this = append(*this, m...)
}

func (this *FuncValuesSlice) AppendSlice(m ...[]map[string]*FuncValue) {
	for _, m := range m {
		*this = append(*this, m)
	}
}

func (this *FuncValuesSlice) AppendMap(m ...map[string]*FuncValue) {
	this.Append(FuncValues(m))
}

type FuncValues []map[string]*FuncValue

var ContextType = reflect.TypeOf(&Context{})

func (v FuncValues) Get(name string) *FuncValue {
	if len(v) == 0 {
		return nil
	}

	for _, m := range v {
		if f := m[name]; f != nil {
			return f
		}
	}
	return nil
}

func (v *FuncValues) SetPair(name string, f interface{}, vf reflect.Value, check ...bool) (err error) {
	if checkArg(check) {
		err = CheckFuncValue(name, vf)
		if err != nil {
			return
		}
	}
	v.SetValue(name, NewFuncValue(f, &vf), false)
	return nil
}

func (v *FuncValues) SetValue(name string, value *FuncValue, check ...bool) error {
	if checkArg(check) {
		err := CheckFuncValue(name, value.v)
		if err != nil {
			return err
		}
	}
	if len(*v) == 0 {
		*v = []map[string]*FuncValue{{}}
	}
	(*v)[0][name] = value
	return nil
}

func (v *FuncValues) Set(name string, f interface{}, check ...bool) error {
	return v.SetPair(name, f, reflect.ValueOf(f), check...)
}

func (v *FuncValues) Has(name string) bool {
	return v.Get(name) != nil
}

func (v *FuncValues) SetDefault(name string, f interface{}) interface{} {
	fv := v.Get(name)
	if fv == nil {
		v.Set(name, f)
		return f
	}
	return fv.f
}

func (v *FuncValues) GetDefault(name string, f interface{}) interface{} {
	fv := v.Get(name)
	if fv == nil {
		return f
	}
	return fv.f
}

func (v *FuncValues) Append(funcMaps ...FuncMap) error {
	for _, funcMap := range funcMaps {
		for name, fn := range funcMap {
			err := v.Set(name, fn)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *FuncValues) AppendValues(items ...FuncValues) {
	for _, item := range items {
		if item != nil {
			*v = append(*v, item...)
		}
	}
}

func (v *FuncValues) Start() *FuncValues {
	if len(*v) == 0 {
		*v = []map[string]*FuncValue{{}}
	}
	return v
}

func NewValues(items ...FuncValues) FuncValues {
	values := FuncValues{{}}

	for _, item := range items {
		for _, e := range item {
			if len(e) > 0 {
				values = append(values, e)
			}
		}
	}

	return values
}

type Context struct {
	Value reflect.Value
	Funcs FuncValues
}

func (ctx *Context) Get(name string) *ContextCaller {
	return ctx.Funcs.Get(name).Caller(ctx)
}

func NewContextValue(funcs FuncValues) reflect.Value {
	ctx := &Context{Funcs: funcs}
	ctx.Value = reflect.ValueOf(ctx)
	return ctx.Value
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// GoodFunc reports whether the function or method has the right result signature.
func GoodFunc(typ reflect.Type) bool {
	// We allow functions with 0 or 1 result or 2 results where the second is an error.
	switch typ.NumOut() {
	case 0, 1:
		return true
	case 2:
		return typ.NumOut() == 2 && typ.Out(1) == errorType
	default:
		return false
	}
}

// GoodName reports whether the function name is a valid identifier.
func GoodName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r == '_':
		case i == 0 && !unicode.IsLetter(r):
			return false
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			return false
		}
	}
	return true
}

func CreateValuesFunc(funcMaps ...FuncMap) (FuncValues, error) {
	values := NewValues()
	err := values.Append(funcMaps...)
	if err != nil {
		return nil, err
	}
	return values, nil
}

func CheckName(name string) error {
	if !GoodName(name) {
		return fmt.Errorf("function name %q is not a valid identifier", name)
	}
	return nil
}

func CheckFuncValue(name string, vf reflect.Value) error {
	if vf.Kind() != reflect.Func {
		return fmt.Errorf("value for %q not a function", name)
	}
	if !vf.IsValid() {
		return fmt.Errorf("value for %q isn't a valid function", name)
	}
	if !GoodFunc(vf.Type()) {
		return fmt.Errorf("can't install method/function %q: bad return type", name)
	}
	return nil
}

func CheckFunc(name string, f interface{}) (err error) {
	err = CheckName(name)
	if err != nil {
		return
	}
	err = CheckFuncValue(name, reflect.ValueOf(f))
	return
}

func checkArg(check []bool) bool {
	return len(check) == 0 || check[0]
}

type DataFuncs struct {
	data  interface{}
	funcs FuncValues
}

func (df *DataFuncs) Data() interface{} {
	return df.data
}

func (df *DataFuncs) Funcs(funcs ...FuncMap) error {
	return df.funcs.Append(funcs...)
}

func (df *DataFuncs) FuncsValues(funcsValues ...FuncValues) {
	df.funcs.AppendValues(funcsValues...)
}

func (df *DataFuncs) GetFuncValues() FuncValues {
	return df.funcs
}

func NewDataFuncs(data interface{}) *DataFuncs {
	var fv FuncValues
	if df, ok := data.(*DataFuncs); ok {
		data = df.data
		fv.AppendValues(df.GetFuncValues())
	}
	return &DataFuncs{data, fv}
}
