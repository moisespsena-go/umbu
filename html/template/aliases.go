package template

import (
	"github.com/moisespsena-go/umbu/funcs"
	"github.com/moisespsena-go/umbu/text/template"
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
	WalkHandler     = template.WalkHandler
	RangeElemState  = template.RangeElemState
)

var (
	NewDataFuncs      = funcs.NewDataFuncs
	RangeCallback     = template.RangeCallback
	ExecutorOfRawData = template.ExecutorOfRawData
)
