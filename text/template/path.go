package template

import (
	"fmt"
	"strings"
)

type StateLocation struct {
	TemplateName, TemplatePath, Location, Context string
}

type TemplatePath struct {
	pth []StateLocation
}

func (this TemplatePath) Format(f fmt.State, c rune) {
	switch c {
	case 'q', 's':
		f.Write([]byte(this.String()))
	default:
		fmt.Fprint(f, "%v", this.pth)
	}
}

func (this TemplatePath) String() string {
	var quoted = make([]string, len(this.pth))
	for i, p := range this.pth {
		v := p.TemplateName
		if v == "" {
			v = p.TemplatePath
		} else if p.TemplatePath != "" && p.TemplatePath != v {
			v = p.TemplatePath + "[" + v + "]"
		}
		quoted[i] = "`" + doublePercent(v)
		if p.Location != "" {
			quoted[i] += ":" + p.Location + " at <" + doublePercent(p.Context) + ">"
		}
		quoted[i] += "`"
	}
	return strings.Join(quoted, " » ")
}
