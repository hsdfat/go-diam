package main

import (
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam/diampb"
)

type avpEntry struct {
	name string
	code int32
	typ  diampb.Type
}

// dictionary builds an enum marked (diam.dictionary) with one value per entry.
func dictionary(entries ...avpEntry) *descriptorpb.EnumDescriptorProto {
	opts := &descriptorpb.EnumOptions{}
	proto.SetExtension(opts, diampb.E_Dictionary, true)
	e := &descriptorpb.EnumDescriptorProto{
		Name:    proto.String("Avp"),
		Options: opts,
		Value:   []*descriptorpb.EnumValueDescriptorProto{{Name: proto.String("AVP_UNSPECIFIED"), Number: proto.Int32(0)}},
	}
	for _, en := range entries {
		vo := &descriptorpb.EnumValueOptions{}
		proto.SetExtension(vo, diampb.E_Def, &diampb.AVPDef{M: true, Type: en.typ})
		e.Value = append(e.Value, &descriptorpb.EnumValueDescriptorProto{
			Name: proto.String(en.name), Number: proto.Int32(en.code), Options: vo,
		})
	}
	return e
}

func scalarField(name string, number int32, typ descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:     proto.String(name),
		JsonName: proto.String(name),
		Number:   proto.Int32(number),
		Type:     typ.Enum(),
		Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
	}
}

func file(name, pkg string, deps []string, enums []*descriptorpb.EnumDescriptorProto, msgs ...*descriptorpb.DescriptorProto) *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:        proto.String(name),
		Package:     proto.String(pkg),
		Syntax:      proto.String("proto3"),
		Dependency:  append([]string{"diam/options.proto"}, deps...),
		EnumType:    enums,
		MessageType: msgs,
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("example.test/" + strings.ReplaceAll(pkg, ".", "/")),
		},
	}
}

// generate runs the plugin in-process on files, generating the last one.
func generate(t *testing.T, files ...*descriptorpb.FileDescriptorProto) (*pluginpb.CodeGeneratorResponse, error) {
	t.Helper()
	return generateFiles(t, []string{files[len(files)-1].GetName()}, files...)
}

// generateFiles runs the plugin in-process on files, generating those named.
func generateFiles(t *testing.T, toGenerate []string, files ...*descriptorpb.FileDescriptorProto) (*pluginpb.CodeGeneratorResponse, error) {
	t.Helper()
	req := &pluginpb.CodeGeneratorRequest{
		ProtoFile: []*descriptorpb.FileDescriptorProto{
			protodesc.ToFileDescriptorProto(descriptorpb.File_google_protobuf_descriptor_proto),
			protodesc.ToFileDescriptorProto(diampb.File_diam_options_proto),
		},
		FileToGenerate: toGenerate,
	}
	req.ProtoFile = append(req.ProtoFile, files...)
	gen, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("building the plugin request: %v", err)
	}
	if err := run(gen); err != nil {
		return nil, err
	}
	return gen.Response(), nil
}

func wantError(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want one containing %q", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("error = %q, want it to contain %q", err, substr)
	}
}

func TestGeneratesValidFile(t *testing.T) {
	f := file("ok.proto", "ok", nil,
		[]*descriptorpb.EnumDescriptorProto{dictionary(avpEntry{"NAME", 1, diampb.Type_UTF8_STRING})},
		&descriptorpb.DescriptorProto{
			Name:  proto.String("Thing"),
			Field: []*descriptorpb.FieldDescriptorProto{scalarField("name", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING)},
		})
	resp, err := generate(t, f)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetError() != "" {
		t.Fatal(resp.GetError())
	}
	if len(resp.File) != 1 || !strings.HasSuffix(resp.File[0].GetName(), "ok.diam.go") {
		t.Fatalf("generated files = %v", resp.File)
	}
	src := resp.File[0].GetContent()
	for _, want := range []string{"type Thing struct", "func (m *Thing) AppendAVPs(", "func (m *Thing) DecodeAVPs(", "func (m *Thing) Validate()", "AVPName uint32 = 1"} {
		if !strings.Contains(src, want) {
			t.Errorf("generated code lacks %q", want)
		}
	}
	if resp.GetSupportedFeatures()&uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL) == 0 {
		t.Error("plugin does not declare proto3 optional support")
	}
}

