package template

import (
	"github.com/moisespsena/template/funcs"
	"github.com/moisespsena/template/text/template"
)

type (
	Executor        = template.Executor
	DataFuncs       = funcs.DataFuncs
	FuncMap         = funcs.FuncMap
	FuncMapSlice    = funcs.FuncMapSlice
	FuncValues      = funcs.FuncValues
	FuncValuesSlice = funcs.FuncValuesSlice
	LocalData       = template.LocalData
	State           = template.State
)

var NewDataFuncs = funcs.NewDataFuncs
