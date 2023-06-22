package template

type AttrGetter interface {
	GetAttr(name string) (v any, ok bool)
}

type SelfArgCaller interface {
	CallWithSelfArg(self any, args ...any) (ret any, err error)
}
