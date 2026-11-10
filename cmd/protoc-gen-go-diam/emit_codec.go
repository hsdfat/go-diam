package main

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam/diampb"
)

// avpFlags is the flags argument for BeginAVP. The V-bit is added by the
// runtime from the vendor; the P-bit is never set.
func avpFlags(g *protogen.GeneratedFile, fd *field) string {
	if fd.def.m {
		return diamIdent(g, "AVPFlagMandatory")
	}
	return "0"
}

// appendFuncs maps a fixed-width type to its runtime encoder.
var appendFuncs = map[diampb.Type]string{
	diampb.Type_ADDRESS:    "AppendAddress",
	diampb.Type_TIME:       "AppendTime",
	diampb.Type_UNSIGNED32: "AppendUnsigned32",
	diampb.Type_UNSIGNED64: "AppendUnsigned64",
	diampb.Type_INTEGER32:  "AppendInteger32",
	diampb.Type_INTEGER64:  "AppendInteger64",
	diampb.Type_FLOAT32:    "AppendFloat32",
	diampb.Type_FLOAT64:    "AppendFloat64",
}

// decodeFuncs maps a fixed-width type to its runtime decoder.
var decodeFuncs = map[diampb.Type]string{
	diampb.Type_ADDRESS:    "Address",
	diampb.Type_TIME:       "Time",
	diampb.Type_UNSIGNED32: "Unsigned32",
	diampb.Type_UNSIGNED64: "Unsigned64",
	diampb.Type_INTEGER32:  "Integer32",
	diampb.Type_INTEGER64:  "Integer64",
	diampb.Type_FLOAT32:    "Float32",
	diampb.Type_FLOAT64:    "Float64",
	diampb.Type_ENUMERATED: "Integer32",
}

// appendOne emits the encoding of one AVP whose value is the expression v.
// For a grouped AVP v must be a pointer expression.
func appendOne(g *protogen.GeneratedFile, fd *field, v string) {
	d := fd.def
	g.P("b, s = ", diamIdent(g, "BeginAVP"), "(b, ", d.code, ", ", avpFlags(g, fd), ", ", d.vendor, ")")
	switch d.typ {
	case diampb.Type_GROUPED:
		g.P("b = ", v, ".AppendAVPs(b)")
	case diampb.Type_UTF8_STRING, diampb.Type_DIAMETER_IDENTITY, diampb.Type_DIAMETER_URI, diampb.Type_OCTET_STRING:
		g.P("b = append(b, ", v, "...)")
	case diampb.Type_ENUMERATED:
		g.P("b = ", diamIdent(g, "AppendInteger32"), "(b, int32(", v, "))")
	default:
		g.P("b = ", diamIdent(g, appendFuncs[d.typ]), "(b, ", v, ")")
	}
	g.P("b = ", diamIdent(g, "EndAVP"), "(b, s)")
}

// encodeOrder returns the fields with fixed-position AVPs first, each part in
// field order.
func encodeOrder(fields []*field) []*field {
	ordered := append([]*field(nil), fields...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].fixed && !ordered[j].fixed })
	return ordered
}

func genAppendAVPs(g *protogen.GeneratedFile, m *message) {
	g.P("// AppendAVPs appends the AVPs of m: fixed-position AVPs, the others in")
	g.P("// grammar order, then Unknown. Required AVPs are always written.")
	g.P("func (m *", m.name, ") AppendAVPs(b []byte) []byte {")
	if len(m.fields) > 0 {
		g.P("var s int")
	}
	for _, fd := range encodeOrder(m.fields) {
		v := "m." + fd.f.GoName
		switch fd.card {
		case required:
			if fd.grouped() {
				v = "(&" + v + ")"
			}
			appendOne(g, fd, v)
		case optional:
			if fd.grouped() {
				g.P("if ", v, " != nil {")
				appendOne(g, fd, v)
			} else {
				g.P("if ", v, ".Set {")
				appendOne(g, fd, v+".V")
			}
			g.P("}")
		case repeated:
			g.P("for i := range ", v, " {")
			if fd.grouped() {
				appendOne(g, fd, "(&"+v+"[i])")
			} else {
				appendOne(g, fd, v+"[i]")
			}
			g.P("}")
		}
	}
	g.P("for _, a := range m.Unknown {")
	g.P("b = ", diamIdent(g, "AppendRaw"), "(b, a)")
	g.P("}")
	g.P("return b")
	g.P("}")
	g.P()
}

