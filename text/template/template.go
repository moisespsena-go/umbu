// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package template

import (
	"github.com/moisespsena-go/umbu/funcs"
	"github.com/moisespsena-go/umbu/text/template/parse"
)

// common holds the information shared by related templates.
type common struct {
	tmpl   map[string]*Template // Map from name to defined templates.
	option option
}

// Template is the representation of a parsed template. The *parse.Tree
// field is exported only for use by html/template and should be treated
// as unexported by all other clients.
type Template struct {
	Path string
	name string
	args []string
	*parse.Tree
	*common
	leftDelim  string
	rightDelim string
	funcs      funcs.FuncValues
}

// New allocates a new, undefined template with the given name.
func New(name string, args ...string) *Template {
	t := &Template{
		name: name,
		args: args,
	}
	t.init()
	return t
}

// Name returns the name of the template.
func (t *Template) Name() string {
	return t.name
}

func (t *Template) FullName() string {
	name := t.name
	if t.Path != "" {
		name = t.Path + " [" + name + "]"
	}
	return name
}

func (t *Template) SetPath(path string) *Template {
	t.Path = path
	return t
}

// New allocates a new, undefined template associated with the given one and with the same
// delimiters. The association, which is transitive, allows one template to
// invoke another with a {{template}} action.
func (t *Template) New(name string, args ...string) *Template {
	t.init()
	nt := &Template{
		name:       name,
		common:     t.common,
		leftDelim:  t.leftDelim,
		rightDelim: t.rightDelim,
		args:       args,
	}
	return nt
}

// init guarantees that t has a valid common structure.
func (t *Template) init() {
	if t.common == nil {
		c := new(common)
		c.tmpl = make(map[string]*Template)
		t.common = c
	}
}

// Clone returns a duplicate of the template, including all associated
// templates. The actual representation is not copied, but the name space of
// associated templates is, so further calls to Parse in the copy will add
// templates to the copy but not to the original. Clone can be used to prepare
// common templates and use them with variant definitions for other templates
// by adding the variants after the clone is made.
func (t *Template) Clone() (*Template, error) {
	nt := t.copy(nil)
	nt.init()
	if t.common == nil {
		return nt, nil
	}
	for k, v := range t.tmpl {
		if k == t.name {
			nt.tmpl[t.name] = nt
			continue
		}
		// The associated templates share nt's common structure.
		tmpl := v.copy(nt.common)
		nt.tmpl[k] = tmpl
	}
	return nt, nil
}

// copy returns a shallow copy of t, with common set to the argument.
func (t *Template) copy(c *common) *Template {
	nt := New(t.name)
	nt.Tree = t.Tree
	nt.common = c
	nt.args = t.args
	nt.leftDelim = t.leftDelim
	nt.rightDelim = t.rightDelim
	return nt
}

// AddParseTree adds parse tree for template with given name and associates it with t.
// If the template does not already exist, it will create a new one.
// If the template does exist, it will be replaced.
func (t *Template) AddParseTree(name string, tree *parse.Tree) (*Template, error) {
	t.init()
	// If the name is the name of this template, overwrite this template.
	nt := t
	if name != t.name {
		nt = t.New(name, tree.Args()...)
	}
	// Even if nt == t, we need to install it in the common.tmpl map.
	if replace, err := t.associate(nt, tree); err != nil {
		return nil, err
	} else if replace || nt.Tree == nil {
		nt.Tree = tree
	}
	return nt, nil
}

// Templates returns a slice of defined templates associated with t.
func (t *Template) Templates() []*Template {
	if t.common == nil {
		return nil
	}
	// Return a slice so we don't expose the map.
	m := make([]*Template, 0, len(t.tmpl))
	for _, v := range t.tmpl {
		m = append(m, v)
	}
	return m
}

// GetTemplate returns  defined template by name
func (t *Template) Template(name string) *Template {
	if t.common == nil {
		return nil
	}
	return t.tmpl[name]
}

// Delims sets the action delimiters to the specified strings, to be used in
// subsequent calls to Parse, ParseFiles, or ParseGlob. Nested template
// definitions will inherit the settings. An empty delimiter stands for the
// corresponding default: {{ or }}.
// The return value is the template, so calls can be chained.
func (t *Template) Delims(left, right string) *Template {
	t.init()
	t.leftDelim = left
	t.rightDelim = right
	return t
}

// Lookup returns the template with the given name that is associated with t.
// It returns nil if there is no such template or the template has no definition.
func (t *Template) Lookup(name string) *Template {
	if t.common == nil {
		return nil
	}
	return t.tmpl[name]
}

// Parse parses text as a template body for t.
// Named template definitions ({{define ...}} or {{block ...}} statements) in text
// define additional templates associated with t and are removed from the
// definition of t itself.
//
// Templates can be redefined in successive calls to Parse.
// A template definition with a body containing only white space and comments
// is considered empty and will not replace an existing template's body.
// This allows using Parse to add new named template definitions without
// overwriting the main template body.
func (t *Template) Parse(text string) (*Template, error) {
	t.init()
	trees, err := parse.Parse(t.name, text, t.leftDelim, t.rightDelim)
	if err != nil {
		return nil, err
	}
	// Add the newly parsed trees, including the one for t, into our common structure.
	for name, tree := range trees {
		if _, err := t.AddParseTree(name, tree); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// associate installs the new template into the group of templates associated
// with t. The two are already known to share the common structure.
// The boolean return value reports whether to store this tree as t.Tree.
func (t *Template) associate(new *Template, tree *parse.Tree) (bool, error) {
	if new.common != t.common {
		panic("internal error: associate not common")
	}
	if old := t.tmpl[new.name]; old != nil && parse.IsEmptyTree(tree.Root) && old.Tree != nil {
		// If a template by that name exists,
		// don't replace it with an empty template.
		return false, nil
	}
	t.tmpl[new.name] = new
	return true, nil
}

// Funcs add funcs to this Template
func (t *Template) Funcs(funcMaps ...funcs.FuncMap) *Template {
	if len(funcMaps) > 0 {
		fv, err := funcs.CreateValuesFunc(funcMaps...)
		if err != nil {
			panic(err)
		}
		t.funcs.AppendValues(fv)
	}
	return t
}

// FuncsValues add funcs values to this Template
func (t *Template) FuncsValues(funcValues ...funcs.FuncValues) *Template {
	if len(funcValues) > 0 {
		t.funcs.AppendValues(funcs.NewValues(funcValues...))
	}
	return t
}

// SetFuncs set funcs values to this template
func (t *Template) SetFuncs(values funcs.FuncValues) *Template {
	t.funcs = values
	return t
}

// GetFuncs get all funcs values in this template
func (t *Template) GetFuncs() funcs.FuncValues {
	return t.funcs
}
