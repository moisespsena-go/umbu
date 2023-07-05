// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package template

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"unicode"

	"github.com/moisespsena-go/umbu/expr"

	"github.com/pkg/errors"

	"github.com/moisespsena-go/tracederror"
	"github.com/moisespsena-go/umbu/funcs"
	"github.com/moisespsena-go/umbu/text/template/parse"
)

// maxExecDepth specifies the maximum stack depth of templates within
// templates. This limit is only practically reached by accidentally
// recursive template invocations. This limit allows us to return
// an error instead of triggering a stack overflow.
const maxExecDepth = 100000

type StateOptions struct {
	RequireFields bool
	OnNoField     func(recorde interface{}, fieldName string) (r interface{}, ok bool)
	Global        []variable
}

// State represents the State of an execution. It's not part of the
// template so that multiple executions of the same template
// can execute in parallel.
type State struct {
	e            *Executor
	tmpl         *Template
	wr           io.Writer
	node         parse.Node // current node, for errors
	vars         []variable // push-down stack of variable values.
	global       []variable
	depth        int // the height of the stack of executing templates.
	funcsValue   map[string]*funcs.FuncValue
	contextValue reflect.Value
	local        LocalData
	context      context.Context
	data         interface{}
	dataValue    reflect.Value
}

// variable holds the dynamic value of a variable such as $, $x etc.
type variable struct {
	name  string
	value reflect.Value
}

func (this *State) withWriter(w io.Writer) func() {
	oldWr := this.wr
	this.wr = w
	return func() {
		this.wr = oldWr
	}
}

func (this *State) Data() interface{} {
	return this.data
}

// Executor returns the executor object.
func (this *State) Executor() *Executor {
	return this.e
}

// Template returns the template object.
func (this *State) Template() *Template {
	return this.tmpl
}

// Depth returns the height of the stack of executing templates.
func (this *State) Depth() int {
	return this.depth
}

// Local returns the local data.
func (this *State) Local() LocalData {
	return this.local
}

// Context returns the context object.
func (this *State) Context() context.Context {
	return this.context
}

// WithContext set temporary context and rollback it on ret func called.
func (this *State) WithContext(ctx context.Context) func() {
	old := this.context
	this.context = ctx
	return func() {
		this.context = old
	}
}

// Local returns the writer.
func (this *State) Writer() io.Writer {
	return this.wr
}

// push pushes a new variable on the stack.
func (this *State) push(name string, value reflect.Value) {
	this.vars = append(this.vars, variable{name, value})
}

// mark returns the length of the variable stack.
func (this *State) mark() int {
	return len(this.vars)
}

// pop pops the variable stack up to the mark.
func (this *State) pop(mark int) {
	this.vars = this.vars[0:mark]
}

// setVar overwrites the top-nth variable on the stack. Used by range iterations.
func (this *State) setVar(n int, value reflect.Value) {
	this.vars[len(this.vars)-n].value = value
}

// getVar returns the top-nth variable on the stack.
func (this *State) getVar(n int) *variable {
	return &this.vars[len(this.vars)-n]
}

// varValue returns the value of the named variable.
func (this *State) updateVar(name string, value reflect.Value) {
	var v *variable
	for i := this.mark() - 1; i >= 0; i-- {
		if v2 := &this.vars[i]; v2.name == name {
			v = &this.vars[i]
			break
		}
	}
	if v == nil {
		this.errorf("undefined variable: %s", name)
		return
	}
	v.value = value
}

// varValue returns the value of the named variable.
func (this *State) changeVarExpr(name string, value reflect.Value, op rune) {
	var v *variable
	for i := this.mark() - 1; i >= 0; i-- {
		if v2 := &this.vars[i]; v2.name == name {
			v = &this.vars[i]
			break
		}
	}
	if v == nil {
		this.errorf("undefined variable: %s", name)
		return
	}

	switch value.Kind() {
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value = reflect.ValueOf(value.Uint())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value = reflect.ValueOf(value.Int())
	case reflect.Float32, reflect.Float64:
		value = reflect.ValueOf(value.Float())
	}
	v.value = this.exp(op, v.value, value)
}

// varValue returns the value of the named variable.
func (this *State) varValue(name string) (value reflect.Value) {
	for i := this.mark() - 1; i >= 0; i-- {
		if v := &this.vars[i]; v.name == name {
			return this.vars[i].value
		}
	}
	l := len(this.global)
	for i := l; i > 0; i-- {
		if this.global[i-1].name == name {
			return this.global[i-1].value
		}
	}
	this.errorf("undefined variable: %s", name)
	return zero
}

func (this *State) GetVar(name string) (value reflect.Value) {
	l := len(this.vars)
	for i := l; i > 0; i-- {
		if this.vars[i-1].name == name {
			return this.vars[i-1].value
		}
	}
	l = len(this.global)
	for i := l; i > 0; i-- {
		if this.global[i-1].name == name {
			return this.global[i-1].value
		}
	}
	return
}

var zero reflect.Value

// at marks the State to be on node n, for error reporting.
func (this *State) at(node parse.Node) {
	this.node = node
}

// doublePercent returns the string with %'s replaced by %%, if necessary,
// so it can be used safely inside a Printf format string.
func doublePercent(str string) string {
	if strings.Contains(str, "%") {
		str = strings.Replace(str, "%", "%%", -1)
	}
	return str
}

func (this *State) errorInfo() (info string) {
	name := this.e.FullPath()
	if this.node == nil {
		info = fmt.Sprintf("template: %q", name)
	} else {
		location, context := this.tmpl.ErrorContext(this.node)
		info = fmt.Sprintf("template: %q: executing %q at <%s>", location, name, doublePercent(context))
	}
	return
}

func (this *State) panic(err error) {
	if err == errExit {
		panic(err)
	}
	info := this.errorInfo()
	var ewt tracederror.TracedError
	switch t := err.(type) {
	case tracederror.TracedError:
		ewt = &fatal{errors.Wrap(err, info), t.Trace()}
	case interface {
		error
		Trace() []byte
	}:
		ewt = &fatal{errors.Wrap(err, info), t.Trace()}
	default:
		ewt = &fatal{errors.Wrap(err, info), debug.Stack()}
	}
	panic(ewt)
}

// errorf records an ExecError and terminates processing.
func (this *State) errorf(format string, args ...interface{}) {
	panic(ExecError{
		Node: this.node,
		Name: this.tmpl.Name(),
		Err:  tracederror.New(errors.Wrap(fmt.Errorf(format, args...), this.errorInfo())),
	})
}

// writeError is the wrapper type used internally when Execute has an
// error writing to its output. We strip the wrapper in errRecover.
// Note that this is not an implementation of error, so it cannot escape
// from the package as an error value.
type writeError struct {
	Err error // Original error.
}

func (this *State) writeError(err error) {
	panic(writeError{
		Err: err,
	})
}