func TestRejectsKindMismatch(t *testing.T) {
	f := file("bad.proto", "bad", nil,
		[]*descriptorpb.EnumDescriptorProto{dictionary(avpEntry{"NAME", 1, diampb.Type_UTF8_STRING})},
		&descriptorpb.DescriptorProto{
			Name:  proto.String("Thing"),
			Field: []*descriptorpb.FieldDescriptorProto{scalarField("name", 1, descriptorpb.FieldDescriptorProto_TYPE_UINT32)},
		})
	_, err := generate(t, f)
	wantError(t, err, "does not fit AVP type UTF8_STRING")
	wantError(t, err, "bad.Thing.name")
}

func TestMissingDictionaryEntry(t *testing.T) {
	f := file("missing.proto", "missing", nil,
		[]*descriptorpb.EnumDescriptorProto{dictionary(avpEntry{"NAME", 1, diampb.Type_UTF8_STRING})},
		&descriptorpb.DescriptorProto{
			Name:  proto.String("Thing"),
			Field: []*descriptorpb.FieldDescriptorProto{scalarField("other", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING)},
		})
	_, err := generate(t, f)
	wantError(t, err, "no dictionary entry OTHER")
}

func TestAmbiguousDictionaryEntry(t *testing.T) {
	d1 := file("d1.proto", "d1", nil, []*descriptorpb.EnumDescriptorProto{dictionary(avpEntry{"NAME", 1, diampb.Type_UTF8_STRING})})
	d2 := file("d2.proto", "d2", nil, []*descriptorpb.EnumDescriptorProto{dictionary(avpEntry{"NAME", 2, diampb.Type_UTF8_STRING})})
	msg := func(f *descriptorpb.FieldDescriptorProto) *descriptorpb.FileDescriptorProto {
		return file("user.proto", "user", []string{"d1.proto", "d2.proto"}, nil,
			&descriptorpb.DescriptorProto{Name: proto.String("Thing"), Field: []*descriptorpb.FieldDescriptorProto{f}})
	}
	_, err := generate(t, d1, d2, msg(scalarField("name", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING)))
	wantError(t, err, "ambiguous; set (diam.avp)")

	// A dictionary entry in the same file wins without (diam.avp).
	same := file("same.proto", "same", []string{"d1.proto"},
		[]*descriptorpb.EnumDescriptorProto{dictionary(avpEntry{"NAME", 3, diampb.Type_UTF8_STRING})},
		&descriptorpb.DescriptorProto{
			Name:  proto.String("Thing"),
			Field: []*descriptorpb.FieldDescriptorProto{scalarField("name", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING)},
		})
	resp, err := generate(t, d1, same)
	if err != nil {
		t.Fatalf("same-file entry: %v", err)
	}
	if !strings.Contains(resp.File[0].GetContent(), "NAME code 3") {
		t.Fatal("same-file dictionary entry was not preferred")
	}
}

func TestTooManyRequired(t *testing.T) {
	build := func(n int) *descriptorpb.FileDescriptorProto {
		var entries []avpEntry
		var fields []*descriptorpb.FieldDescriptorProto
		for i := 1; i <= n; i++ {
			entries = append(entries, avpEntry{fmt.Sprintf("F%d", i), int32(i), diampb.Type_UNSIGNED32})
			fields = append(fields, scalarField(fmt.Sprintf("f%d", i), int32(i), descriptorpb.FieldDescriptorProto_TYPE_UINT32))
		}
		return file("many.proto", "many", nil,
			[]*descriptorpb.EnumDescriptorProto{dictionary(entries...)},
			&descriptorpb.DescriptorProto{Name: proto.String("Thing"), Field: fields})
	}
	if _, err := generate(t, build(64)); err != nil {
		t.Fatalf("64 required fields: %v", err)
	}
	_, err := generate(t, build(65))
	wantError(t, err, "more than 64 required AVPs")
}

