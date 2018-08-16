package template

var DefaultFuncMap = map[string]interface{}{
	"dict": func(args ...interface{}) map[interface{}]interface{} {
		dict := make(map[interface{}]interface{})
		for i := 0; i < len(args); i += 2 {
			dict[args[i]] = args[i+1]
		}
		return dict
	},
	"slice": func(args ...interface{}) []interface{} {
		return args
	},
}
