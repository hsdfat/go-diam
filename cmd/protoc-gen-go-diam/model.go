package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam/diampb"
)

const diamPkg = protogen.GoImportPath("github.com/hsdfat/go-diam/diam")

var (
	netipPkg   = protogen.GoImportPath("net/netip")
	timePkg    = protogen.GoImportPath("time")
	strconvPkg = protogen.GoImportPath("strconv")
)

// maxRequired is the number of required AVPs one message can track: one bit
// each in a uint64.
const maxRequired = 64

// card is the cardinality of a field in the command or group grammar.
type card int

const (
	required card = iota // { X }
	optional             // [ X ]
	repeated             // *[ X ]
)

// mark is the ABNF bracket shown in generated comments.
func (c card) mark() string {
	switch c {
	case optional:
		return "[ ]"
	case repeated:
		return "*[ ]"
	}
	return "{ }"
}

// field is one AVP of a message grammar.
type field struct {
	f      *protogen.Field
	def    avpDef
	card   card
	min    int    // repeated only
	max    int    // repeated only; 0 = unbounded
	m      bool   // M-bit on send: the dictionary value or the (diam.m) override
	fixed  bool   // < X >
	pos    int    // position among the fixed AVPs, from 0; -1 unless fixed
	goType string // element type, without Opt, pointer or slice
	reqBit int    // bit in the seen mask; -1 unless required
}

func (fd *field) grouped() bool { return fd.def.typ == diampb.Type_GROUPED }

// key identifies the AVP in the decode switch.
func (fd *field) key() uint64 { return uint64(fd.def.vendor)<<32 | uint64(fd.def.code) }

// message is one proto message: a grouped AVP, or a command when cmd is set.
type message struct {
	msg    *protogen.Message
	name   string
	cmd    *diampb.Command
	fields []*field
}

// hasFixed reports whether any AVP of the grammar has a fixed position.
func (m *message) hasFixed() bool {
	for _, fd := range m.fields {
		if fd.fixed {
			return true
		}
	}
	return false
}

// diamIdent returns the qualified name of an identifier in the runtime.
func diamIdent(g *protogen.GeneratedFile, name string) string {
	return g.QualifiedGoIdent(diamPkg.Ident(name))
}

func buildMessage(g *protogen.GeneratedFile, d *dict, f *protogen.File, m *protogen.Message) (*message, error) {
	msg := &message{msg: m, name: m.GoIdent.GoName}
	if proto.HasExtension(m.Desc.Options(), diampb.E_Command) {
		msg.cmd = proto.GetExtension(m.Desc.Options(), diampb.E_Command).(*diampb.Command)
	}
	if len(m.Messages) > 0 || len(m.Enums) > 0 {
		return nil, fmt.Errorf("%s: nested messages, enums and map fields are not supported; declare the type at the top level", m.Desc.FullName())
	}
	if msg.cmd != nil && msg.cmd.GetCode() > maxCommandCode {
		return nil, fmt.Errorf("%s: command code %d does not fit in 24 bits", m.Desc.FullName(), msg.cmd.GetCode())
	}
	seen := map[uint64]string{}
	reqBit, fixedPos := 0, 0
	for _, fl := range m.Fields {
		if err := checkField(msg, fl); err != nil {
			return nil, err
		}
		def, err := d.lookup(fl, f.Desc)
		if err != nil {
			return nil, err
		}
		fd := &field{f: fl, def: def, m: def.m, reqBit: -1, pos: -1}
		if proto.HasExtension(fl.Desc.Options(), diampb.E_M) {
			fd.m = proto.GetExtension(fl.Desc.Options(), diampb.E_M).(bool)
		}
		if fd.fixed = proto.GetExtension(fl.Desc.Options(), diampb.E_Fixed).(bool); fd.fixed {
			fd.pos = fixedPos
			fixedPos++
		}
		switch {
		case fl.Desc.IsList():
			fd.card = repeated
			fd.min = int(proto.GetExtension(fl.Desc.Options(), diampb.E_Min).(uint32))
			fd.max = int(proto.GetExtension(fl.Desc.Options(), diampb.E_Max).(uint32))
		case fl.Desc.HasOptionalKeyword():
			fd.card = optional
		default:
			// Includes message fields without `optional`: proto3 gives them
			// presence, the grammar reads them as required.
			fd.card = required
			fd.reqBit = reqBit
			reqBit++
			if reqBit > maxRequired {
				return nil, fmt.Errorf("%s: more than %d required AVPs", m.Desc.FullName(), maxRequired)
			}
		}
		if other, dup := seen[fd.key()]; dup {
			return nil, fmt.Errorf("%s: fields %s and %s use the same AVP %s", m.Desc.FullName(), other, fl.Desc.Name(), def.label())
		}
		seen[fd.key()] = string(fl.Desc.Name())
		if fd.goType, err = goType(g, fl, def); err != nil {
			return nil, err
		}
		msg.fields = append(msg.fields, fd)
	}
	return msg, nil
}

