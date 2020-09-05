// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package template

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"text/template"
	"time"

	"github.com/vjeantet/jodaTime"

	"github.com/moisespsena/template/funcs"
)

var builtins = funcs.FuncMap{
	"and":      and,
	"call":     call,
	"html":     template.HTMLEscaper,
	"index":    index,
	"js":       template.JSEscaper,
	"len":      length,
	"not":      not,
	"or":       or,
	"print":    fmt.Sprint,
	"printf":   fmt.Sprintf,
	"println":  fmt.Sprintln,
	"urlquery": template.URLQueryEscaper,
	"contains": contains,
	"to_time":  toTime,
	"timef":    timeFormat,
	"default":  defaultValue,

	// Comparisons
	"eq": eq, // ==
	"ge": ge, // >=
	"gt": gt, // >
	"le": le, // <=
	"lt": lt, // <
	"ne": ne, // !=
}

var builtinFuncs funcs.FuncValues

func init() {
	fcs, err := funcs.CreateValuesFunc(builtins)
	if err != nil {
		panic(err)
	}

	builtinFuncs = fcs
}

// prepareArg checks if value can be used as an argument of type argType, and
// converts an invalid value to appropriate zero if possible.
func prepareArg(value reflect.Value, argType reflect.Type) (reflect.Value, error) {
	if !value.IsValid() {
		if !canBeNil(argType) {
			return reflect.Value{}, fmt.Errorf("value is nil; should be of type %s", argType)
		}
		value = reflect.Zero(argType)
	}
	if !value.Type().AssignableTo(argType) {
		return reflect.Value{}, fmt.Errorf("value has type %s; should be %s", value.Type(), argType)
	}
	return value, nil
}

var FALSE = reflect.ValueOf(false)
var TRUE = reflect.ValueOf(true)

func contains(item reflect.Value, sub ...reflect.Value) (reflect.Value, error) {
	v := indirectInterface(item)
	if !v.IsValid() {
		return reflect.Value{}, fmt.Errorf("index of untyped nil")
	}

	switch item.Kind() {
	case reflect.Array, reflect.Slice:
		l := v.Len()
		if l == 0 {
			return FALSE, nil
		}

		for _, i := range sub {
			index := indirectInterface(i)
			var isNil bool
			if v, isNil = indirect(v); isNil {
				return reflect.Value{}, fmt.Errorf("index of nil pointer")
			}

			var tok bool

			for j := 0; j < l; j++ {
				if v.Index(j) == index {
					tok = true
					break
				}
			}

			if !tok {
				return FALSE, nil
			}
		}
	case reflect.String:
		str := v.String()

		for ix, i := range sub {
			index := indirectInterface(i)
			var isNil bool
			if v, isNil = indirect(v); isNil {
				return reflect.Value{}, fmt.Errorf("index of nil pointer")
			}
			if index.Kind() != reflect.String {
				return reflect.Value{}, fmt.Errorf("Arg %v is not a string value.", ix+1)
			}
			if !strings.Contains(str, index.String()) {
				return FALSE, nil
			}
		}
	case reflect.Map:
		l := v.Len()
		if l == 0 {
			return FALSE, nil
		}

		for _, i := range sub {
			index := indirectInterface(i)
			var isNil bool
			if v, isNil = indirect(v); isNil {
				return reflect.Value{}, fmt.Errorf("index of nil pointer")
			}

			if !v.MapIndex(index).IsValid() {
				return FALSE, nil
			}
		}
	}
	return TRUE, nil
}

// Indexing.

// index returns the result of indexing its first argument by the following
// arguments. Thus "index x 1 2 3" is, in Go syntax, x[1][2][3]. Each
// indexed item must be a map, slice, or array.
func index(item reflect.Value, indices ...reflect.Value) (reflect.Value, error) {
	v := indirectInterface(item)
	if !v.IsValid() {
		return reflect.Value{}, fmt.Errorf("index of untyped nil")
	}
	for _, i := range indices {
		index := indirectInterface(i)
		var isNil bool
		if v, isNil = indirect(v); isNil {
			return reflect.Value{}, fmt.Errorf("index of nil pointer")
		}
		switch v.Kind() {
		case reflect.Array, reflect.Slice, reflect.String:
			var x int64
			switch index.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				x = index.Int()
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				x = int64(index.Uint())
			case reflect.Invalid:
				return reflect.Value{}, fmt.Errorf("cannot index slice/array with nil")
			default:
				return reflect.Value{}, fmt.Errorf("cannot index slice/array with type %s", index.Type())
			}
			if x < 0 || x >= int64(v.Len()) {
				return reflect.Value{}, fmt.Errorf("index out of range: %d", x)
			}
			v = v.Index(int(x))
		case reflect.Map:
			index, err := prepareArg(index, v.Type().Key())
			if err != nil {
				return reflect.Value{}, err
			}
			if x := v.MapIndex(index); x.IsValid() {
				v = x
			} else {
				v = reflect.Zero(v.Type().Elem())
			}
		case reflect.Invalid:
			// the loop holds invariant: v.IsValid()
			panic("unreachable")
		default:
			return reflect.Value{}, fmt.Errorf("can't index item of type %s", v.Type())
		}
	}
	return v, nil
}

