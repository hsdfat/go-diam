package main

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
)

// genValidate emits Validate. The order of the checks is part of the
// contract: a caller answers with the first violation.
//
//  1. the first AVP whose value could not be decoded
//  2. an occurrence beyond a maximum
//  3. a missing required AVP (decoded messages only)
//  4. a repeated AVP under its minimum
//  5. a fixed-position AVP found elsewhere
//  6. violations inside nested groups, with the group's code added to Path
//  7. an unknown AVP with the M-bit
func genValidate(g *protogen.GeneratedFile, m *message) {
	g.P("// Validate checks m against its grammar and returns the first violation")
	g.P("// as a *diam.AVPError. Required presence is checked on decoded messages")
	g.P("// only: a zero value is a valid encoding, so a hand-built message has")
	g.P("// nothing to check.")
	g.P("func (m *", m.name, ") Validate() error {")
	g.P("if m.decodeErr != nil {")
	g.P("return m.decodeErr")
	g.P("}")
	g.P("if m.excess != nil {")
	g.P("return m.excess")
	g.P("}")

	for _, fd := range m.fields {
		if fd.card != required {
			continue
		}
		g.P("if m.decoded && m.seen&(uint64(1)<<", fd.reqBit, ") == 0 {")
		g.P("return ", diamIdent(g, "Missing"), "(", fd.def.code, ", ", fd.def.vendor, ", ", fmt.Sprintf("%q", fd.def.name), ")")
		g.P("}")
	}

	for _, fd := range m.fields {
		if fd.card != repeated || fd.min == 0 {
			continue
		}
		g.P("if len(m.", fd.f.GoName, ") < ", fd.min, " {")
		g.P("return ", diamIdent(g, "TooFew"), "(", fd.def.code, ", ", fd.def.vendor, ", ", fmt.Sprintf("%q", fd.def.name), ", ", fd.min, ")")
		g.P("}")
	}

	if m.hasFixed() {
		g.P("if m.misplaced != nil {")
		g.P("return m.misplaced")
		g.P("}")
	}

	for _, fd := range m.fields {
		if !fd.grouped() {
			continue
		}
		v := "m." + fd.f.GoName
		wrap := func(expr string) {
			g.P("if err := ", expr, ".Validate(); err != nil {")
			g.P("return ", diamIdent(g, "InGroup"), "(err, ", fd.def.code, ")")
			g.P("}")
		}
		switch fd.card {
		case required:
			wrap(v)
		case optional:
			g.P("if ", v, " != nil {")
			wrap(v)
			g.P("}")
		case repeated:
			g.P("for i := range ", v, " {")
			wrap(v + "[i]")
			g.P("}")
		}
	}

	g.P("return ", diamIdent(g, "CheckUnknown"), "(m.Unknown)")
	g.P("}")
	g.P()
}