// errRecover is the handler that turns panics into returns from the top
// level of Parse.
func errRecover(errp *error) {
	e := recover()
	if e != nil {
		switch err := e.(type) {
		case runtime.Error:
			panic(e)
		case writeError:
			*errp = err.Err // Strip the wrapper.
		case ExecError:
			*errp = err // Keep the wrapper.
		default:
			panic(e)
		}
	}
}

// ExecuteTemplate applies the template associated with t that has the given name
// to the specified data object and writes the output to wr.
// If an error occurs executing the template or writing its output,
// execution stops, but partial results may already have been written to
// the output writer.
// A template may be executed safely in parallel, although if parallel
// executions share a Writer the output may be interleaved.
func (t *Template) ExecuteTemplate(wr io.Writer, name string, data interface{}) error {
	var tmpl *Template
	if t.common != nil {
		tmpl = t.tmpl[name]
	}
	if tmpl == nil {
		return fmt.Errorf("template: no template %q associated with template %q", name, t.name)
	}
	return tmpl.Executor().Execute(wr, data)
}

func (t *Template) Executor(funcMaps ...funcs.FuncMap) *Executor {
	return t.CreateExecutor(funcMaps...)
}

func (t *Template) CreateExecutor(funcMaps ...funcs.FuncMap) *Executor {
	return NewExecutor(t).SetFuncs(builtinFuncs).FuncsValues(t.funcs).Funcs(funcMaps...)
}

// Execute applies a parsed template to the specified data object,
// and writes the output to wr.
// If an error occurs executing the template or writing its output,
// execution stops, but partial results may already have been written to
// the output writer.
// A template may be executed safely in parallel, although if parallel
// executions share a Writer the output may be interleaved.
//
// If data is a reflect.Value, the template applies to the concrete
// value that the reflect.Value holds, as in fmt.Print.
func (t *Template) Execute(wr io.Writer, data interface{}) error {
	return t.Executor().Execute(wr, data)
}

func (t *Template) ExecuteString(data interface{}) (string, error) {
	return t.CreateExecutor().ExecuteString(data)
}

// DefinedTemplates returns a string listing the defined templates,
// prefixed by the string "; defined templates are: ". If there are none,
// it returns the empty string. For generating an error message here
// and in html/template.
func (t *Template) DefinedTemplates() string {
	if t.common == nil {
		return ""
	}
	var b bytes.Buffer
	for name, tmpl := range t.tmpl {
		if tmpl.Tree == nil || tmpl.Root == nil {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", name)
	}
	var s string
	if b.Len() > 0 {
		s = "; defined templates are: " + b.String()
	}
	return s
}

// Walk functions step through the major pieces of the template structure,
// generating output as they go.
func (this *State) walk(dot reflect.Value, node parse.Node) {
	this.at(node)
	switch node := node.(type) {
	case *parse.ActionNode:
		// Do not pop variables so they persist until next end.
		// Also, if the action declares variables, don't print the result.
		val := this.evalPipeline(dot, node.Pipe)
		if len(node.Pipe.Decl) == 0 {
			this.printValue(node, val)
		}
	case *parse.ExprNode:
		println("***")
	case *parse.IfNode:
		this.walkIfOrWith(parse.NodeIf, dot, node.Pipe, node.List, node.ElseList)
	case *parse.ListNode:
		for _, node := range node.Nodes {
			this.walk(dot, node)
		}
	case *parse.RangeNode:
		this.walkRange(dot, node)
	case *parse.TemplateNode:
		this.walkTemplate(dot, node)
	case *parse.TextNode:
		if _, err := this.wr.Write(node.Text); err != nil {
			this.writeError(err)
		}
	case *parse.WithNode:
		this.walkIfOrWith(parse.NodeWith, dot, node.Pipe, node.List, node.ElseList)
	case *parse.ArgNode:
		this.walkArg(parse.NodeArg, dot, node.Pipe, node.List)
	case *parse.CallbackNode:
		this.walkCallback(parse.NodeCallback, dot, node.Pipe, node.List)
	case *parse.WrapNode:
		this.walkWrap(parse.NodeWrap, dot, node)
	default:
		this.errorf("unknown node: %s", node)
	}
}

// walkIfOrWith walks an 'if' or 'with' node. The two control structures
// are identical in behavior except that 'with' sets dot.
func (this *State) walkIfOrWith(typ parse.NodeType, dot reflect.Value, pipe *parse.PipeNode, list, elseList *parse.ListNode) {
	defer this.pop(this.mark())
	val := this.evalPipeline(dot, pipe)
	truth, ok := isTrue(val)
	if !ok {
		this.errorf("if/with can't use %v", val)
	}
	if truth {
		if typ == parse.NodeWith {
			this.walk(val, list)
		} else {
			this.walk(dot, list)
		}
	} else if elseList != nil {
		this.walk(dot, elseList)
	}
}

// walkArg walks an 'arg' node.
func (this *State) walkArg(typ parse.NodeType, dot reflect.Value, pipe *parse.PipeNode, list *parse.ListNode) {
	defer this.pop(this.mark())
	args, pipec := pipe.Cmds[0:len(pipe.Cmds)-1], pipe.Cmds[len(pipe.Cmds)-1]
	if len(args) > 0 {
		dot = this.evalCommand(dot, args[0], dot) // previous value is this one's final arg.
		// If the object has type interface{}, dig down one level to the thing inside.
		if dot.Kind() == reflect.Interface && dot.Type().NumMethod() == 0 {
			dot = reflect.ValueOf(dot.Interface()) // lovely!
		}
	}
	oldWr := this.wr
	var w bytes.Buffer
	this.wr = &w
	this.walk(dot, list)
	this.wr = oldWr

	pipec = pipec.Copy().(*parse.CommandNode)
	pipec.Args = append(pipec.Args, &parse.StringNode{Text: w.String()})
	var value reflect.Value
	value = this.evalCommand(dot, pipec, value)
	switch value.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Interface:
		if !value.IsNil() {
			fmt.Fprint(this.wr, value)
		}
	default:
		fmt.Fprint(this.wr, value)
	}
}

// walkArg walks an 'arg' node.
func (this *State) walkCallback(typ parse.NodeType, dot reflect.Value, pipe *parse.PipeNode, list *parse.ListNode) {
	defer this.pop(this.mark())
	args, pipec := pipe.Cmds[0:len(pipe.Cmds)-1], pipe.Cmds[len(pipe.Cmds)-1]
	if len(args) > 0 {
		dot = this.evalCommand(dot, args[0], dot) // previous value is this one's final arg.
		// If the object has type interface{}, dig down one level to the thing inside.
		if dot.Kind() == reflect.Interface && dot.Type().NumMethod() == 0 {
			dot = reflect.ValueOf(dot.Interface()) // lovely!
		}
	}

	var callCount int

	this.push("$0", reflect.ValueOf(&callCount))
	this.push("$@", reflect.Value{})
	this.push("$!", reflect.Value{})

	handler := WalkHandler(func(w io.Writer, dot interface{}, args ...interface{}) (err error) {
		defer this.pop(this.mark())
		this.setVar(2, reflect.ValueOf(args))
		this.setVar(1, reflect.ValueOf(len(args)))
		if w != nil {
			defer this.withWriter(w)()
		}
		defer func() {
			if r := recover(); r != nil {
				err = r.(error)
			}
		}()
		this.walk(reflect.ValueOf(dot), list)
		callCount++
		return
	})

	pipec = pipec.Copy().(*parse.CommandNode)

	newArgs := make([]parse.Node, len(pipec.Args)+2)
	// function handler is first arg
	newArgs[0] = pipec.Args[0]
	newArgs[1] = &parse.ValNode{
		NodeType: parse.NodeVal,
		Pos:      pipe.Pos,
		Value:    dot,
	}
	newArgs[2] = &parse.ValNode{
		NodeType: parse.NodeVal,
		Pos:      pipe.Pos,
		Value:    reflect.ValueOf(handler),
	}
	// other args is after callback function
	copy(newArgs[3:], pipec.Args[1:])
	pipec.Args = newArgs

	var value reflect.Value
	value = this.evalCommand(dot, pipec, value)
	switch value.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Interface:
		if !value.IsNil() {
			fmt.Fprint(this.wr, value)
		}
	default:
		fmt.Fprint(this.wr, value)
	}
}

