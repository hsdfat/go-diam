package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

// genDictConsts emits one AVP<Name> code constant per dictionary entry.
func genDictConsts(g *protogen.GeneratedFile, e *protogen.Enum) {
	g.P("// AVP codes defined by this file.")
	g.P("const (")
	for _, v := range e.Values {
		if v.Desc.Number() == 0 {
			continue
		}
		g.P("AVP", camel(string(v.Desc.Name())), " uint32 = ", v.Desc.Number())
	}
	g.P(")")
	g.P()
}

// stripEnumPrefix removes the enum's name from a value name: the shortest
// leading run of words that spells the enum name, ignoring case and
// underscores. Matching on letters rather than on a snake_case rendering of
// the name keeps it right for names with acronyms or digits (CCRequestType
// -> CC_REQUEST_TYPE_, Interworking5gsIndicator -> INTERWORKING5GS_INDICATOR_).
// A value without the prefix is returned unchanged.
func stripEnumPrefix(enum, value string) string {
	for i := 0; i < len(value); i++ {
		if value[i] == '_' && strings.EqualFold(strings.ReplaceAll(value[:i], "_", ""), enum) {
			return value[i+1:]
		}
	}
	return value
}

// genEnum emits a Go type for an Enumerated AVP. The enum's own name is
// stripped from the value names, and a zero value named *_UNSPECIFIED, which
// exists only because proto3 demands a zero, is left out.
func genEnum(g *protogen.GeneratedFile, e *protogen.Enum) {
	name := e.GoIdent.GoName
	type value struct{ goName, text string }
	var values []value

	g.P(e.Comments.Leading, "type ", name, " int32")
	g.P()
	g.P("const (")
	for _, v := range e.Values {
		vn := string(v.Desc.Name())
		if v.Desc.Number() == 0 && strings.HasSuffix(vn, "_UNSPECIFIED") {
			continue
		}
		text := stripEnumPrefix(string(e.Desc.Name()), vn)
		val := value{goName: name + camel(text), text: text}
		values = append(values, val)
		g.P(val.goName, " ", name, " = ", v.Desc.Number())
	}
	g.P(")")
	g.P()
	g.P("// String returns the name of a known value and the number otherwise.")
	g.P("func (v ", name, ") String() string {")
	if len(values) > 0 {
		g.P("switch v {")
		for _, val := range values {
			g.P("case ", val.goName, ":")
			g.P("return ", fmt.Sprintf("%q", val.text))
		}
		g.P("}")
	}
	g.P("return ", g.QualifiedGoIdent(strconvPkg.Ident("Itoa")), "(int(v))")
	g.P("}")
	g.P()
}

// genStruct emits the struct of a command or grouped AVP.
func genStruct(g *protogen.GeneratedFile, m *message) {
	g.P(m.msg.Comments.Leading, "type ", m.name, " struct {")
	if m.cmd != nil {
		g.P("Header ", diamIdent(g, "Header"))
		g.P()
	}
	for _, fd := range m.fields {
		mark := fd.card.mark()
		if fd.fixed {
			mark = "< >"
		}
		g.P(fd.f.GoName, " ", fullType(g, fd), " // ", mark, " ", fd.def.label())
	}
	if len(m.fields) > 0 {
		g.P()
	}
	g.P("// Unknown holds AVPs not described above. They are re-encoded verbatim.")
	g.P("Unknown []", diamIdent(g, "RawAVP"))
	g.P()
	g.P("seen      uint64 // required AVPs found by DecodeAVPs")
	g.P("decodeErr error  // first AVP whose value could not be decoded")
	g.P("excess    error  // first occurrence beyond the grammar maximum")
	if m.hasFixed() {
		g.P("misplaced error  // first fixed-position AVP found elsewhere")
	}
	g.P("decoded   bool   // set by DecodeAVPs; required presence is checked only then")
	g.P("}")
	g.P()
}
