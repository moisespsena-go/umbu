package template

import (
	"github.com/moisespsena/template/funcs"
	"github.com/moisespsena/template/text/template"
)

type Executor = template.Executor
type DataFuncs = funcs.DataFuncs
type FuncMap = funcs.FuncMap
type FuncValues = funcs.FuncValues
type ErrorWithTrace = template.ErrorWithTrace
type LocalData = template.LocalData

var NewDataFuncs = funcs.NewDataFuncs