// walkWrap walks an 'wrap' node.
func (this *State) walkWrap(_ parse.NodeType, dot reflect.Value, node *parse.WrapNode) {
	defer this.pop(this.mark())
	oldWr := this.wr
	var w = wrapWriter{
		begin: func(w io.Writer) {
			oldW := this.wr
			defer func() {
				this.wr = oldW
			}()
			this.wr = w
			if node.BeginList != nil {
				this.walk(dot, node.BeginList)
			}
		},
		w:     this.wr,
		strip: node.Pipe.TrimRight,
	}
	if len(node.Pipe.Cmds) == 1 && node.Pipe.Cmds[0].String() == "strip" {
		w.strip = true
	}
	this.wr = &w
	this.walk(dot, node.List)
	if w.noEmpty {
		if node.AfterList != nil {
			this.walk(dot, node.AfterList)
		}
	} else {
		this.wr = oldWr
		if node.ElseList != nil {
			this.walk(dot, node.ElseList)
		}
	}
}

// IsTrue reports whether the value is 'true', in the sense of not the zero of its type,
// and whether the value has a meaningful truth value. This is the definition of
// truth used by if and other such actions.
func IsTrue(val interface{}) (truth, ok bool) {
	return isTrue(reflect.ValueOf(val))
}

func isTrue(val reflect.Value) (truth, ok bool) {
	if !val.IsValid() {
		// Something like var x interface{}, never set. It's a form of nil.
		return false, true
	}
	switch val.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		truth = val.Len() > 0
	case reflect.Bool:
		truth = val.Bool()
	case reflect.Complex64, reflect.Complex128:
		truth = val.Complex() != 0
	case reflect.Chan, reflect.Func:
		truth = !val.IsNil()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		truth = val.Int() != 0
	case reflect.Float32, reflect.Float64:
		truth = val.Float() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		truth = val.Uint() != 0
	case reflect.Ptr, reflect.Interface:
		if val.IsNil() {
			truth = false
		} else {
			return isTrue(val.Elem())
		}
	case reflect.Struct:
		switch vt := val.Interface().(type) {
		case ResultOk:
			truth = vt.Ok
		case interface{ IsZero() bool }:
			truth = !vt.IsZero()
		default:
			truth = true // Struct values are always true.
		}
	default:
		return
	}
	return truth, true
}

func (this *State) walkTemplate(dot reflect.Value, t *parse.TemplateNode) {
	this.at(t)
	tmpl := this.tmpl.tmpl[t.Name]
	if tmpl == nil {
		this.errorf("template %q not defined", t.Name)
	}
	if this.depth == maxExecDepth {
		this.errorf("exceeded maximum template depth (%v)", maxExecDepth)
	}

	var args []parse.Node
	if t.Pipe != nil {
		if len(t.Pipe.Cmds) == 1 {
			oldArgs := t.Pipe.Cmds[0].Args
			args = oldArgs[1:]
			t.Pipe.Cmds[0].Args = oldArgs[0:1]
			// Variables declared by the pipeline persist.
			dot = this.evalPipeline(dot, t.Pipe)
			t.Pipe.Cmds[0].Args = oldArgs
		}
	}
	if len(args) < len(tmpl.args) {
		this.errorf("bad template args %q. Want %d but got %d.", t.Name, len(tmpl.args), len(args))
	}
	newState := *this
	newState.depth++
	newState.tmpl = tmpl
	if len(tmpl.funcs) > 0 {
		defer this.e.funcs.With(tmpl.funcs)()
	}
	// No dynamic scoping: template invocations inherit no variables.
	newState.vars = append(append([]variable{}, newState.vars[:tmpl.Tree.InheritedVarsLen]...), variable{"$", dot})
	for i, arg := range args {
		cmd := *t.Pipe.Cmds[0]
		cmd.Args = []parse.Node{arg}
		newState.vars = append(newState.vars, variable{tmpl.args[i], this.evalCommand(dot, &cmd, reflect.Value{})})
	}
	newState.walk(dot, tmpl.Root)
}

// Eval functions evaluate pipelines, commands, and their elements and extract
// values from the data structure by examining fields, calling methods, and so on.
// The printing of those values happens only through walk functions.

// evalPipeline returns the value acquired by evaluating a pipeline. If the
// pipeline has a variable declaration, the variable will be pushed on the
// stack. Callers should therefore pop the stack after they are finished
// executing commands depending on the pipeline value.
func (this *State) evalPipeline(dot reflect.Value, pipe *parse.PipeNode) (value reflect.Value) {
	if pipe == nil {
		return
	}
	this.at(pipe)
	for _, cmd := range pipe.Cmds {
		value = this.evalCommand(dot, cmd, value) // previous value is this one's final arg.
		// If the object has type interface{}, dig down one level to the thing inside.
		if value.Kind() == reflect.Interface && value.Type().NumMethod() == 0 {
			value = reflect.ValueOf(value.Interface()) // lovely!
		}
	}
	for _, variable := range pipe.Decl {
		if variable.Op == '=' {
			if variable.Update {
				this.updateVar(variable.Ident[0], value)
			} else {
				this.push(variable.Ident[0], value)
			}
		} else {
			this.changeVarExpr(variable.Ident[0], value, variable.Op)
		}
	}
	return value
}

func (this *State) notAFunction(args []parse.Node, final reflect.Value) {
	if len(args) > 1 || final.IsValid() {
		this.errorf("can't give argument to non-function %s", args[0])
	}
}

