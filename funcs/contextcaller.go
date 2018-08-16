package funcs

import "reflect"

type ContextCaller struct {
	f reflect.Value
	context *Context
	args []reflect.Value
}

func (ctx *ContextCaller) Args(args... interface{}) *ContextCaller {
	for _, arg := range args {
		ctx.args = append(ctx.args, reflect.ValueOf(arg))
	}
	return ctx
}

func (ctx *ContextCaller) SetArgs(args []interface{}) *ContextCaller {
	vargs := make([]reflect.Value, len(args))
	for i, arg := range args {
		vargs[i] = reflect.ValueOf(arg)
	}
	return ctx
}

func (ctx *ContextCaller) Call() []reflect.Value {
	return ctx.f.Call(ctx.args)
}

func (ctx *ContextCaller) CallFirst() reflect.Value {
	return ctx.Call()[0]
}

func (ctx *ContextCaller) CallFirstInterface() interface{} {
	return ctx.CallFirst().Interface()
}

func (ctx *ContextCaller) String() string {
	return ctx.CallFirstInterface().(string)
}

func (ctx *ContextCaller) Int() int {
	return ctx.CallFirstInterface().(int)
}

func (ctx *ContextCaller) Int64() int64 {
	return ctx.CallFirstInterface().(int64)
}

func (ctx *ContextCaller) Float32() float32 {
	return ctx.CallFirstInterface().(float32)
}

func (ctx *ContextCaller) Float64() float64 {
	return ctx.CallFirstInterface().(float64)
}

func (ctx *ContextCaller) Bool() bool {
	return ctx.CallFirstInterface().(bool)
}