// Length

// length returns the length of the item, with an error if it has no defined length.
func length(item interface{}) (int, error) {
	v := reflect.ValueOf(item)
	if !v.IsValid() {
		return 0, fmt.Errorf("len of untyped nil")
	}
	v, isNil := indirect(v)
	if isNil {
		return 0, fmt.Errorf("len of nil pointer")
	}
	switch v.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return v.Len(), nil
	}
	return 0, fmt.Errorf("len of type %s", v.Type())
}

// Function invocation

// call returns the result of evaluating the first argument as a function.
// The function must return 1 result, or 2 results, the second of which is an error.
func call(fn reflect.Value, args ...reflect.Value) (reflect.Value, error) {
	v := indirectInterface(fn)
	if !v.IsValid() {
		return reflect.Value{}, fmt.Errorf("call of nil")
	}
	typ := v.Type()
	if typ.Kind() != reflect.Func {
		return reflect.Value{}, fmt.Errorf("non-function of type %s", typ)
	}
	if !funcs.GoodFunc(typ) {
		return reflect.Value{}, fmt.Errorf("function called with %d args; should be 1 or 2", typ.NumOut())
	}
	numIn := typ.NumIn()
	var dddType reflect.Type
	if typ.IsVariadic() {
		if len(args) < numIn-1 {
			return reflect.Value{}, fmt.Errorf("wrong number of args: got %d want at least %d", len(args), numIn-1)
		}
		dddType = typ.In(numIn - 1).Elem()
	} else {
		if len(args) != numIn {
			return reflect.Value{}, fmt.Errorf("wrong number of args: got %d want %d", len(args), numIn)
		}
	}
	argv := make([]reflect.Value, len(args))
	for i, arg := range args {
		value := indirectInterface(arg)
		// Compute the expected type. Clumsy because of variadics.
		var argType reflect.Type
		if !typ.IsVariadic() || i < numIn-1 {
			argType = typ.In(i)
		} else {
			argType = dddType
		}

		var err error
		if argv[i], err = prepareArg(value, argType); err != nil {
			return reflect.Value{}, fmt.Errorf("arg %d: %s", i, err)
		}
	}
	result := v.Call(argv)
	if len(result) == 2 && !result[1].IsNil() {
		return result[0], result[1].Interface().(error)
	}
	return result[0], nil
}

// Boolean logic.

func truth(arg reflect.Value) bool {
	t, _ := isTrue(indirectInterface(arg))
	return t
}

// and computes the Boolean AND of its arguments, returning
// the first false argument it encounters, or the last argument.
func and(arg0 reflect.Value, args ...reflect.Value) reflect.Value {
	if !truth(arg0) {
		return arg0
	}
	for i := range args {
		arg0 = args[i]
		if !truth(arg0) {
			break
		}
	}
	return arg0
}

// or computes the Boolean OR of its arguments, returning
// the first true argument it encounters, or the last argument.
func or(arg0 reflect.Value, args ...reflect.Value) reflect.Value {
	if truth(arg0) {
		return arg0
	}
	for i := range args {
		arg0 = args[i]
		if truth(arg0) {
			break
		}
	}
	return arg0
}

// not returns the Boolean negation of its argument.
func not(arg reflect.Value) bool {
	return !truth(arg)
}

// Comparison.

// TODO: Perhaps allow comparison between signed and unsigned integers.

var (
	errBadComparisonType = errors.New("invalid type for comparison")
	errBadComparison     = errors.New("incompatible types for comparison")
	errNoComparison      = errors.New("missing argument for comparison")
)

type kind int

const (
	invalidKind kind = iota
	boolKind
	complexKind
	intKind
	floatKind
	stringKind
	uintKind
)

func basicKind(v reflect.Value) (kind, error) {
	switch v.Kind() {
	case reflect.Bool:
		return boolKind, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intKind, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return uintKind, nil
	case reflect.Float32, reflect.Float64:
		return floatKind, nil
	case reflect.Complex64, reflect.Complex128:
		return complexKind, nil
	case reflect.String:
		return stringKind, nil
	}
	return invalidKind, errBadComparisonType
}