func (this *State) evalCommand(dot reflect.Value, cmd *parse.CommandNode, final reflect.Value) reflect.Value {
	firstWord := cmd.Args[0]
	switch n := firstWord.(type) {
	case *parse.FieldNode:
		return this.evalFieldNode(dot, n, cmd.Args, final)
	case *parse.ChainNode:
		return this.evalChainNode(dot, n, cmd.Args, final)
	case *parse.IdentifierNode:
		if n.Ident == Globals {
			return this.dataValue
		}
		if n.Ident == Self {
			return this.vars[0].value
		}
		// Must be a function.
		return this.evalFunction(dot, n, cmd, cmd.Args, final)
	case *parse.PipeNode:
		// Parenthesized pipeline. The arguments are all inside the pipeline; final is ignored.
		return this.evalPipeline(dot, n)
	case *parse.VariableNode:
		return this.evalVariableNode(dot, n, cmd.Args, final)
	case *parse.ExprNode:
		return this.evalExprNode(dot, n, cmd.Args, final)
	}
	this.at(firstWord)
	this.notAFunction(cmd.Args, final)
	switch word := firstWord.(type) {
	case *parse.BoolNode:
		return reflect.ValueOf(word.True)
	case *parse.DotNode:
		return dot
	case *parse.NilNode:
		return reflect.Value{}
	case *parse.NumberNode:
		return this.idealConstant(word)
	case *parse.StringNode:
		return reflect.ValueOf(word.Text)
	case *parse.ExprNode:
		return this.evalExprNode(dot, word, cmd.Args, final)
	case *parse.ValNode:
		return word.Value
	case *parse.ValFactoryNode:
		return word.New()
	}
	this.errorf("can't evaluate command %q", firstWord)
	panic("not reached")
}

// idealConstant is called to return the value of a number in a context where
// we don't know the type. In that case, the syntax of the number tells us
// its type, and we use Go rules to resolve. Note there is no such thing as
// a uint ideal constant in this situation - the value must be of int type.
func (this *State) idealConstant(constant *parse.NumberNode) reflect.Value {
	// These are ideal constants but we don't know the type
	// and we have no context.  (If it was a method argument,
	// we'd know what we need.) The syntax guides us to some extent.
	this.at(constant)
	switch {
	case constant.IsComplex:
		return reflect.ValueOf(constant.Complex128) // incontrovertible.
	case constant.IsFloat && !isHexConstant(constant.Text) && strings.ContainsAny(constant.Text, ".eE"):
		return reflect.ValueOf(constant.Float64)
	case constant.IsInt:
		n := int(constant.Int64)
		if int64(n) != constant.Int64 {
			this.errorf("%s overflows int", constant.Text)
		}
		return reflect.ValueOf(n)
	case constant.IsUint:
		this.errorf("%s overflows int", constant.Text)
	}
	return zero
}

func isHexConstant(s string) bool {
	return len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X')
}

func (this *State) evalFieldNode(dot reflect.Value, field *parse.FieldNode, args []parse.Node, final reflect.Value) reflect.Value {
	this.at(field)
	return this.evalFieldChain(dot, dot, field, field.Ident, args, final)
}

func (this *State) evalChainNode(dot reflect.Value, chain *parse.ChainNode, args []parse.Node, final reflect.Value) reflect.Value {
	this.at(chain)
	if len(chain.Field) == 0 {
		this.errorf("internal error: no fields in evalChainNode")
	}
	if chain.Node.Type() == parse.NodeNil {
		this.errorf("indirection through explicit nil in %s", chain)
	}
	// (pipe).Field1.Field2 has pipe as .Node, fields as .Field. Eval the pipeline, then the fields.
	pipe := this.evalArg(dot, nil, chain.Node)
	return this.evalFieldChain(dot, pipe, chain, chain.Field, args, final)
}

func (this *State) evalVariableNode(dot reflect.Value, variable *parse.VariableNode, args []parse.Node, final reflect.Value) reflect.Value {
	// $x.Field has $x as the first ident, Field as the second. Eval the var, then the fields.
	this.at(variable)
	value := this.varValue(variable.Ident[0])
	if len(variable.Ident) == 1 {
		this.notAFunction(args, final)
		return value
	}
	return this.evalFieldChain(dot, value, variable, variable.Ident[1:], args, final)
}

func (this *State) evalExprNode(dot reflect.Value, node *parse.ExprNode, args []parse.Node, final reflect.Value) (v reflect.Value) {
	a := this.evalCommand(dot, node.A, final)
	b := this.evalCommand(dot, node.B, final)
	v, err := expr.Expr(node.Op, a, b)
	if err != nil {
		this.errorf(err.Error())
	}
	return v
}

// evalFieldChain evaluates .X.Y.Z possibly followed by arguments.
// dot is the environment in which to evaluate arguments, while
// receiver is the value being walked along the chain.
func (this *State) evalFieldChain(dot, receiver reflect.Value, node parse.Node, ident []string, args []parse.Node, final reflect.Value) reflect.Value {
	n := len(ident)
	for i := 0; i < n-1; i++ {
		receiver = this.evalField(dot, ident[i], node, nil, zero, receiver)
	}
	// Now if it's a method, it gets the arguments.
	return this.evalField(dot, ident[n-1], node, args, final, receiver)
}

func (this *State) getFuncs(names ...string) (funcs funcs.FuncValues) {
	var err error
	if funcs, err = this.e.FilterFuncs(names...); err != nil {
		this.panic(err)
	}
	return
}

func (this *State) getExecutor() *Executor {
	return this.e
}

func (this *State) dataFuncs(data interface{}, funcsNames ...interface{}) (df *funcs.DataFuncs) {
	df = funcs.NewDataFuncs(data)
	funcValues := funcs.FuncValues{}

	l := len(funcsNames)
	for i := 0; i < l; i++ {
		switch fv := funcsNames[i].(type) {
		case string:
			x := i + 1
			if x == l {
				funcValues.SetValue(fv, this.getFuncValue(fv))
			} else {
				next := funcsNames[x]
				if _, ok := next.(string); ok {
					funcValues.SetValue(fv, this.getFuncValue(fv), false)
				} else {
					err := funcValues.Set(fv, next)
					if err != nil {
						this.errorf("dataFuncs: invalid funcName[%v]: %v", x, next)
					}
					i++
				}
			}
		default:
			this.errorf("dataFuncs: invalid funcName[%v]", i)
			return
		}
	}

	return
}

var (
	nilValue   = reflect.Value{}
	blankValue = reflect.ValueOf("")
)

func (this *State) getFuncValue(name string) (v *funcs.FuncValue) {
	if v = this.GetFunc(name); v == nil {
		this.errorf("%q is not a defined function", name)
	}
	return v
}

