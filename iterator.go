package template

import "reflect"

var IteratorType = reflect.TypeOf((*Iterator)(nil)).Elem()

type Iterator interface {
	Start() (state interface{})
	Done(state interface{}) bool
	Next(state interface{}) (item, nextState interface{})
}

type IteratorGetter interface {
	Iterator() Iterator
}
