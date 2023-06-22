package template

import (
	"bytes"
	"io"
)

type WrapWriter interface {
	io.Writer
	BeginHandler() func(w io.Writer)
	io.StringWriter
}

type wrapWriter struct {
	w       io.Writer
	begin   func(w io.Writer)
	noEmpty bool
	strip   bool
	buf     bytes.Buffer
}

func NewWrapWriter(w io.Writer, begin func(w io.Writer), strip bool) *wrapWriter {
	return &wrapWriter{w: w, begin: begin, strip: strip}
}

func (w *wrapWriter) BeginHandler() func(w io.Writer) {
	return w.begin
}

func (w *wrapWriter) WriteString(s string) (n int, err error) {
	return w.Write([]byte(s))
}

func (w *wrapWriter) Write(p []byte) (n int, err error) {
	if n = len(p); n == 0 {
		return
	}

	if w.noEmpty {
		return w.w.Write(p)
	}

	if w.strip {
		var (
			i int
			b byte
		)

	l0:
		for i, b = range p {
			switch b {
			case ' ', '\t', '\r', '\n':
			default:
				i--
				break l0
			}
		}
		p = p[i+1:]

		if len(p) > 0 {
			w.noEmpty = true
			w.begin(w.w)
			_, err = w.w.Write(p)
		}
		return
	} else {
		var (
			i int
			b byte
		)

	l1:
		for i, b = range p {
			switch b {
			case ' ', '\t', '\r', '\n':
			default:
				i--
				break l1
			}
		}

		w.buf.Write(p[0 : i+1])
		p = p[i+1:]

		if len(p) > 0 {
			w.noEmpty = true
			w.begin(w.w)
			if _, err = w.w.Write(append(w.buf.Bytes(), p...)); err != nil {
				return
			}
			w.buf.Reset()
			return
		}
		return
	}
}