// eq evaluates the comparison a == b || a == c || ...
func eq(arg1 reflect.Value, arg2 ...reflect.Value) (bool, error) {
	v1 := indirectInterface(arg1)
	k1, err := basicKind(v1)
	if err != nil {
		return false, err
	}
	if len(arg2) == 0 {
		return false, errNoComparison
	}
	for _, arg := range arg2 {
		v2 := indirectInterface(arg)
		k2, err := basicKind(v2)
		if err != nil {
			return false, err
		}
		truth := false
		if k1 != k2 {
			// Special case: Can compare integer values regardless of type's sign.
			switch {
			case k1 == intKind && k2 == uintKind:
				truth = v1.Int() >= 0 && uint64(v1.Int()) == v2.Uint()
			case k1 == uintKind && k2 == intKind:
				truth = v2.Int() >= 0 && v1.Uint() == uint64(v2.Int())
			default:
				return false, errBadComparison
			}
		} else {
			switch k1 {
			case boolKind:
				truth = v1.Bool() == v2.Bool()
			case complexKind:
				truth = v1.Complex() == v2.Complex()
			case floatKind:
				truth = v1.Float() == v2.Float()
			case intKind:
				truth = v1.Int() == v2.Int()
			case stringKind:
				truth = v1.String() == v2.String()
			case uintKind:
				truth = v1.Uint() == v2.Uint()
			default:
				panic("invalid kind")
			}
		}
		if truth {
			return true, nil
		}
	}
	return false, nil
}

// ne evaluates the comparison a != b.
func ne(arg1, arg2 reflect.Value) (bool, error) {
	// != is the inverse of ==.
	equal, err := eq(arg1, arg2)
	return !equal, err
}

// lt evaluates the comparison a < b.
func lt(arg1, arg2 reflect.Value) (bool, error) {
	v1 := indirectInterface(arg1)
	k1, err := basicKind(v1)
	if err != nil {
		return false, err
	}
	v2 := indirectInterface(arg2)
	k2, err := basicKind(v2)
	if err != nil {
		return false, err
	}
	truth := false
	if k1 != k2 {
		// Special case: Can compare integer values regardless of type's sign.
		switch {
		case k1 == intKind && k2 == uintKind:
			truth = v1.Int() < 0 || uint64(v1.Int()) < v2.Uint()
		case k1 == uintKind && k2 == intKind:
			truth = v2.Int() >= 0 && v1.Uint() < uint64(v2.Int())
		default:
			return false, errBadComparison
		}
	} else {
		switch k1 {
		case boolKind, complexKind:
			return false, errBadComparisonType
		case floatKind:
			truth = v1.Float() < v2.Float()
		case intKind:
			truth = v1.Int() < v2.Int()
		case stringKind:
			truth = v1.String() < v2.String()
		case uintKind:
			truth = v1.Uint() < v2.Uint()
		default:
			panic("invalid kind")
		}
	}
	return truth, nil
}

// le evaluates the comparison <= b.
func le(arg1, arg2 reflect.Value) (bool, error) {
	// <= is < or ==.
	lessThan, err := lt(arg1, arg2)
	if lessThan || err != nil {
		return lessThan, err
	}
	return eq(arg1, arg2)
}

// gt evaluates the comparison a > b.
func gt(arg1, arg2 reflect.Value) (bool, error) {
	// > is the inverse of <=.
	lessOrEqual, err := le(arg1, arg2)
	if err != nil {
		return false, err
	}
	return !lessOrEqual, nil
}

// ge evaluates the comparison a >= b.
func ge(arg1, arg2 reflect.Value) (bool, error) {
	// >= is the inverse of <.
	lessThan, err := lt(arg1, arg2)
	if err != nil {
		return false, err
	}
	return !lessThan, nil
}

// toTime parse object as time
func toTime(item interface{}) (t time.Time, err error) {
	v := reflect.ValueOf(item)
	if !v.IsValid() {
		return t, fmt.Errorf("toTime of untyped nil")
	}
	v, isNil := indirect(v)
	if isNil {
		return t, fmt.Errorf("toTime of nil pointer")
	}
	var ok bool
	if t, ok = v.Interface().(time.Time); ok {
		return
	}
	return t, fmt.Errorf("toTime of type %s", v.Type())
}

// timeFormat format time object
func timeFormat(item interface{}, layout string, defaul ...string) (vs string, err error) {
	if len(defaul) > 0 {
		vs = defaul[0]
	}
	var t time.Time
	v := reflect.ValueOf(item)
	if !v.IsValid() {
		return
	}
	v, isNil := indirect(v)
	if isNil {
		return
	}
	var ok bool
	if t, ok = v.Interface().(time.Time); ok {
		vs = jodaTime.Format(layout, t)
		return
	}
	return
}

// defaultValue return first not empty value
func defaultValue(item ...interface{}) interface{} {
	for _, item := range item {
		if truth(reflect.ValueOf(item)) {
			return item
		}
	}
	return nil
}
