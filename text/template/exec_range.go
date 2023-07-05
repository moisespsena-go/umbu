package template

import (
	"reflect"

	"github.com/moisespsena-go/umbu"
	"github.com/moisespsena-go/umbu/text/template/parse"
)

func (this *State) walkRange(dot reflect.Value, r *parse.RangeNode) {
	this.at(r)
	defer this.pop(this.mark())
	val, _ := indirect(this.evalPipeline(dot, r.Pipe))
	// mark top of stack before any variables in the body are pushed.
	mark := this.mark()

	switch len(r.Pipe.Decl) {
	case 0:
		if this.walkRangeDefault(func(elem reflect.Value) {}, mark, val, r) {
			break
		}
	case 1:
		if r.Pipe.Decl[0].Ptr {
			if this.walkRangeWithState(dot, mark, val, r) {
				break
			}
		} else {
			if this.walkRangeDefault(func(elem reflect.Value) {
				// Set top var (lexically the second if there are two) to the element.
				this.setVar(1, elem)
			}, mark, val, r) {
				break
			}
		}
		return
	case 2:
		if this.walkRangeWithArgElemAndIndex(dot, mark, val, r) {
			break
		}
		return
	case 3:
		if this.walkRangeWithArgElemAndIndexAndLast(dot, mark, val, r) {
			break
		}
		return
	}
	if r.ElseList != nil {
		this.walk(dot, r.ElseList)
	}
}

func (this *State) walkRangeDefault(onElem func(elem reflect.Value), mark int, val reflect.Value, r *parse.RangeNode) (empty bool) {
	oneIteration := func(elem reflect.Value) {
		onElem(elem)
		this.walk(elem, r.List)
		this.pop(mark)
	}
	switch val.Kind() {
	case reflect.Array, reflect.Slice:
		if val.Len() == 0 {
			break
		}

		for i, l := 0, val.Len(); i < l; i++ {
			oneIteration(val.Index(i))
		}
		return
	case reflect.Map:
		if val.Len() == 0 {
			break
		}
		for _, key := range sortKeys(val.MapKeys()) {
			oneIteration(val.MapIndex(key))
		}
		return
	case reflect.Chan:
		if val.IsNil() {
			break
		}
		var i int
		for ; ; i++ {
			if elem, ok := val.Recv(); ok {
				oneIteration(elem)
			} else {
				break
			}
		}
		if i == 0 {
			break
		}
		return
	case reflect.Int:
		for i, max := int64(0), val.Int(); i < max; i++ {
			oneIteration(reflect.ValueOf(i))
		}
	case reflect.Invalid:
		break // An invalid value is likely a nil map, etc. and acts like an empty map.
	default:
		this.errorf("range can't iterate over %v %s.%s", val, val.Type().PkgPath(), val.Type().String())
	}

	return true
}

func (this *State) walkRangeWithArgElemAndIndex(dot reflect.Value, mark int, val reflect.Value, r *parse.RangeNode) (empty bool) {
	oneIteration := func(index, elem reflect.Value) {
		// Set top var (lexically the second if there are two) to the element.
		this.setVar(1, elem)
		// Set next var (lexically the first if there are two) to the index.
		this.setVar(2, index)
		this.walk(dot, r.List)
		this.pop(mark)
	}
	switch val.Kind() {
	case reflect.Array, reflect.Slice:
		if val.Len() == 0 {
			break
		}

		for i, l := 0, val.Len(); i < l; i++ {
			oneIteration(reflect.ValueOf(i), val.Index(i))
		}
		return
	case reflect.Map:
		if val.Len() == 0 {
			break
		}
		for _, key := range sortKeys(val.MapKeys()) {
			oneIteration(key, val.MapIndex(key))
		}
		return
	case reflect.Chan:
		if val.IsNil() {
			break
		}
		var i int
		for ; ; i++ {
			if elem, ok := val.Recv(); ok {
				oneIteration(reflect.ValueOf(i), elem)
			} else {
				break
			}
		}
		return
	case reflect.Struct:
		valPtr := val
		if valPtr.CanAddr() {
			valPtr = val.Addr()
		}

		var it umbu.Iterator

		switch t := valPtr.Interface().(type) {
		case umbu.Iterator:
			it = t
		case umbu.IteratorGetter:
			it = t.Iterator()
		}

		if it == nil {
			this.errorf("range can't iterate over %v: %s doesn't implements Iterator", val, val.Type())
		}

		var (
			state = it.Start()
			item  interface{}
		)
		var i int
		for !it.Done(state) {
			item, state = it.Next(state)
			oneIteration(reflect.ValueOf(i), reflect.ValueOf(item))
			i++
		}
		return i == 0
	case reflect.Invalid:
		break // An invalid value is likely a nil map, etc. and acts like an empty map.
	default:
		this.errorf("range can't iterate over %v", val)
	}
	return true
}

