package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam/diampb"
)

// avpDef is one dictionary entry: an enum value carrying (diam.def).
type avpDef struct {
	name   string // dictionary value name, e.g. ORIGIN_HOST
	code   uint32
	vendor uint32
	m      bool
	typ    diampb.Type
	file   string // path of the proto file that defines it
}

// label describes the entry in generated comments.
func (d avpDef) label() string {
	s := fmt.Sprintf("%s code %d", d.name, d.code)
	if d.vendor != 0 {
		s += fmt.Sprintf(" vendor %d", d.vendor)
	}
	return s
}

// dict holds every dictionary entry of every file in the request.
type dict struct {
	byName  map[string][]avpDef
	visible map[string]map[string]bool // file path -> itself and its transitive imports
}

func newDict() *dict {
	return &dict{byName: map[string][]avpDef{}, visible: map[string]map[string]bool{}}
}

// scope returns the files whose dictionaries f may use: f and everything it
// imports, directly or not. An entry in a file that merely happens to be
// generated in the same run must not resolve.
func (d *dict) scope(f protoreflect.FileDescriptor) map[string]bool {
	if s, ok := d.visible[f.Path()]; ok {
		return s
	}
	s := map[string]bool{}
	var walk func(protoreflect.FileDescriptor)
	walk = func(fd protoreflect.FileDescriptor) {
		if s[fd.Path()] {
			return
		}
		s[fd.Path()] = true
		imports := fd.Imports()
		for i := 0; i < imports.Len(); i++ {
			walk(imports.Get(i).FileDescriptor)
		}
	}
	walk(f)
	d.visible[f.Path()] = s
	return s
}

// collect adds the entries of the dictionary enums in f.
func (d *dict) collect(f *protogen.File) {
	for _, e := range f.Enums {
		if !proto.GetExtension(e.Desc.Options(), diampb.E_Dictionary).(bool) {
			continue
		}
		for _, v := range e.Values {
			if !proto.HasExtension(v.Desc.Options(), diampb.E_Def) {
				continue
			}
			def := proto.GetExtension(v.Desc.Options(), diampb.E_Def).(*diampb.AVPDef)
			name := string(v.Desc.Name())
			d.byName[name] = append(d.byName[name], avpDef{
				name:   name,
				code:   uint32(v.Desc.Number()),
				vendor: def.GetVendor(),
				m:      def.GetM(),
				typ:    def.GetType(),
				file:   f.Desc.Path(),
			})
		}
	}
}

// lookup resolves a field to its dictionary entry. The entry name is the
// field name in upper case unless (diam.avp) names another. An entry in the
// field's own file wins; otherwise the name must be unique among the files
// that file imports.
func (d *dict) lookup(fld *protogen.Field, f protoreflect.FileDescriptor) (avpDef, error) {
	name := proto.GetExtension(fld.Desc.Options(), diampb.E_Avp).(string)
	if name == "" {
		name = strings.ToUpper(string(fld.Desc.Name()))
	}
	scope := d.scope(f)
	var cands []avpDef
	for _, c := range d.byName[name] {
		if c.file == f.Path() {
			return c, nil
		}
		if scope[c.file] {
			cands = append(cands, c)
		}
	}
	switch len(cands) {
	case 0:
		return avpDef{}, fmt.Errorf("field %s: no dictionary entry %s", fld.Desc.FullName(), name)
	case 1:
		return cands[0], nil
	}
	return avpDef{}, fmt.Errorf("field %s: dictionary entry %s is ambiguous; set (diam.avp)", fld.Desc.FullName(), name)
}