func (this *State) GetFunc(name string) (v *funcs.FuncValue) {
	if v, ok := this.funcsValue[name]; ok {
		return v
	}
	if v = this.tmpl.funcs.Get(name); v != nil {
		return v
	}
	if v = this.e.FindFunc(name); v != nil {
		return v
	}

	// try get func from global attr
	receiver := reflect.ValueOf(this.data)

	if !receiver.IsValid() {
		return
	}

	typ := receiver.Type()

	if i, ok := receiver.Interface().(AttrGetter); ok {
		if val, ok := i.GetAttr(name); ok {
			rv := reflect.ValueOf(val)
			if rv.Kind() == reflect.Func {
				return funcs.NewFuncValue(val, &rv)
			}
		}
		return
	}

	receiver, isNil := indirect(receiver)
	// Unless it's an interface, need to get to a value of type *T to guarantee
	// we see all methods of T and *T.
	ptr := receiver
	if ptr.Kind() != reflect.Interface && ptr.Kind() != reflect.Ptr && ptr.CanAddr() {
		ptr = ptr.Addr()
	}
	if method := ptr.MethodByName(name); method.IsValid() {
		return funcs.NewFuncValue(nil, &method)
	}

	switch receiver.Kind() {
	case reflect.Struct:
		tField, ok := receiver.Type().FieldByName(name)
		if ok {
			if isNil {
				return
			}
			field := receiver.FieldByIndex(tField.Index)
			if tField.PkgPath != "" { // field is unexported
				return
			}
			return funcs.NewFuncValue(nil, &field)
		}
	case reflect.Map:
		if isNil {
			this.errorf("nil pointer evaluating %s.%s", typ, name)
		}
		// If it's a map, attempt to use the field name as a key.
		nameVal := reflect.ValueOf(name)
		if nameVal.Type().AssignableTo(receiver.Type().Key()) {
			result := receiver.MapIndex(nameVal)
			if !result.IsValid() {
				return
			}
			return funcs.NewFuncValue(nil, &result)
		}
	}
	return
}

func (this *State) Call(name string, args, result []any) (ok bool) {
	f := this.GetFunc(name)
	if f == nil {
		return
	}
	ok = true

	var (
		fun     = f.ContextualValue(this.contextValue)
		typ     = fun.Type()
		in, out []reflect.Value
		i       int
	)

	if typ.NumIn() > 0 && typ.In(0) == stateType {
		in = make([]reflect.Value, len(args)+1)
		in[0] = reflect.ValueOf(this)
		i++
	} else {
		in = make([]reflect.Value, len(args))
	}

	for _, v := range args {
		in[i] = reflect.ValueOf(v)
		i++
	}

	out = fun.Call(in)

	for i := range result {
		switch out[i].Kind() {
		case reflect.Ptr:
			reflect.ValueOf(result[i]).Set(out[i])
		default:
			reflect.ValueOf(result[i]).Elem().Set(out[i])
		}
	}

	return
}

func (this *State) getFuncRvalue(name string) reflect.Value {
	return this.getFuncValue(name).ContextualValue(this.contextValue)
}

func (this *State) evalFunction(dot reflect.Value, node *parse.IdentifierNode, cmd parse.Node, args []parse.Node, final reflect.Value) reflect.Value {
	this.at(node)
	name := node.Ident
	v := this.getFuncRvalue(name)
	return this.evalCall(dot, v, cmd, name, args, final)
}

// evalField evaluates an expression like (.Field) or (.Field arg1 arg2).
// The 'final' argument represents the return value from the preceding
// value of the pipeline, if any.
func (this *State) evalField(dot reflect.Value, fieldName string, node parse.Node, args []parse.Node, final, receiver reflect.Value) reflect.Value {
	if !receiver.IsValid() {
		if this.tmpl.option.missingKey == mapError { // Treat invalid value as missing map key.
			this.errorf("nil data; no entry for key %q", fieldName)
		}
		return zero
	}

	typ := receiver.Type()

	if i, ok := receiver.Interface().(AttrGetter); ok {
		if val, ok := i.GetAttr(fieldName); ok {
			val := reflect.ValueOf(val)
			if val.Kind() == reflect.Func {
				return this.evalCall(dot, val, node, fieldName, args, final)
			}
			return val
		}
		return reflect.Value{}
	}

	receiver, isNil := indirect(receiver)
	// Unless it's an interface, need to get to a value of type *T to guarantee
	// we see all methods of T and *T.
	ptr := receiver
	if ptr.Kind() != reflect.Interface && ptr.Kind() != reflect.Ptr && ptr.CanAddr() {
		ptr = ptr.Addr()
	}
	if method := ptr.MethodByName(fieldName); method.IsValid() {
		return this.evalCall(dot, method, node, fieldName, args, final)
	}
	hasArgs := len(args) > 1 || final.IsValid()
	// It's not a method; must be a field of a struct or an element of a map.
	switch receiver.Kind() {
	case reflect.Struct:
		tField, ok := receiver.Type().FieldByName(fieldName)
		if ok {
			if isNil {
				this.errorf("nil pointer evaluating %s.%s", typ, fieldName)
			}
			field := receiver.FieldByIndex(tField.Index)
			if tField.PkgPath != "" { // field is unexported
				this.errorf("%s is an unexported field of struct type %s", fieldName, typ)
			}
			// If it's a function, we must call it.
			if hasArgs {
				this.errorf("%s has arguments but cannot be invoked as function", fieldName)
			}
			return field
		} else if f, ok := node.(*parse.FieldNode); ok {
			if !this.e.StateOptions.RequireFields && f.NotRequired {
				return reflect.ValueOf("")
			} else if result, ok := this.e.StateOptions.OnNoField(receiver.Interface(), fieldName); ok {
				return reflect.ValueOf(result)
			}
		}
	case reflect.Map:
		if isNil {
			this.errorf("nil pointer evaluating %s.%s", typ, fieldName)
		}
		// If it's a map, attempt to use the field name as a key.
		nameVal := reflect.ValueOf(fieldName)
		if nameVal.Type().AssignableTo(receiver.Type().Key()) {
			result := receiver.MapIndex(nameVal)
			if !result.IsValid() {
				switch this.tmpl.option.missingKey {
				case mapInvalid:
					// Just use the invalid value.
					if f, ok := node.(*parse.FieldNode); ok {
						if !this.e.StateOptions.RequireFields && f.NotRequired {
							return reflect.ValueOf("")
						} else if result, ok := this.e.StateOptions.OnNoField(receiver.Interface(), fieldName); ok {
							return reflect.ValueOf(result)
						}
					}
				case mapZeroValue:
					result = reflect.Zero(receiver.Type().Elem())
				case mapError:
					this.errorf("map has no entry for key %q", fieldName)
				}
			}
			return result
		}
	}

	if typ.Kind() == reflect.Interface && !isNil && ptr.IsValid() {
		typ = ptr.Type()
	}

	var nils string
	if isNil {
		nils = "(nil)"
	}
	this.errorf("can't evaluate field %s in type %s%s", fieldName, typ, nils)
	panic("not reached")
}

var (
	errorType        = reflect.TypeOf((*error)(nil)).Elem()
	fmtStringerType  = reflect.TypeOf((*fmt.Stringer)(nil)).Elem()
	reflectValueType = reflect.TypeOf((*reflect.Value)(nil)).Elem()
	stateType        = reflect.TypeOf((*State)(nil))
)