// maxCommandCode is the largest value of the 24-bit Command Code field.
const maxCommandCode = 0xFFFFFF

// reservedNames are Go names the generated struct already uses. A field with
// one of them would not compile.
var (
	reservedNames        = map[string]bool{"Unknown": true, "Validate": true, "AppendAVPs": true, "DecodeAVPs": true}
	reservedCommandNames = map[string]bool{"Header": true, "AppendMarshal": true, "DiamHeader": true}
)

// checkField rejects the field shapes the schema language has no meaning for,
// so a mistake fails here instead of producing code that does not compile or
// a grammar nobody wrote.
func checkField(msg *message, fl *protogen.Field) error {
	name := fl.Desc.FullName()
	opts := fl.Desc.Options()
	if fl.Oneof != nil && !fl.Oneof.Desc.IsSynthetic() {
		return fmt.Errorf("field %s: oneof is not supported; a Diameter grammar has no alternatives, use optional fields", name)
	}
	if reservedNames[fl.GoName] || (msg.cmd != nil && reservedCommandNames[fl.GoName]) {
		return fmt.Errorf("field %s: the Go name %s is reserved by the generated code; rename the field and set (diam.avp)", name, fl.GoName)
	}
	min := proto.GetExtension(opts, diampb.E_Min).(uint32)
	max := proto.GetExtension(opts, diampb.E_Max).(uint32)
	fixed := proto.GetExtension(opts, diampb.E_Fixed).(bool)
	if !fl.Desc.IsList() && (min != 0 || max != 0) {
		return fmt.Errorf("field %s: (diam.min) and (diam.max) only apply to repeated fields", name)
	}
	if fl.Desc.IsList() && fixed {
		return fmt.Errorf("field %s: (diam.fixed) on a repeated field has no defined position", name)
	}
	if max != 0 && min > max {
		return fmt.Errorf("field %s: (diam.min) is greater than (diam.max)", name)
	}
	return nil
}

// goType returns the Go element type for a field and checks that the proto
// kind agrees with the dictionary type.
func goType(g *protogen.GeneratedFile, fl *protogen.Field, def avpDef) (string, error) {
	k := fl.Desc.Kind()
	want := func(kind protoreflect.Kind, typ string) (string, error) {
		if k != kind {
			return "", fmt.Errorf("field %s: proto kind %s does not fit AVP type %s", fl.Desc.FullName(), k, def.typ)
		}
		return typ, nil
	}
	switch def.typ {
	case diampb.Type_UTF8_STRING, diampb.Type_DIAMETER_IDENTITY, diampb.Type_DIAMETER_URI:
		return want(protoreflect.StringKind, "string")
	case diampb.Type_OCTET_STRING:
		if k == protoreflect.StringKind {
			return "string", nil
		}
		return want(protoreflect.BytesKind, "[]byte")
	case diampb.Type_ADDRESS:
		return want(protoreflect.BytesKind, g.QualifiedGoIdent(netipPkg.Ident("Addr")))
	case diampb.Type_TIME:
		return want(protoreflect.Uint32Kind, g.QualifiedGoIdent(timePkg.Ident("Time")))
	case diampb.Type_UNSIGNED32:
		return want(protoreflect.Uint32Kind, "uint32")
	case diampb.Type_UNSIGNED64:
		return want(protoreflect.Uint64Kind, "uint64")
	case diampb.Type_INTEGER32:
		return want(protoreflect.Int32Kind, "int32")
	case diampb.Type_INTEGER64:
		return want(protoreflect.Int64Kind, "int64")
	case diampb.Type_FLOAT32:
		return want(protoreflect.FloatKind, "float32")
	case diampb.Type_FLOAT64:
		return want(protoreflect.DoubleKind, "float64")
	case diampb.Type_ENUMERATED:
		if k == protoreflect.EnumKind {
			return g.QualifiedGoIdent(fl.Enum.GoIdent), nil
		}
		return want(protoreflect.Int32Kind, "int32")
	case diampb.Type_GROUPED:
		if k != protoreflect.MessageKind {
			return want(protoreflect.MessageKind, "")
		}
		return g.QualifiedGoIdent(fl.Message.GoIdent), nil
	}
	return "", fmt.Errorf("field %s: dictionary entry %s has no type", fl.Desc.FullName(), def.name)
}

// fullType is the Go type of the struct field, with its cardinality.
func fullType(g *protogen.GeneratedFile, fd *field) string {
	switch fd.card {
	case optional:
		if fd.grouped() {
			return "*" + fd.goType
		}
		return diamIdent(g, "Opt") + "[" + fd.goType + "]"
	case repeated:
		return "[]" + fd.goType
	}
	return fd.goType
}

// camel turns SCREAMING_SNAKE into CamelCase.
func camel(s string) string {
	var b strings.Builder
	for _, p := range strings.Split(strings.ToLower(s), "_") {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	return b.String()
}
