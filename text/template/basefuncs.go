package template

type TemplateFuncs struct {
	funcMaps []FuncMap
	funcValues []*FuncValues
}

func (t *TemplateFuncs) AppendFuncs(funcMap ... FuncMap) {
	t.funcMaps = append(t.funcMaps, funcMap...)
}

func (t *TemplateFuncs) AppendFuncValues(funcValues ... *FuncValues) {
	t.funcValues = append(t.funcValues, funcValues...)
}