// evalCall executes a function or method call. If it's a method, fun already has the receiver bound, so
// it looks just like a function call. The arg list, if non-nil, includes (in the manner of the shell), arg[0]
// as the function itself.
func (this *State) evalCall(dot, fun reflect.Value, node parse.Node, name string, args []parse.Node, final reflect.Value) reflect.Value {
	if args != nil {
		args = args[1:] // Zeroth arg is function name/node; not passed to function.
	}
	typ := fun.Type()
	numIn := len(args)
	if final.IsValid() {
		numIn++
	}
	fNumIn := typ.NumIn()
	stateArg := name == "call" || typ.NumIn() > 0 && typ.In(0) == stateType
	if stateArg {
		fNumIn--
	}
	numFixed := len(args)
	if typ.IsVariadic() {
		numFixed = fNumIn - 1 // last arg is the variadic one.
		if numIn < numFixed {
			this.errorf("wrong number of args for %s: want at least %d got %d", name, typ.NumIn()-1, len(args))
		}
	} else if numIn != fNumIn {
		this.errorf("wrong number of args for %s: want %d got %d", name, typ.NumIn(), len(args))
	}
	if !funcs.GoodFunc(typ) {
		// TODO: This could still be a confusing error; maybe goodFunc should provide info.
		this.errorf("can't call method/function %q with %d results", name, typ.NumOut())
	}
	// Build the arg list.
	argv := make([]reflect.Value, numIn)
	// Args must be evaluated. Fixed args first.
	i, j := 0, 0
	if stateArg {
		j++
	}

	for ; i < numFixed && i < len(args); i++ {
		argv[i] = this.evalArg(dot, typ.In(i+j), args[i])
	}

	// Now the ... args.
	if typ.IsVariadic() {
		argType := typ.In(typ.NumIn() - 1).Elem() // Argument is a slice.
		for ; i < len(args); i++ {
			argv[i] = this.evalArg(dot, argType, args[i])
		}
	}
	// Add final value if necessary.
	if final.IsValid() {
		t := typ.In(typ.NumIn() - 1 + j)
		if typ.IsVariadic() {
			if numIn-1 < numFixed {
				// The added final argument corresponds to a fixed parameter of the function.
				// Validate against the type of the actual parameter.
				t = typ.In(numIn - 1 + j)
			} else {
				// The added final argument corresponds to the variadic part.
				// Validate against the type of the elements of the variadic slice.
				t = t.Elem()
			}
		}
		argv[i] = this.validateType(final, t)
	}
	if fun.IsNil() || !fun.IsValid() {
		this.errorf("error calling %q: %s", name, fun.String())
	}
	if stateArg {
		argv = append([]reflect.Value{reflect.ValueOf(this)}, argv...)
	}
	return this.funCallResult(node, name, fun, argv)
}

func (this *State) funCallResult(node parse.Node, name string, fun reflect.Value, argv []reflect.Value) (v reflect.Value) {
	if name == "" {
		name = "≪anonymous≫"
	}
	result, err := this.funCall(fun, argv)
	if err != nil {
		if IsFatal(err) {
			panic(err)
		}
		this.panic(errors.Wrap(err, fmt.Sprintf("calling %q", name)))
	}

	switch len(result) {
	case 0:
		return blankValue
	case 1:
		if valType := result[0].Type(); valType.Kind() == reflect.Interface && valType.Name() == "error" {
			// If we have an error that is not nil, stop execution and return that error to the caller.
			if !result[0].IsNil() {
				this.at(node)
				this.errorf("error calling %s: %s", name, result[0].Interface().(error))
			}
			return blankValue
		}
	case 2:
		switch result[1].Kind() {
		case reflect.Bool:
			result[0] = reflect.ValueOf(ResultOk{result[0].Interface(), result[1].Bool()})
		default:
			if valType := result[1].Type(); valType.Kind() == reflect.Interface && valType.Name() == "error" {
				// If we have an error that is not nil, stop execution and return that error to the caller.
				if !result[1].IsNil() {
					this.at(node)
					this.errorf("error calling %s: %s", name, result[1].Interface().(error))
				}
			} else {
				return reflect.ValueOf([]any{result[0].Interface(), result[1].Interface()})
			}
		}
	default:
		var l = len(result)
		v = reflect.ValueOf(make([]any, l))

		for i := 0; i < l; i++ {
			v.Index(i).Set(result[i])
		}
		return
	}
	v = result[0]
	if v.Type() == reflectValueType {
		v = v.Interface().(reflect.Value)
	}
	return v
}

func (this *State) funCall(fun reflect.Value, argv []reflect.Value) (r []reflect.Value, err tracederror.TracedError) {
	defer func() {
		if r := recover(); r != nil {
			if r == errExit {
				panic(r)
			}
			switch t := r.(type) {
			case tracederror.TracedError:
				err = t
			case error:
				err = tracederror.New(ExecError{
					Node: this.node,
					Name: this.tmpl.Name(),
					Err:  errors.Wrap(t, this.errorInfo()),
				})
			default:
				err = tracederror.New(ExecError{
					Node: this.node,
					Name: this.tmpl.Name(),
					Err:  errors.Wrap(fmt.Errorf("%#v", t), this.errorInfo()),
				})
			}
		}
	}()
	return fun.Call(argv), nil
}

// canBeNil reports whether an untyped nil can be assigned to the type. See reflect.Zero.
func canBeNil(typ reflect.Type) bool {
	switch typ.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return true
	case reflect.Struct:
		return typ == reflectValueType
	}
	return false
}

// validateType guarantees that the value is valid and assignable to the type.
func (this *State) validateType(value reflect.Value, typ reflect.Type) reflect.Value {
	if !value.IsValid() {
		if typ == nil || canBeNil(typ) {
			// An untyped nil interface{}. Accept as a proper nil value.
			return reflect.Zero(typ)
		}
		this.errorf("invalid value; expected %s", typ)
	}
	if typ == reflectValueType && value.Type() != typ {
		return reflect.ValueOf(value)
	}
	if typ != nil && !value.Type().AssignableTo(typ) {
		if value.Kind() == reflect.Interface && !value.IsNil() {
			value = value.Elem()
			if value.Type().AssignableTo(typ) {
				return value
			}
			// fallthrough
		}
		// Does one dereference or indirection work? We could do more, as we
		// do with method receivers, but that gets messy and method receivers
		// are much more constrained, so it makes more sense there than here.
		// Besides, one is almost always all you need.
		switch {
		case value.Kind() == reflect.Ptr && value.Type().Elem().AssignableTo(typ):
			value = value.Elem()
			if !value.IsValid() {
				this.errorf("dereference of nil pointer of type %s", typ)
			}
		case value.Type().ConvertibleTo(typ):
			value = value.Convert(typ)
		case reflect.PtrTo(value.Type()).AssignableTo(typ) && value.CanAddr():
			value = value.Addr()
		default:
			if typ.Kind() == reflect.Slice && value.Kind() == reflect.Slice {
				if elType, valueElType := typ.Elem(), value.Type().Elem(); valueElType.AssignableTo(elType) {
					newSlice := reflect.MakeSlice(typ, value.Len(), value.Len())
					for i, l := 0, value.Len(); i < l; i++ {
						newSlice.Index(i).Set(value.Index(i))
					}
					return newSlice
				} else if valueElType.ConvertibleTo(elType) {
					newSlice := reflect.MakeSlice(typ, value.Len(), value.Len())
					for i, l := 0, value.Len(); i < l; i++ {
						newSlice.Index(i).Set(value.Index(i).Convert(elType))
					}
					return newSlice
				}
			}
			this.errorf("wrong type for value; expected %s; got %s", typ, value.Type())
		}
	}
	return value
}

