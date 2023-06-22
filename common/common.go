package common

import (
	"io"

	"github.com/moisespsena-go/umbu/funcs"
)

type TemplateExecutorInterface interface {
	Parent() TemplateExecutorInterface
	Template() TemplateInterface
	Execute(wr io.Writer, data interface{}, funcs ...interface{}) error
}

type TemplateInterface interface {
	Executor(funcMaps ...funcs.FuncMap) TemplateExecutorInterface
	RawText() string
}
