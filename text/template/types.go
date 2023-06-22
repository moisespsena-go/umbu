package template

import "io"

type WalkHandler func(w io.Writer, dot interface{}, args ...interface{}) (err error)