func (this *State) evalArg(dot reflect.Value, typ reflect.Type, n parse.Node) reflect.Value {
	this.at(n)
	switch arg := n.(type) {
	case *parse.DotNode:
		return this.validateType(dot, typ)
	case *parse.NilNode:
		if canBeNil(typ) {
			return reflect.Zero(typ)
		}
		this.errorf("cannot assign nil to %s", typ)
	case *parse.FieldNode:
		return this.validateType(this.evalFieldNode(dot, arg, []parse.Node{n}, zero), typ)
	case *parse.VariableNode:
		return this.validateType(this.evalVariableNode(dot, arg, nil, zero), typ)
	case *parse.PipeNode:
		return this.validateType(this.evalPipeline(dot, arg), typ)
	case *parse.IdentifierNode:
		if arg.Ident == Globals {
			return this.dataValue
		}
		if arg.Ident == Self {
			return this.vars[0].value
		}
		return this.validateType(this.evalFunction(dot, arg, arg, nil, zero), typ)
	case *parse.ChainNode:
		return this.validateType(this.evalChainNode(dot, arg, nil, zero), typ)
	case *parse.ValNode:
		return arg.Value
	case *parse.ValFactoryNode:
		return arg.New()
	}
	switch typ.Kind() {
	case reflect.Bool:
		return this.evalBool(typ, n)
	case reflect.Complex64, reflect.Complex128:
		return this.evalComplex(typ, n)
	case reflect.Float32, reflect.Float64:
		return this.evalFloat(typ, n)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return this.evalInteger(typ, n)
	case reflect.Interface:
		if typ.NumMethod() == 0 {
			return this.evalEmptyInterface(dot, n)
		}
	case reflect.Struct:
		if typ == reflectValueType {
			return reflect.ValueOf(this.evalEmptyInterface(dot, n))
		}
	case reflect.String:
		return this.evalString(typ, n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return this.evalUnsignedInteger(typ, n)
	}
	this.errorf("can't handle %s for arg of type %s", n, typ)
	panic("not reached")
}

func (this *State) evalBool(typ reflect.Type, n parse.Node) reflect.Value {
	this.at(n)
	if n, ok := n.(*parse.BoolNode); ok {
		value := reflect.New(typ).Elem()
		value.SetBool(n.True)
		return value
	}
	this.errorf("expected bool; found %s", n)
	panic("not reached")
}

func (this *State) evalString(typ reflect.Type, n parse.Node) reflect.Value {
	this.at(n)
	if n, ok := n.(*parse.StringNode); ok {
		value := reflect.New(typ).Elem()
		value.SetString(n.Text)
		return value
	}
	this.errorf("expected string; found %s", n)
	panic("not reached")
}

func (this *State) evalInteger(typ reflect.Type, n parse.Node) reflect.Value {
	this.at(n)
	if n, ok := n.(*parse.NumberNode); ok && n.IsInt {
		value := reflect.New(typ).Elem()
		value.SetInt(n.Int64)
		return value
	}
	this.errorf("expected integer; found %s", n)
	panic("not reached")
}

func (this *State) evalUnsignedInteger(typ reflect.Type, n parse.Node) reflect.Value {
	this.at(n)
	if n, ok := n.(*parse.NumberNode); ok && n.IsUint {
		value := reflect.New(typ).Elem()
		value.SetUint(n.Uint64)
		return value
	}
	this.errorf("expected unsigned integer; found %s", n)
	panic("not reached")
}

func (this *State) evalFloat(typ reflect.Type, n parse.Node) reflect.Value {
	this.at(n)
	if n, ok := n.(*parse.NumberNode); ok && n.IsFloat {
		value := reflect.New(typ).Elem()
		value.SetFloat(n.Float64)
		return value
	}
	this.errorf("expected float; found %s", n)
	panic("not reached")
}

func (this *State) evalComplex(typ reflect.Type, n parse.Node) reflect.Value {
	if n, ok := n.(*parse.NumberNode); ok && n.IsComplex {
		value := reflect.New(typ).Elem()
		value.SetComplex(n.Complex128)
		return value
	}
	this.errorf("expected complex; found %s", n)
	panic("not reached")
}

func (this *State) evalEmptyInterface(dot reflect.Value, n parse.Node) reflect.Value {
	this.at(n)
	switch n := n.(type) {
	case *parse.BoolNode:
		return reflect.ValueOf(n.True)
	case *parse.DotNode:
		return dot
	case *parse.FieldNode:
		return this.evalFieldNode(dot, n, nil, zero)
	case *parse.IdentifierNode:
		return this.evalFunction(dot, n, n, nil, zero)
	case *parse.NilNode:
		// NilNode is handled in evalArg, the only place that calls here.
		this.errorf("evalEmptyInterface: nil (can't happen)")
	case *parse.NumberNode:
		return this.idealConstant(n)
	case *parse.StringNode:
		return reflect.ValueOf(n.Text)
	case *parse.VariableNode:
		return this.evalVariableNode(dot, n, nil, zero)
	case *parse.PipeNode:
		return this.evalPipeline(dot, n)
	case *parse.ValNode:
		return n.Value
	case *parse.ValFactoryNode:
		return n.New()
	}
	this.errorf("can't handle assignment of %s to empty interface argument", n)
	panic("not reached")
}

// indirect returns the item at the end of indirection, and a bool to indicate if it's nil.
func indirect(v reflect.Value) (rv reflect.Value, isNil bool) {
	for ; v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface; v = v.Elem() {
		if v.IsNil() {
			return v, true
		}
	}
	return v, false
}

// indirectInterface returns the concrete value in an interface value,
// or else the zero reflect.Value.
// That is, if v represents the interface value x, the result is the same as reflect.ValueOf(x):
// the fact that x was an interface value is forgotten.
func indirectInterface(v reflect.Value) reflect.Value {
	if v.Kind() != reflect.Interface {
		return v
	}
	if v.IsNil() {
		return reflect.Value{}
	}
	return v.Elem()
}

// printValue writes the textual representation of the value to the output of
// the template.
func (this *State) printValue(n parse.Node, v reflect.Value) {
	this.at(n)
	if v.IsValid() && v.Type().Kind() == reflect.Func {
		if v = this.funCallResult(n, "", v, nil); v == blankValue {
			return
		}
	}
	iface, ok := printableValue(v)
	if !ok {
		this.errorf("can't print %s of type %s", n, v.Type())
	}
	_, err := fmt.Fprint(this.wr, iface)
	if err != nil {
		this.writeError(err)
	}
}

// trim remove left spaces of value
func (this *State) trim(value reflect.Value, sep ...reflect.Value) reflect.Value {
	f := unicode.IsSpace

	if len(sep) > 0 {
		sepRune := rune(sep[0].String()[0])
		f = func(r rune) bool {
			return r == sepRune
		}
	}

	trim := func(value reflect.Value) reflect.Value {
		stringValue := value.String()
		stringValue = strings.TrimFunc(stringValue, f)
		return reflect.ValueOf(stringValue)
	}

	var trimSlice func(value reflect.Value) reflect.Value

	trimSlice = func(value reflect.Value) reflect.Value {
		var (
			result = reflect.MakeSlice(value.Type().Elem(), value.Len(), value.Len())
			v      reflect.Value
		)
		for l, i := value.Len(), 0; i < l; i++ {
			v = value.Index(i)
			switch v.Kind() {
			case reflect.String:
				result.Index(i).Set(trim(v))
			case reflect.Slice:
				result.Index(i).Set(trimSlice(v))
			default:
				result.Index(i).Set(v)
			}
		}
		return result
	}

	switch value.Kind() {
	case reflect.String:
		return trim(value)
	case reflect.Slice:
		return trimSlice(value)
	default:
		return value
	}
}

// trim remove left spaces of value
func (this *State) join(value reflect.Value, args ...reflect.Value) {
	var (
		sep  = ", "
		and  string
		l, i = value.Len(), 1
	)

	if l == 0 {
		return
	}

	for _, arg := range args {
		switch arg.Kind() {
		case reflect.String:
			parts := strings.SplitN(arg.String(), ":", 2)
			switch len(parts) {
			case 1:
				sep = parts[0]
			case 2:
				key, value := strings.TrimSpace(parts[0]), parts[1]
				switch key {
				case "sep":
					sep = value
				case "and":
					if value == "" {
						and = " and "
					} else {
						and = value
					}
				default:
					this.errorf("invalid join option %q", key)
				}
			}
		}
	}

	if _, err := fmt.Fprint(this.wr, value.Index(0).Interface()); err != nil {
		this.panic(err)
	}

	if and != "" {
		l--
		defer func() {
			if _, err := this.wr.Write([]byte(and)); err != nil {
				this.panic(err)
			}
			if _, err := fmt.Fprint(this.wr, value.Index(i).Interface()); err != nil {
				this.panic(err)
			}
		}()
	}
	for ; i < l; i++ {
		if _, err := this.wr.Write([]byte(sep)); err != nil {
			this.panic(err)
		}
		if _, err := fmt.Fprint(this.wr, value.Index(i).Interface()); err != nil {
			this.panic(err)
		}
	}
}

func (this *State) exp(op rune, a, b reflect.Value) (value reflect.Value) {
	var err error
	if value, err = expr.Expr(op, a, b); err != nil {
		this.errorf(err.Error())
	}
	return
}

// templateExec executes the template and return the result value.
func (this *State) templateExec(name reflect.Value, pipe ...reflect.Value) reflect.Value {
	var (
		result bytes.Buffer
		oldW   = this.wr
	)
	this.wr = &result
	defer func() {
		this.wr = oldW
	}()

	this.templateYield(name, pipe...)

	return reflect.ValueOf(result.String())
}

// templateYield executes the template and writes result into this writer
func (this *State) templateYield(name reflect.Value, pipe ...reflect.Value) {
	this.templateYieldName(name.String(), pipe...)
}

// templateYield executes the template and writes result into this writer
func (this *State) templateYieldName(name string, pipe ...reflect.Value) {
	var data reflect.Value

	if len(pipe) == 1 {
		data = pipe[0]
	}

	tmpl := this.tmpl.tmpl[name]
	if tmpl == nil {
		this.errorf("template %q not defined", name)
	}
	if this.depth == maxExecDepth {
		this.errorf("exceeded maximum template depth (%v)", maxExecDepth)
	}

	executor := tmpl.CreateExecutor()
	executor.noCaptureError = true
	executor.parent = this.e
	executor.StateOptions.Global = append(this.global, this.vars...)
	err := executor.Execute(this.wr, data)
	if err != nil {
		this.panic(ExecError{
			Name: this.tmpl.name + "/" + name,
			Err:  err,
		})
	}
}

// Exec executes the template and return the result value.
func (this *State) Exec(name string, pipe ...interface{}) string {
	var data reflect.Value

	if len(pipe) == 1 {
		switch pt := pipe[0].(type) {
		case reflect.Value:
			data = pt
		case *reflect.Value:
			data = *pt
		default:
			data = reflect.ValueOf(pt)
		}
	}

	tmpl := this.tmpl.tmpl[name]
	if tmpl == nil {
		this.errorf("template %q not defined", name)
	}
	if this.depth == maxExecDepth {
		this.errorf("exceeded maximum template depth (%v)", maxExecDepth)
	}

	executor := tmpl.CreateExecutor()
	executor.noCaptureError = true
	executor.parent = this.e
	result, err := executor.ExecuteString(data)
	if err != nil {
		this.panic(ExecError{
			Name: this.tmpl.name + "/" + name,
			Err:  err,
		})
	}
	return result
}

// printableValue returns the, possibly indirected, interface value inside v that
// is best for a call to formatted printer.
func printableValue(v reflect.Value) (interface{}, bool) {
	if v.Kind() == reflect.Ptr {
		v, _ = indirect(v) // fmt.Fprint handles nil.
	}
	if !v.IsValid() {
		return "<no value>", true
	}

	if !v.Type().Implements(errorType) && !v.Type().Implements(fmtStringerType) {
		if v.CanAddr() && (reflect.PtrTo(v.Type()).Implements(errorType) || reflect.PtrTo(v.Type()).Implements(fmtStringerType)) {
			v = v.Addr()
		} else {
			switch v.Kind() {
			case reflect.Chan, reflect.Func:
				return nil, false
			}
		}
	}
	return v.Interface(), true
}

// Types to help sort the keys in a map for reproducible output.

type rvs []reflect.Value

func (x rvs) Len() int      { return len(x) }
func (x rvs) Swap(i, j int) { x[i], x[j] = x[j], x[i] }

type rvInts struct{ rvs }

func (x rvInts) Less(i, j int) bool { return x.rvs[i].Int() < x.rvs[j].Int() }

type rvUints struct{ rvs }

func (x rvUints) Less(i, j int) bool { return x.rvs[i].Uint() < x.rvs[j].Uint() }

type rvFloats struct{ rvs }

func (x rvFloats) Less(i, j int) bool { return x.rvs[i].Float() < x.rvs[j].Float() }

type rvStrings struct{ rvs }

func (x rvStrings) Less(i, j int) bool { return x.rvs[i].String() < x.rvs[j].String() }

// sortKeys sorts (if it can) the slice of reflect.Values, which is a slice of map keys.
func sortKeys(v []reflect.Value) []reflect.Value {
	if len(v) <= 1 {
		return v
	}
	switch v[0].Kind() {
	case reflect.Float32, reflect.Float64:
		sort.Sort(rvFloats{v})
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		sort.Sort(rvInts{v})
	case reflect.String:
		sort.Sort(rvStrings{v})
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		sort.Sort(rvUints{v})
	}
	return v
}