func (this *State) walkRangeWithArgElemAndIndexAndLast(dot reflect.Value, mark int, val reflect.Value, r *parse.RangeNode) (empty bool) {
	oneIteration := func(index, elem, isLast reflect.Value) {
		// Set top var (lexically the second if there are two) to the element.
		this.setVar(1, elem)
		// Set next var (lexically the first if there are two) to the index.
		this.setVar(2, index)
		// Set next var (lexically the two if there are three) to the is last.
		this.setVar(3, isLast)
		this.walk(dot, r.List)
		this.pop(mark)
	}
	switch val.Kind() {
	case reflect.Array, reflect.Slice:
		if val.Len() == 0 {
			break
		}

		for i, l := 0, val.Len(); i < l; i++ {
			isLast := i == l-1
			oneIteration(reflect.ValueOf(i), val.Index(i), reflect.ValueOf(isLast))
		}
		return
	case reflect.Map:
		var (
			i int
			l = val.Len()
		)
		if l == 0 {
			break
		}
		for _, key := range sortKeys(val.MapKeys()) {
			oneIteration(key, val.MapIndex(key), reflect.ValueOf(i == l-1))
			i++
		}
		return
	case reflect.Chan:
		if val.IsNil() {
			break
		}
		i := 0
		var next reflect.Value
		elem, ok := val.Recv()
		if !ok {
			break
		}

		for ; ; i++ {
			if next, ok = val.Recv(); ok {
				oneIteration(reflect.ValueOf(i), elem, reflect.ValueOf(false))
				elem = next
			} else {
				break
			}
		}
		oneIteration(reflect.ValueOf(i), elem, reflect.ValueOf(true))
		return
	case reflect.Invalid:
		break // An invalid value is likely a nil map, etc. and acts like an empty map.
	default:
		this.errorf("range can't iterate over %v", val)
	}
	return true
}

func (this *State) walkRangeWithState(dot reflect.Value, mark int, val reflect.Value, r *parse.RangeNode) (empty bool) {
	var state = &RangeElemState{Self: val.Interface()}
	var stateValue = reflect.ValueOf(state)

	oneIteration := func(elem reflect.Value) {
		state.Value = elem.Interface()
		// Set top var (lexically the second if there are two) to the element.
		this.setVar(1, stateValue)
		this.walk(dot, r.List)
		this.pop(mark)
	}
	switch val.Kind() {
	case reflect.Array, reflect.Slice:
		if val.Len() == 0 {
			break
		}

		for i, l := 0, val.Len(); i < l; i++ {
			state.IsLast = i == l-1
			state.IsFirst = i == 0
			state.Index = i
			state.Key = uint64(i)
			oneIteration(val.Index(i))
		}
		return
	case reflect.Map:
		var (
			i int
			l = val.Len()
		)
		if l == 0 {
			break
		}
		for _, key := range sortKeys(val.MapKeys()) {
			state.IsLast = i == l-1
			state.IsFirst = i == 0
			state.Index = i
			state.Key = key
			oneIteration(val.MapIndex(key))
			i++
		}
		return
	case reflect.Chan:
		if val.IsNil() {
			break
		}
		i := 0
		var next reflect.Value
		elem, ok := val.Recv()
		if !ok {
			break
		}

		for ; ; i++ {
			if next, ok = val.Recv(); ok {
				state.IsLast = false
				state.IsFirst = i == 0
				state.Index = i
				state.Key = uint64(i)
				oneIteration(elem)
				elem = next
			} else {
				break
			}
		}
		state.IsLast = true
		state.IsFirst = i == 0
		state.Index = i
		state.Key = state.Index
		oneIteration(elem)
		return
	case reflect.Invalid:
		break // An invalid value is likely a nil map, etc. and acts like an empty map.
	default:
		this.errorf("range can't iterate over %v", val)
	}
	return true
}

type RangeElemState struct {
	Value   interface{}
	Index   int
	Key     interface{}
	IsLast  bool
	IsFirst bool
	Self    interface{}
	Data    interface{}
}