func TestDictionaryEntryMustBeImported(t *testing.T) {
	// d1.proto is generated in the same run but user.proto does not import
	// it, so its entries must not resolve: an unrelated file would otherwise
	// supply the AVP code silently.
	d1 := file("d1.proto", "d1", nil, []*descriptorpb.EnumDescriptorProto{dictionary(avpEntry{"NAME", 1, diampb.Type_UTF8_STRING})})
	user := file("user.proto", "user", nil, nil,
		&descriptorpb.DescriptorProto{
			Name:  proto.String("Thing"),
			Field: []*descriptorpb.FieldDescriptorProto{scalarField("name", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING)},
		})
	_, err := generateFiles(t, []string{"d1.proto", "user.proto"}, d1, user)
	wantError(t, err, "no dictionary entry NAME")

	// Reached through a transitive import, it resolves.
	mid := file("mid.proto", "mid", []string{"d1.proto"}, nil)
	user = file("user.proto", "user", []string{"mid.proto"}, nil,
		&descriptorpb.DescriptorProto{
			Name:  proto.String("Thing"),
			Field: []*descriptorpb.FieldDescriptorProto{scalarField("name", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING)},
		})
	if _, err := generate(t, d1, mid, user); err != nil {
		t.Fatalf("transitive import: %v", err)
	}
}

// thing builds a one-message file whose dictionary has an entry per field.
func thing(msg *descriptorpb.DescriptorProto, entries ...avpEntry) *descriptorpb.FileDescriptorProto {
	return file("shape.proto", "shape", nil, []*descriptorpb.EnumDescriptorProto{dictionary(entries...)}, msg)
}

func command(code uint32, request bool) *descriptorpb.MessageOptions {
	opts := &descriptorpb.MessageOptions{}
	proto.SetExtension(opts, diampb.E_Command, &diampb.Command{Code: code, Request: request})
	return opts
}

func TestRejectsOneof(t *testing.T) {
	a := scalarField("a", 1, descriptorpb.FieldDescriptorProto_TYPE_UINT32)
	a.OneofIndex = proto.Int32(0)
	b := scalarField("b", 2, descriptorpb.FieldDescriptorProto_TYPE_UINT32)
	b.OneofIndex = proto.Int32(0)
	msg := &descriptorpb.DescriptorProto{
		Name:      proto.String("Thing"),
		Field:     []*descriptorpb.FieldDescriptorProto{a, b},
		OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: proto.String("x")}},
	}
	_, err := generate(t, thing(msg, avpEntry{"A", 1, diampb.Type_UNSIGNED32}, avpEntry{"B", 2, diampb.Type_UNSIGNED32}))
	wantError(t, err, "oneof")
}

func TestRejectsNestedMessage(t *testing.T) {
	msg := &descriptorpb.DescriptorProto{
		Name:       proto.String("Thing"),
		Field:      []*descriptorpb.FieldDescriptorProto{scalarField("a", 1, descriptorpb.FieldDescriptorProto_TYPE_UINT32)},
		NestedType: []*descriptorpb.DescriptorProto{{Name: proto.String("Inner")}},
	}
	_, err := generate(t, thing(msg, avpEntry{"A", 1, diampb.Type_UNSIGNED32}))
	wantError(t, err, "nested")
}

