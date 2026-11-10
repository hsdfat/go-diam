package gentest

import (
	"bytes"
	"errors"
	"net/netip"
	"reflect"
	"testing"
	"time"

	"github.com/hsdfat/go-diam/diam"
)

const vendor = 99999

// rawU32 builds an Unsigned32 AVP to inject through Unknown.
func rawU32(code, vnd uint32, flags uint8, v uint32) diam.RawAVP {
	return diam.RawAVP{Code: code, Vendor: vnd, Flags: flags, Data: diam.AppendUnsigned32(nil, v)}
}

// avps splits an AVP sequence.
func avps(t *testing.T, data []byte) []diam.RawAVP {
	t.Helper()
	var out []diam.RawAVP
	for len(data) > 0 {
		a, rest, err := diam.NextAVP(data)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
		data = rest
	}
	return out
}

func body(t *testing.T, msg []byte) []byte {
	t.Helper()
	_, b, err := diam.ParseHeader(msg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func wantResult(t *testing.T, err error, code uint32) *diam.AVPError {
	t.Helper()
	var ae *diam.AVPError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v, want *diam.AVPError with result-code %d", err, code)
	}
	if ae.ResultCode != code {
		t.Fatalf("result-code = %d, want %d (%v)", ae.ResultCode, code, err)
	}
	return ae
}

func sampleAllTypes() AllTypes {
	return AllTypes{
		TOctetString:      []byte{0, 1, 2},
		TInteger32:        -2,
		TInteger64:        -12345,
		TUnsigned32:       7,
		TUnsigned64:       1 << 40,
		TFloat32:          1.5,
		TFloat64:          -1.5,
		TGrouped:          Leaf{LeafValue: 9},
		TAddress:          netip.MustParseAddr("2001:db8::1"),
		TTime:             time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC),
		TUtf8String:       "héllo",
		TDiameterIdentity: "host.example.org",
		TDiameterUri:      "aaa://host.example.org:3868",
		TEnumerated:       ColorRed,
		TText:             "text",
	}
}

func sampleRequest() *TestRequest {
	return &TestRequest{
		Header:    diam.Header{HopByHop: 0x1111, EndToEnd: 0x2222},
		SessionId: "client;1;2",
		AllTypes:  sampleAllTypes(),
		Cards: Cards{
			OptU32:  diam.Some(uint32(3)),
			RepU32:  []uint32{4, 5},
			MinU32:  []uint32{6},
			MaxU32:  []uint32{7, 8},
			OptLeaf: &Leaf{LeafValue: 10},
			RepLeaf: []Leaf{{LeafValue: 11}, {LeafValue: 12}},
		},
	}
}

func TestAllTypesRoundTrip(t *testing.T) {
	in := sampleRequest()
	b1, err := in.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	m, err := diam.Decode(b1)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := m.(*TestRequest)
	if !ok {
		t.Fatalf("Decode returned %T", m)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	want, at := in.AllTypes, got.AllTypes
	if !bytes.Equal(at.TOctetString, want.TOctetString) ||
		at.TInteger32 != want.TInteger32 || at.TInteger64 != want.TInteger64 ||
		at.TUnsigned32 != want.TUnsigned32 || at.TUnsigned64 != want.TUnsigned64 ||
		at.TFloat32 != want.TFloat32 || at.TFloat64 != want.TFloat64 ||
		at.TGrouped.LeafValue != 9 || at.TAddress != want.TAddress || !at.TTime.Equal(want.TTime) ||
		at.TUtf8String != want.TUtf8String || at.TDiameterIdentity != want.TDiameterIdentity ||
		at.TDiameterUri != want.TDiameterUri || at.TEnumerated != ColorRed || at.TText != "text" {
		t.Fatalf("AllTypes = %+v\nwant %+v", at, want)
	}
	c := got.Cards
	if v, ok := c.OptU32.Get(); !ok || v != 3 {
		t.Fatalf("OptU32 = %v", c.OptU32)
	}
	if !reflect.DeepEqual(c.RepU32, []uint32{4, 5}) || !reflect.DeepEqual(c.MinU32, []uint32{6}) || !reflect.DeepEqual(c.MaxU32, []uint32{7, 8}) {
		t.Fatalf("repeated fields = %+v", c)
	}
	if c.OptLeaf == nil || c.OptLeaf.LeafValue != 10 || len(c.RepLeaf) != 2 || c.RepLeaf[1].LeafValue != 12 {
		t.Fatalf("grouped fields = %+v", c)
	}
	if c.OptStr.Set || c.OptBytes.Set {
		t.Fatalf("absent optional fields decoded as present: %+v", c)
	}
	if got.SessionId != "client;1;2" || got.Header.HopByHop != 0x1111 || got.Header.EndToEnd != 0x2222 ||
		got.Header.CommandCode != 1 || got.Header.AppID != vendor || len(got.Unknown) != 0 {
		t.Fatalf("message = %+v", got)
	}

	b2, err := got.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("re-encode differs\n%x\n%x", b1, b2)
	}
}

