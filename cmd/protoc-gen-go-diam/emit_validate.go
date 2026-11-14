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
//
// An answer command with the E bit set is checked against the
// answer-message grammar of RFC 6733 §7.2 instead, after steps 1 and 2: see
// genValidateError.
func genValidate(g *protogen.GeneratedFile, m *message) {
	if len(m.fields) == 0 {
		// A grammar with no named AVP, such as Failed-AVP ::= 1* {AVP}, is a
		// container for any AVP: its content is data, not unknown AVPs of
		// this message, so the M-bit rule of CheckUnknown does not apply.
		g.P("// Validate always succeeds: the grammar names no AVP, so every AVP in")
		g.P("// Unknown is content, whatever its M-bit.")
		g.P("func (m *", m.name, ") Validate() error { return nil }")
		g.P()
		return
	}
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
	answer := m.cmd != nil && !m.cmd.GetRequest()
	if answer {
		g.P("if m.Header.Flags&", diamIdent(g, "FlagError"), " != 0 {")
		g.P("return m.validateError()")
		g.P("}")
	}

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

	genValidateGroups(g, m)
	g.P("return ", diamIdent(g, "CheckUnknown"), "(m.Unknown)")
	g.P("}")
	g.P()
	if answer {
		genValidateError(g, m)
	}
}

// genValidateGroups emits the checks of the grouped AVPs present in m, with
// the group's code added to the Path of a violation.
func genValidateGroups(g *protogen.GeneratedFile, m *message) {
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

}

// answerMessageRequired are the AVPs that the answer-message grammar of
// RFC 6733 §7.2 requires, in grammar order.
var answerMessageRequired = []struct {
	code uint32
	name string
}{{264, "ORIGIN_HOST"}, {296, "ORIGIN_REALM"}, {268, "RESULT_CODE"}}

// genValidateError emits validateError, the check of an answer with the E
// bit. RFC 6733 §7.2: such an answer does not follow its command's grammar
// but the answer-message one, so the command's required AVPs are not
// required, Origin-Host, Origin-Realm and Result-Code are (as a field of
// the command or in Unknown), and the AVPs answer-message names are never
// unknown. Positions and nested groups are checked as for the command.
func genValidateError(g *protogen.GeneratedFile, m *message) {
	g.P("// validateError checks an answer with the E bit against the answer-message")
	g.P("// grammar of RFC 6733 §7.2, which it follows instead of its command's.")
	g.P("func (m *", m.name, ") validateError() error {")
	for _, r := range answerMessageRequired {
		present := diamIdent(g, "HasAVP") + fmt.Sprintf("(m.Unknown, %d, 0)", r.code)
		for _, fd := range m.fields {
			if fd.def.code != r.code || fd.def.vendor != 0 {
				continue
			}
			switch {
			case fd.card == required:
				present = fmt.Sprintf("m.seen&(uint64(1)<<%d) != 0", fd.reqBit)
			case fd.card == repeated:
				present = "len(m." + fd.f.GoName + ") > 0"
			case fd.grouped():
				present = "m." + fd.f.GoName + " != nil"
			default:
				present = "m." + fd.f.GoName + ".Set"
			}
		}
		g.P("if m.decoded && !(", present, ") {")
		g.P("return ", diamIdent(g, "Missing"), "(", r.code, ", 0, ", fmt.Sprintf("%q", r.name), ")")
		g.P("}")
	}
	if m.hasFixed() {
		g.P("if m.misplaced != nil {")
		g.P("return m.misplaced")
		g.P("}")
	}
	genValidateGroups(g, m)
	g.P("return ", diamIdent(g, "CheckUnknownError"), "(m.Unknown)")
	g.P("}")
	g.P()
}