// genExcess emits the bookkeeping for an occurrence beyond the maximum.
func genExcess(g *protogen.GeneratedFile, fd *field) {
	g.P("if m.excess == nil {")
	g.P("m.excess = ", diamIdent(g, "TooMany"), "(", diamIdent(g, "Clone"), "(a), ", fmt.Sprintf("%q", fd.def.name), ")")
	g.P("}")
	g.P("continue")
}

// genDecodeValue emits the decoding of the current AVP `a` and returns the
// expression that holds the decoded value.
func genDecodeValue(g *protogen.GeneratedFile, fd *field) string {
	d := fd.def
	switch d.typ {
	case diampb.Type_GROUPED:
		g.P("var v ", fd.goType)
		g.P("if err := v.DecodeAVPs(a.Data); err != nil {")
		g.P("return err")
		g.P("}")
		return "v"
	case diampb.Type_UTF8_STRING, diampb.Type_DIAMETER_IDENTITY, diampb.Type_DIAMETER_URI:
		return "string(a.Data)"
	case diampb.Type_OCTET_STRING:
		if fd.goType == "string" {
			return "string(a.Data)"
		}
		g.P("v, _ := ", diamIdent(g, "Bytes"), "(a)")
		return "v"
	}
	g.P("v, err := ", diamIdent(g, decodeFuncs[d.typ]), "(a)")
	g.P("if err != nil {")
	g.P("return err")
	g.P("}")
	if d.typ == diampb.Type_ENUMERATED {
		return fd.goType + "(v)"
	}
	return "v"
}

func genDecodeAVPs(g *protogen.GeneratedFile, m *message) {
	g.P("// DecodeAVPs fills m from an AVP sequence. AVPs that the grammar does not")
	g.P("// describe go to Unknown. Occurrences beyond a maximum are dropped and")
	g.P("// reported by Validate.")
	g.P("func (m *", m.name, ") DecodeAVPs(data []byte) error {")
	g.P("m.decoded = true")
	g.P("for len(data) > 0 {")
	g.P("a, rest, err := ", diamIdent(g, "NextAVP"), "(data)")
	g.P("if err != nil {")
	g.P("return err")
	g.P("}")
	g.P("data = rest")
	if len(m.fields) == 0 {
		g.P("m.Unknown = append(m.Unknown, ", diamIdent(g, "Clone"), "(a))")
		g.P("}")
		g.P("return nil")
		g.P("}")
		g.P()
		return
	}
	g.P("// A V-bit with Vendor-Id 0 is not the IETF AVP of that code.")
	g.P("if a.Vendor == 0 && a.Flags&", diamIdent(g, "AVPFlagVendor"), " != 0 {")
	g.P("m.Unknown = append(m.Unknown, ", diamIdent(g, "Clone"), "(a))")
	g.P("continue")
	g.P("}")
	g.P("switch uint64(a.Vendor)<<32 | uint64(a.Code) {")
	for _, fd := range m.fields {
		target := "m." + fd.f.GoName
		g.P("case ", fd.key(), ": // ", fd.def.label())
		switch fd.card {
		case required:
			bit := fmt.Sprintf("uint64(1)<<%d", fd.reqBit)
			g.P("if m.seen&(", bit, ") != 0 {")
			genExcess(g, fd)
			g.P("}")
			val := genDecodeValue(g, fd)
			g.P("m.seen |= ", bit)
			g.P(target, " = ", val)
		case optional:
			present := target + ".Set"
			if fd.grouped() {
				present = target + " != nil"
			}
			g.P("if ", present, " {")
			genExcess(g, fd)
			g.P("}")
			val := genDecodeValue(g, fd)
			if fd.grouped() {
				g.P(target, " = &", val)
			} else {
				g.P(target, " = ", diamIdent(g, "Some"), "(", val, ")")
			}
		case repeated:
			if fd.max > 0 {
				g.P("if len(", target, ") >= ", fd.max, " {")
				genExcess(g, fd)
				g.P("}")
			}
			val := genDecodeValue(g, fd)
			g.P(target, " = append(", target, ", ", val, ")")
		}
	}
	g.P("default:")
	g.P("m.Unknown = append(m.Unknown, ", diamIdent(g, "Clone"), "(a))")
	g.P("}")
	g.P("}")
	g.P("return nil")
	g.P("}")
	g.P()
}