func TestRejectsDuplicateCommand(t *testing.T) {
	f := file("dup.proto", "dup", nil, nil,
		&descriptorpb.DescriptorProto{Name: proto.String("R1"), Options: command(9, true)},
		&descriptorpb.DescriptorProto{Name: proto.String("R2"), Options: command(9, true)})
	_, err := generate(t, f)
	wantError(t, err, "same command")

	// A request and its answer share the code and are fine.
	ok := file("pair.proto", "pair", nil, nil,
		&descriptorpb.DescriptorProto{Name: proto.String("R"), Options: command(9, true)},
		&descriptorpb.DescriptorProto{Name: proto.String("A"), Options: command(9, false)})
	if _, err := generate(t, ok); err != nil {
		t.Fatalf("request and answer: %v", err)
	}

	big := file("big.proto", "big", nil, nil,
		&descriptorpb.DescriptorProto{Name: proto.String("R"), Options: command(1<<24, true)})
	_, err = generate(t, big)
	wantError(t, err, "24 bits")
}

func TestRejectsReservedFieldNames(t *testing.T) {
	for _, name := range []string{"unknown", "validate"} {
		msg := &descriptorpb.DescriptorProto{
			Name:  proto.String("Thing"),
			Field: []*descriptorpb.FieldDescriptorProto{scalarField(name, 1, descriptorpb.FieldDescriptorProto_TYPE_UINT32)},
		}
		_, err := generate(t, thing(msg, avpEntry{strings.ToUpper(name), 1, diampb.Type_UNSIGNED32}))
		wantError(t, err, "reserved")
	}
	// Header and the command methods only clash in a command. (protogen
	// itself renames fields called marshal or unmarshal to Marshal_.)
	for _, name := range []string{"header", "append_marshal", "diam_header"} {
		fld := scalarField(name, 1, descriptorpb.FieldDescriptorProto_TYPE_UINT32)
		entry := avpEntry{strings.ToUpper(name), 1, diampb.Type_UNSIGNED32}
		cmd := &descriptorpb.DescriptorProto{Name: proto.String("Thing"), Field: []*descriptorpb.FieldDescriptorProto{fld}, Options: command(9, true)}
		_, err := generate(t, thing(cmd, entry))
		if err == nil {
			t.Fatalf("command field %s: no error", name)
		}
		wantError(t, err, "reserved")
		group := &descriptorpb.DescriptorProto{Name: proto.String("Thing"), Field: []*descriptorpb.FieldDescriptorProto{fld}}
		if _, err := generate(t, thing(group, entry)); err != nil {
			t.Fatalf("field %s in a grouped AVP: %v", name, err)
		}
	}
}

func TestRejectsMisplacedFieldOptions(t *testing.T) {
	entry := avpEntry{"A", 1, diampb.Type_UNSIGNED32}
	withOpts := func(label descriptorpb.FieldDescriptorProto_Label, set func(*descriptorpb.FieldOptions)) *descriptorpb.FileDescriptorProto {
		fld := scalarField("a", 1, descriptorpb.FieldDescriptorProto_TYPE_UINT32)
		fld.Label = label.Enum()
		fld.Options = &descriptorpb.FieldOptions{}
		set(fld.Options)
		return thing(&descriptorpb.DescriptorProto{Name: proto.String("Thing"), Field: []*descriptorpb.FieldDescriptorProto{fld}}, entry)
	}
	rep, opt := descriptorpb.FieldDescriptorProto_LABEL_REPEATED, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL

	_, err := generate(t, withOpts(rep, func(o *descriptorpb.FieldOptions) { proto.SetExtension(o, diampb.E_Fixed, true) }))
	wantError(t, err, "(diam.fixed) on a repeated field")
	_, err = generate(t, withOpts(opt, func(o *descriptorpb.FieldOptions) { proto.SetExtension(o, diampb.E_Min, uint32(1)) }))
	wantError(t, err, "only apply to repeated fields")
	_, err = generate(t, withOpts(opt, func(o *descriptorpb.FieldOptions) { proto.SetExtension(o, diampb.E_Max, uint32(2)) }))
	wantError(t, err, "only apply to repeated fields")
	_, err = generate(t, withOpts(rep, func(o *descriptorpb.FieldOptions) {
		proto.SetExtension(o, diampb.E_Min, uint32(3))
		proto.SetExtension(o, diampb.E_Max, uint32(2))
	}))
	wantError(t, err, "(diam.min) is greater than (diam.max)")
}
