package template

import (
	"io"
)

type beginWriter struct {
	begin   func()
	w       io.Writer
	noEmpty bool
	strip   bool
}

func (b *beginWriter) Write(p []byte) (n int, err error) {
	if !b.noEmpty {
		if b.strip {
			for i, b := range p {
				switch b {
				case ' ', '\t', '\r', '\n':
				default:
					p = p[i:]
					goto done
				}
			}
		done:
			if len(p) == 0 {
				return 0, nil
			}
		}
		b.noEmpty = true
		b.begin()
	}
	return b.w.Write(p)
}