func TestDecodedDoesNotAliasInput(t *testing.T) {
	b, _ := sampleRequest().Marshal()
	var got TestRequest
	if err := got.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	for i := diam.HeaderLen; i < len(b); i++ {
		b[i] = 0xff
	}
	if !bytes.Equal(got.AllTypes.TOctetString, []byte{0, 1, 2}) || got.AllTypes.TText != "text" || got.SessionId != "client;1;2" {
		t.Fatalf("decoded message changed with the input buffer: %+v", got.AllTypes)
	}
}

func TestZeroRequiredEncoded(t *testing.T) {
	var at AllTypes
	got := avps(t, at.AppendAVPs(nil))
	if len(got) != 15 {
		t.Fatalf("zero AllTypes encodes %d AVPs, want all 15 required ones", len(got))
	}
	for i, a := range got {
		if a.Code != uint32(i+1) {
			t.Fatalf("AVP %d has code %d, want field order", i, a.Code)
		}
	}
}

func decodeCards(t *testing.T, in Cards) *Cards {
	t.Helper()
	var out Cards
	if err := out.DecodeAVPs(in.AppendAVPs(nil)); err != nil {
		t.Fatal(err)
	}
	return &out
}

func TestCardinality(t *testing.T) {
	ok := decodeCards(t, Cards{MinU32: []uint32{1}, MaxU32: []uint32{1, 2}, OptU32: diam.Some(uint32(1))})
	if err := ok.Validate(); err != nil {
		t.Fatalf("within bounds: %v", err)
	}

	tooMany := decodeCards(t, Cards{MinU32: []uint32{1}, MaxU32: []uint32{1, 2},
		Unknown: []diam.RawAVP{rawU32(AVPMaxU32, vendor, diam.AVPFlagMandatory, 3)}})
	ae := wantResult(t, tooMany.Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPMaxU32 || ae.AVP.Vendor != vendor || !bytes.Equal(ae.AVP.Data, []byte{0, 0, 0, 3}) {
		t.Fatalf("max=2 with 3 occurrences: AVP = %+v, want the third occurrence", ae.AVP)
	}
	if len(tooMany.MaxU32) != 2 {
		t.Fatalf("MaxU32 = %v, want the first two occurrences only", tooMany.MaxU32)
	}

	tooFew := decodeCards(t, Cards{})
	ae = wantResult(t, tooFew.Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPMinU32 || ae.AVP.Vendor != vendor {
		t.Fatalf("min=1 with 0 occurrences: AVP = %+v", ae.AVP)
	}

	twice := decodeCards(t, Cards{MinU32: []uint32{1}, OptU32: diam.Some(uint32(1)),
		Unknown: []diam.RawAVP{rawU32(AVPOptU32, vendor, diam.AVPFlagMandatory, 2)}})
	ae = wantResult(t, twice.Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPOptU32 {
		t.Fatalf("optional field twice: AVP = %+v", ae.AVP)
	}
	if v, _ := twice.OptU32.Get(); v != 1 {
		t.Fatalf("OptU32 = %d, want the first occurrence", v)
	}
}

func TestRequiredTwiceAndMissing(t *testing.T) {
	ans := &TestAnswer{SessionId: "s", Unknown: []diam.RawAVP{
		{Code: AVPSessionId, Vendor: vendor, Flags: diam.AVPFlagMandatory, Data: []byte("again")}}}
	b, _ := ans.Marshal()
	var got TestAnswer
	if err := got.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	ae := wantResult(t, got.Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPSessionId || string(ae.AVP.Data) != "again" {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// A message with no AVPs at all: Session-Id is missing.
	empty, _ := diam.FinishMessage(diam.AppendHeader(nil, &diam.Header{Flags: diam.FlagProxiable, CommandCode: 1, AppID: vendor}), 0)
	got = TestAnswer{}
	if err := got.Unmarshal(empty); err != nil {
		t.Fatal(err)
	}
	ae = wantResult(t, got.Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPSessionId {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// Required presence is not checked on a hand-built message.
	if err := new(TestAnswer).Validate(); err != nil {
		t.Fatalf("hand-built message: %v", err)
	}
}

func TestUnmarshalResets(t *testing.T) {
	a := &TestAnswer{SessionId: "a", OptU32: diam.Some(uint32(5)), PlainU32: diam.Some(uint32(1)), Unknown: []diam.RawAVP{
		{Code: 9999, Vendor: 12345, Data: []byte("opaque")},
		{Code: AVPSessionId, Vendor: vendor, Flags: diam.AVPFlagMandatory, Data: []byte("dup")},
	}}
	ba, _ := a.Marshal()
	bb, _ := (&TestAnswer{Header: diam.Header{HopByHop: 3}, SessionId: "b"}).Marshal()

	var reused TestAnswer
	if err := reused.Unmarshal(ba); err != nil {
		t.Fatal(err)
	}
	if reused.Validate() == nil {
		t.Fatal("message A should fail Validate (duplicate Session-Id)")
	}
	if err := reused.Unmarshal(bb); err != nil {
		t.Fatal(err)
	}
	var fresh TestAnswer
	if err := fresh.Unmarshal(bb); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(&reused, &fresh) {
		t.Fatalf("reused struct = %+v\nfresh decode  = %+v", reused, fresh)
	}
	if err := reused.Validate(); err != nil {
		t.Fatalf("stale state after reuse: %v", err)
	}
	if reused.OptU32.Set || reused.PlainU32.Set || len(reused.Unknown) != 0 {
		t.Fatalf("old fields left behind: %+v", reused)
	}
}

func TestVendorMismatchIsUnknown(t *testing.T) {
	// Opt-U32's code under vendor 0 is a different AVP.
	got := decodeCards(t, Cards{MinU32: []uint32{1}, Unknown: []diam.RawAVP{rawU32(AVPOptU32, 0, 0, 5)}})
	if got.OptU32.Set {
		t.Fatal("AVP with vendor 0 decoded into a vendor 99999 field")
	}
	if len(got.Unknown) != 1 || got.Unknown[0].Code != AVPOptU32 || got.Unknown[0].Vendor != 0 {
		t.Fatalf("Unknown = %+v", got.Unknown)
	}

	// Plain-U32 is defined without a vendor. The same code with the V-bit set
	// is not that AVP, even when the Vendor-Id field is 0.
	withV := []byte{0, 0, 0, 60, 0x80, 0, 0, 16, 0, 0, 0, 0, 0, 0, 0, 5}
	msg := diam.AppendHeader(nil, &diam.Header{Flags: diam.FlagProxiable, CommandCode: 1, AppID: vendor})
	sid, s := diam.BeginAVP(nil, AVPSessionId, diam.AVPFlagMandatory, vendor)
	sid = diam.EndAVP(append(sid, 's'), s)
	msg = append(append(msg, sid...), withV...)
	msg, _ = diam.FinishMessage(msg, 0)

	var ans TestAnswer
	if err := ans.Unmarshal(msg); err != nil {
		t.Fatal(err)
	}
	if ans.PlainU32.Set {
		t.Fatal("AVP with the V-bit set decoded into a field defined without a vendor")
	}
	if len(ans.Unknown) != 1 || ans.Unknown[0].Flags&diam.AVPFlagVendor == 0 {
		t.Fatalf("Unknown = %+v", ans.Unknown)
	}
	out, _ := ans.Marshal()
	if !bytes.Equal(out, msg) {
		t.Fatalf("re-encode differs\n%x\n%x", msg, out)
	}
}

func TestEmptyValuesPreserved(t *testing.T) {
	in := &TestRequest{Cards: Cards{}} // everything empty
	b1, _ := in.Marshal()

	top := avps(t, body(t, b1))
	if len(top) != 3 || top[2].Code != AVPCards || len(top[2].Data) != 0 {
		t.Fatalf("empty Cards group not encoded as an empty AVP: %+v", top)
	}

	var got TestRequest
	if err := got.Unmarshal(b1); err != nil {
		t.Fatal(err)
	}
	// The empty group is present: Validate complains about what is inside
	// it, not about a missing Cards AVP.
	ae := wantResult(t, got.Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPMinU32 {
		t.Fatalf("Validate = %v, want the Min-U32 bound inside the empty group", ae)
	}
	if got.AllTypes.TOctetString == nil || len(got.AllTypes.TOctetString) != 0 {
		t.Fatalf("empty OctetString = %#v, want present and empty", got.AllTypes.TOctetString)
	}
	b2, _ := got.Marshal()
	if !bytes.Equal(b1, b2) {
		t.Fatalf("re-encode differs\n%x\n%x", b1, b2)
	}

	// Optional empty string, empty bytes and an empty optional group.
	c := decodeCards(t, Cards{OptStr: diam.Some(""), OptBytes: diam.Some([]byte{}),
		Unknown: []diam.RawAVP{{Code: AVPRepLeaf, Vendor: vendor, Flags: diam.AVPFlagMandatory}}})
	if !c.OptStr.Set || !c.OptBytes.Set || c.OptBytes.V == nil || len(c.RepLeaf) != 1 {
		t.Fatalf("empty optional values lost: %+v", c)
	}
}

func TestUnknownPreserved(t *testing.T) {
	in := &TestAnswer{SessionId: "s", Unknown: []diam.RawAVP{
		{Code: 9999, Vendor: 12345, Data: []byte("opaque")},
		{Code: 9998, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 1}},
	}}
	b1, _ := in.Marshal()
	var got TestAnswer
	if err := got.Unmarshal(b1); err != nil {
		t.Fatal(err)
	}
	if len(got.Unknown) != 2 || string(got.Unknown[0].Data) != "opaque" || got.Unknown[0].Vendor != 12345 {
		t.Fatalf("Unknown = %+v", got.Unknown)
	}
	ae := wantResult(t, got.Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != 9998 {
		t.Fatalf("AVP = %+v, want the M-bit one", ae.AVP)
	}
	b2, _ := got.Marshal()
	if !bytes.Equal(b1, b2) {
		t.Fatal("unknown AVPs not re-encoded verbatim")
	}
}

func TestFlags(t *testing.T) {
	req := sampleRequest()
	req.Header.Flags = diam.FlagError | diam.FlagRetransmit // no R, no P
	b, _ := req.Marshal()
	if got, want := b[4], diam.FlagRequest|diam.FlagProxiable|diam.FlagError|diam.FlagRetransmit; got != want {
		t.Fatalf("request flags = %#x, want %#x", got, want)
	}
	if req.Header.Flags != diam.FlagError|diam.FlagRetransmit {
		t.Fatal("Marshal changed the caller's header")
	}

	ans := &TestAnswer{Header: diam.Header{Flags: diam.FlagRequest | diam.FlagError}, SessionId: "s", PlainU32: diam.Some(uint32(1))}
	ab, _ := ans.Marshal()
	if got, want := ab[4], diam.FlagProxiable|diam.FlagError; got != want {
		t.Fatalf("answer flags = %#x, want %#x", got, want)
	}

	top := avps(t, body(t, ab))
	if len(top) != 2 {
		t.Fatalf("answer has %d AVPs", len(top))
	}
	if top[0].Code != AVPSessionId || top[0].Flags != diam.AVPFlagVendor|diam.AVPFlagMandatory || top[0].Vendor != vendor {
		t.Fatalf("Session-Id AVP = %+v", top[0])
	}
	if top[1].Code != AVPPlainU32 || top[1].Flags != 0 || top[1].Vendor != 0 {
		t.Fatalf("Plain-U32 AVP = %+v, want no flags and no vendor", top[1])
	}

	var walk func(data []byte, grouped map[uint32]bool)
	grouped := map[uint32]bool{AVPAllTypes: true, AVPCards: true, AVPTGrouped: true, AVPOptLeaf: true, AVPRepLeaf: true}
	walk = func(data []byte, grouped map[uint32]bool) {
		for _, a := range avps(t, data) {
			if a.Flags&diam.AVPFlagVendor == 0 || a.Flags&diam.AVPFlagProtected != 0 {
				t.Errorf("AVP %d flags = %#x, want V set and P clear", a.Code, a.Flags)
			}
			if grouped[a.Code] {
				walk(a.Data, grouped)
			}
		}
	}
	walk(body(t, b), grouped)
}

func TestEnumNames(t *testing.T) {
	if ColorRed != 1 {
		t.Fatalf("ColorRed = %d", ColorRed)
	}
	if got := ColorRed.String(); got != "RED" {
		t.Fatalf("ColorRed.String() = %q", got)
	}
	if got := Color(7).String(); got != "7" {
		t.Fatalf("Color(7).String() = %q", got)
	}
	// A value the schema does not know decodes as the raw number.
	at := sampleAllTypes()
	at.TEnumerated = Color(42)
	var got AllTypes
	if err := got.DecodeAVPs(at.AppendAVPs(nil)); err != nil || got.TEnumerated != 42 {
		t.Fatalf("unknown enum value = %d, %v", got.TEnumerated, err)
	}
}

func TestGroupInterfaces(t *testing.T) {
	var _ diam.Message = (*TestRequest)(nil)
	var _ diam.Group = (*TestRequest)(nil)
	var _ diam.Group = (*Cards)(nil)
}
