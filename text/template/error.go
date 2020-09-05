package template

import (
	"github.com/moisespsena-go/tracederror"
	"github.com/moisespsena/template/text/template/parse"
)

func Fatal(err interface{}) *fatal {
	switch t := err.(type) {
	case *fatal:
		return t
	case tracederror.Causer:
		return Fatal(t.Cause())
	default:
		return nil
	}
}

func IsFatal(err error) bool {
	return Fatal(err) != nil
}

type fatal struct {
	cause error
	trace []byte
}

func (this fatal) Error() string {
	return this.cause.Error()
}

func (this fatal) Cause() error {
	return this.cause
}

func (this fatal) Trace() []byte {
	return this.trace
}

// TODO: It would be nice if ExecError was more broken down, but
// the way ErrorContext embeds the template name makes the
// processing too clumsy.

// ExecError is the custom error type returned when Execute has an
// error evaluating its template. (If a write error occurs, the actual
// error is returned; it will not be of type ExecError.)
type ExecError struct {
	Name string      // Name of template.
	Node parse.Node  // the Node
	Err  error       // Pre-formatted error.
	V    interface{} // the Value
}

func (e ExecError) Cause() error {
	return e.Err
}

func (e ExecError) Error() string {
	return e.Err.Error()
}

func (e ExecError) Value() interface{} {
	return e.V
}

func GetExecError(err error) (ee ExecError, ok bool) {
	switch et := err.(type) {
	case ExecError:
		ok = true
		return
	case tracederror.TracedError:
		ee, ok = et.Cause().(ExecError)
	}
	return
}
