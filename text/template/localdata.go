package template

type LocalData map[interface{}]interface{}

func (l *LocalData) Merge(m ...map[interface{}]interface{}) {
	for _, mv := range m {
		for k, v := range mv {
			(*l)[k] = v
		}
	}
}

func (l *LocalData) Set(args ...interface{}) string {
	for i := 0; i < len(args); i += 2 {
		(*l)[args[i]] = args[i+1]
	}
	return ""
}

func (l LocalData) Get(key ...interface{}) interface{} {
	if len(key) == 0 {
		return l
	}
	return l[key[0]]
}

func (l LocalData) Has(key interface{}) bool {
	_, ok := l[key]
	return ok
}
