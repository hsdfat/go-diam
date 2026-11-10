package main

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
)

func genValidate(g *protogen.GeneratedFile, m *message) {
	g.P("// Validate checks m against its grammar and returns the first violation")
	g.P("// as a *diam.AVPError. Required presence is checked on decoded messages")
	g.P("// only: a zero value is a valid encoding, so a hand-built message has")
	g.P("// nothing to check.")
	g.P("func (m *", m.name, ") Validate() error {")
	g.P("if m.excess != nil {")
	g.P("return m.excess")
	g.P("}")
	for _, fd := range m.fields {
		d := fd.def
		v := "m." + fd.f.GoName
		name := fmt.Sprintf("%q", d.name)
		switch fd.card {
		case required:
			g.P("if m.decoded && m.seen&(uint64(1)<<", fd.reqBit, ") == 0 {")
			g.P("return ", diamIdent(g, "Missing"), "(", d.code, ", ", d.vendor, ", ", name, ")")
			g.P("}")
			if fd.grouped() {
				g.P("if err := ", v, ".Validate(); err != nil {")
				g.P("return err")
				g.P("}")
			}
		case optional:
			if fd.grouped() {
				g.P("if ", v, " != nil {")
				g.P("if err := ", v, ".Validate(); err != nil {")
				g.P("return err")
				g.P("}")
				g.P("}")
			}
		case repeated:
			if fd.min > 0 {
				g.P("if len(", v, ") < ", fd.min, " {")
				g.P("return ", diamIdent(g, "TooFew"), "(", d.code, ", ", d.vendor, ", ", name, ", ", fd.min, ")")
				g.P("}")
			}
			if fd.grouped() {
				g.P("for i := range ", v, " {")
				g.P("if err := ", v, "[i].Validate(); err != nil {")
				g.P("return err")
				g.P("}")
				g.P("}")
			}
		}
	}
	g.P("return ", diamIdent(g, "CheckUnknown"), "(m.Unknown)")
	g.P("}")
	g.P()
}
