package template

import (
	"fmt"

	"github.com/moisespsena/template/funcs"
	"github.com/moisespsena/template/text/template"
)

// builtinsFuncMap maps command names to functions that render their inputs safe.
var builtinsFuncMap = funcs.FuncMap{
	"_html_template_attrescaper":     attrEscaper,
	"_html_template_commentescaper":  commentEscaper,
	"_html_template_cssescaper":      cssEscaper,
	"_html_template_cssvaluefilter":  cssValueFilter,
	"_html_template_htmlnamefilter":  htmlNameFilter,
	"_html_template_htmlescaper":     htmlEscaper,
	"_html_template_jsregexpescaper": jsRegexpEscaper,
	"_html_template_jsstrescaper":    jsStrEscaper,
	"_html_template_jsvalescaper":    jsValEscaper,
	"_html_template_nospaceescaper":  htmlNospaceEscaper,
	"_html_template_rcdataescaper":   rcdataEscaper,
	"_html_template_urlescaper":      urlEscaper,
	"_html_template_urlfilter":       urlFilter,
	"_html_template_urlnormalizer":   urlNormalizer,
	"_eval_args_":                    evalArgs,

	"safe_html": func(v string) HTML {
		return HTML(v)
	},
	"safe_css": func(v string) CSS {
		return CSS(v)
	},
	"safe_js": func(v string) JS {
		return JS(v)
	},
	"safe_raw_js": func(v string) JSStr {
		return JSStr(v)
	},
	"safe_attr": func(v string) HTMLAttr {
		return HTMLAttr(v)
	},
}

var (
	builtins     funcs.FuncValues
	builtinNames []string
)

func init() {
	fcs, err := funcs.CreateValuesFunc(builtinsFuncMap)
	if err != nil {
		panic(err)
	}

	builtins = fcs

	builtinNames = template.BuiltinNames()

	for name := range builtinsFuncMap {
		builtinNames = append(builtinNames, name)
	}
}

func BuiltinNames() []string {
	return builtinNames
}

// evalArgs formats the list of arguments into a string. It is equivalent to
// fmt.Sprint(args...), except that it deferences all pointers.
func evalArgs(args ...interface{}) string {
	// Optimization for simple common case of a single string argument.
	if len(args) == 1 {
		if s, ok := args[0].(string); ok {
			return s
		}
	}
	for i, arg := range args {
		args[i] = indirectToStringerOrError(arg)
	}
	return fmt.Sprint(args...)
}
