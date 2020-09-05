package expr

import (
	"fmt"
	"math"
	"reflect"
)

const (
	OpSum   = '+'
	OpSub   = '-'
	OpMulti = '*'
	OpDiv   = '/'
	OpPow   = '^'
	OpMod   = '%'
	OpFloor = '\\'
)

func Expr(op rune, a, b reflect.Value) (v reflect.Value, err error) {
	if !a.IsValid() {
		return b, nil
	}
	at := a.Type()
	switch a.Kind() {
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		a = reflect.ValueOf(a.Uint())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		a = reflect.ValueOf(a.Int())
	case reflect.Float32:
		a = reflect.ValueOf(a.Float())
	}
	switch b.Kind() {
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		b = reflect.ValueOf(b.Uint())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		b = reflect.ValueOf(b.Int())
	case reflect.Float32:
		b = reflect.ValueOf(b.Float())
	}

	switch op {
	case OpSum:
		switch a.Kind() {
		case reflect.Uint64:
			switch b.Kind() {
			case reflect.Uint64:
				a = reflect.ValueOf(a.Uint() + b.Uint())
			case reflect.Int64:
				a = reflect.ValueOf(a.Uint() + uint64(b.Int()))
			case reflect.Float64:
				a = reflect.ValueOf(a.Uint() + uint64(b.Float()))
			default:
				a = reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface()))
			}
		case reflect.Int64:
			switch b.Kind() {
			case reflect.Int64:
				a = reflect.ValueOf(a.Int() + b.Int())
			case reflect.Uint64:
				a = reflect.ValueOf(a.Int() + int64(b.Uint()))
			case reflect.Float64:
				a = reflect.ValueOf(a.Int() + int64(b.Float()))
			default:
				a = reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface()))
			}
		case reflect.Float64:
			switch b.Kind() {
			case reflect.Float64:
				a = reflect.ValueOf(a.Float() + b.Float())
			case reflect.Uint64:
				a = reflect.ValueOf(a.Float() + float64(b.Uint()))
			case reflect.Int64:
				a = reflect.ValueOf(a.Float() + float64(b.Int()))
			default:
				a = reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface()))
			}
		case reflect.Slice:
			et := a.Elem().Type()
			if b.Type().AssignableTo(et) {
				a = reflect.Append(a, b)
			} else if b.Type().ConvertibleTo(et) {
				a = reflect.Append(a, b.Convert(et))
			} else {
				goto bad
			}
		default:
			return reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface())), nil
		}
	case OpSub:
		switch a.Kind() {
		case reflect.Uint64:
			switch b.Kind() {
			case reflect.Uint64:
				a = reflect.ValueOf(a.Uint() - b.Uint())
			case reflect.Int64:
				a = reflect.ValueOf(a.Uint() - uint64(b.Int()))
			case reflect.Float64:
				a = reflect.ValueOf(a.Uint() - uint64(b.Float()))
			default:
				a = reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface()))
			}
		case reflect.Int64:
			switch b.Kind() {
			case reflect.Int64:
				a = reflect.ValueOf(a.Int() - b.Int())
			case reflect.Uint64:
				a = reflect.ValueOf(a.Int() - int64(b.Uint()))
			case reflect.Float64:
				a = reflect.ValueOf(a.Int() - int64(b.Float()))
			default:
				a = reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface()))
			}
		case reflect.Float64:
			switch b.Kind() {
			case reflect.Float64:
				a = reflect.ValueOf(a.Float() - b.Float())
			case reflect.Uint64:
				a = reflect.ValueOf(a.Float() - float64(b.Uint()))
			case reflect.Int64:
				a = reflect.ValueOf(a.Float() - float64(b.Int()))
			default:
				a = reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface()))
			}
		default:
			goto bad
		}
	case OpMulti:
		switch a.Kind() {
		case reflect.Uint64:
			switch b.Kind() {
			case reflect.Uint64:
				a = reflect.ValueOf(a.Uint() * b.Uint())
			case reflect.Int64:
				a = reflect.ValueOf(a.Uint() * uint64(b.Int()))
			case reflect.Float64:
				a = reflect.ValueOf(a.Uint() * uint64(b.Float()))
			default:
				a = reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface()))
			}
		case reflect.Int64:
			switch b.Kind() {
			case reflect.Int64:
				a = reflect.ValueOf(a.Int() * b.Int())
			case reflect.Uint64:
				a = reflect.ValueOf(a.Int() * int64(b.Uint()))
			case reflect.Float64:
				a = reflect.ValueOf(a.Int() * int64(b.Float()))
			default:
				a = reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface()))
			}
		case reflect.Float64:
			switch b.Kind() {
			case reflect.Float64:
				a = reflect.ValueOf(a.Float() * b.Float())
			case reflect.Uint64:
				a = reflect.ValueOf(a.Float() * float64(b.Uint()))
			case reflect.Int64:
				a = reflect.ValueOf(a.Float() * float64(b.Int()))
			default:
				goto bad
			}
		default:
			goto bad
		}
	case OpDiv:
		switch a.Kind() {
		case reflect.Uint64:
			switch b.Kind() {
			case reflect.Uint64:
				a = reflect.ValueOf(a.Uint() / b.Uint())
			case reflect.Int64:
				a = reflect.ValueOf(a.Uint() / uint64(b.Int()))
			case reflect.Float64:
				a = reflect.ValueOf(a.Uint() / uint64(b.Float()))
			default:
				a = reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface()))
			}
		case reflect.Int64:
			switch b.Kind() {
			case reflect.Int64:
				a = reflect.ValueOf(a.Int() / b.Int())
			case reflect.Uint64:
				a = reflect.ValueOf(a.Int() / int64(b.Uint()))
			case reflect.Float64:
				a = reflect.ValueOf(a.Int() / int64(b.Float()))
			default:
				a = reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface()))
			}
		case reflect.Float64:
			switch b.Kind() {
			case reflect.Float64:
				a = reflect.ValueOf(a.Float() / b.Float())
			case reflect.Uint64:
				a = reflect.ValueOf(a.Float() / float64(b.Uint()))
			case reflect.Int64:
				a = reflect.ValueOf(a.Float() / float64(b.Int()))
			default:
				goto bad
			}
		default:
			goto bad
		}
	case OpPow:
		tf64 := reflect.TypeOf(float64(0))
		switch a.Kind() {
		case reflect.Float64:
		case reflect.Uint64, reflect.Int64:
			a = a.Convert(tf64)
		default:
			goto bad
		}
		switch b.Kind() {
		case reflect.Float64:
		case reflect.Uint64, reflect.Int64:
			b = b.Convert(tf64)
		default:
			goto bad
		}
		return reflect.ValueOf(math.Pow(a.Float(), b.Float())).Convert(at), nil
	case OpFloor:
		tf64 := reflect.TypeOf(float64(0))
		switch a.Kind() {
		case reflect.Float64:
		case reflect.Uint64, reflect.Int64:
			a = a.Convert(tf64)
		default:
			goto bad
		}
		switch b.Kind() {
		case reflect.Float64:
		case reflect.Uint64, reflect.Int64:
			b = b.Convert(tf64)
		default:
			goto bad
		}
		return reflect.ValueOf(math.Floor(a.Float() / b.Float())).Convert(at), nil
	case OpMod:
		switch a.Kind() {
		case reflect.Uint64:
			switch b.Kind() {
			case reflect.Uint64:
				a = reflect.ValueOf(a.Uint() % b.Uint())
			case reflect.Int64:
				a = reflect.ValueOf(a.Uint() % uint64(b.Int()))
			case reflect.Float64:
				a = reflect.ValueOf(a.Uint() % uint64(b.Float()))
			default:
				a = reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface()))
			}
		case reflect.Int64:
			switch b.Kind() {
			case reflect.Int64:
				a = reflect.ValueOf(a.Int() % b.Int())
			case reflect.Uint64:
				a = reflect.ValueOf(a.Int() % int64(b.Uint()))
			case reflect.Float64:
				a = reflect.ValueOf(a.Int() % int64(b.Float()))
			default:
				a = reflect.ValueOf(fmt.Sprint(a.Interface()) + fmt.Sprint(b.Interface()))
			}
		default:
			goto bad
		}
	}

	return a.Convert(at), nil
bad:
	err = fmt.Errorf("bad operator %q of types %s and %s", string(op), a.Type(), b.Type())
	return
}
