package gentest

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
)

// avp encodes one AVP under the fixture vendor with the M-bit.
func avp(code uint32, data []byte) []byte {
	b, s := diam.BeginAVP(nil, code, diam.AVPFlagMandatory, vendor)
	return diam.EndAVP(append(b, data...), s)
}

// message frames AVP sections into a message with the given command header.
func message(h diam.Header, sections ...[]byte) []byte {
	b := diam.AppendHeader(nil, &h)
	for _, s := range sections {
		b = append(b, s...)
	}
	b, _ = diam.FinishMessage(b, 0)
	return b
}

var (
	testRequestHeader   = diam.Header{Flags: diam.FlagRequest | diam.FlagProxiable, CommandCode: 1, AppID: vendor}
	testAnswerHeader    = diam.Header{Flags: diam.FlagProxiable, CommandCode: 1, AppID: vendor}
	nestedRequestHeader = diam.Header{Flags: diam.FlagRequest, CommandCode: 2, AppID: vendor}
)

func TestBadValueInsideGroupDoesNotAbort(t *testing.T) {
	// Re-encode a valid All-Types group with a 3-byte Unsigned32.
	at := sampleAllTypes()
	var group []byte
	for _, a := range avps(t, at.AppendAVPs(nil)) {
		if a.Code == AVPTUnsigned32 {
			a.Data = []byte{1, 2, 3}
		}
		group = diam.AppendRaw(group, a)
	}
	cards := (&Cards{MinU32: []uint32{1}}).AppendAVPs(nil)
	msg := message(testRequestHeader, avp(AVPSessionId, []byte("s")), avp(AVPAllTypes, group), avp(AVPCards, cards))

	var got TestRequest
	if err := got.Unmarshal(msg); err != nil {
		t.Fatalf("Unmarshal = %v, want nil: a bad value inside a group is a Validate matter", err)
	}
	if got.AllTypes.TUnsigned64 != 1<<40 || got.AllTypes.TText != "text" || !reflect.DeepEqual(got.Cards.MinU32, []uint32{1}) {
		t.Fatalf("AVPs after the bad one were not decoded: %+v", got)
	}
	ae := wantResult(t, got.Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPTUnsigned32 || !bytes.Equal(ae.AVP.Data, []byte{1, 2, 3}) {
		t.Fatalf("AVP = %+v, want the Unsigned32 with its 3 bytes", ae.AVP)
	}
	if !reflect.DeepEqual(ae.Path, []uint32{AVPAllTypes}) {
		t.Fatalf("Path = %v, want [%d]", ae.Path, AVPAllTypes)
	}
}

func TestBadValueAtTopLevel(t *testing.T) {
	msg := message(testAnswerHeader, avp(AVPSessionId, []byte("s")), avp(AVPOptU32, []byte{1, 2, 3, 4, 5}))
	var got TestAnswer
	if err := got.Unmarshal(msg); err != nil {
		t.Fatal(err)
	}
	if got.OptU32.Set {
		t.Fatal("a field with a bad value must stay unset")
	}
	ae := wantResult(t, got.Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPOptU32 || len(ae.Path) != 0 {
		t.Fatalf("error = %+v", ae)
	}
}

func TestGroupedOverrun(t *testing.T) {
	// An inner AVP that claims 64 bytes inside a 12-byte group.
	overrun := []byte{0, 0, 0, 30, 0xc0, 0, 0, 64, 0, 1, 0x86, 0x9f}
	at := sampleAllTypes()
	msg := message(testRequestHeader, avp(AVPSessionId, []byte("s")), avp(AVPAllTypes, at.AppendAVPs(nil)), avp(AVPCards, overrun))

	var got TestRequest
	if err := got.Unmarshal(msg); err != nil {
		t.Fatalf("Unmarshal = %v, want nil", err)
	}
	ae := wantResult(t, got.Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPCards || ae.AVP.Vendor != vendor || !bytes.Equal(ae.AVP.Data, overrun) {
		t.Fatalf("AVP = %+v, want the Cards grouped AVP with its data", ae.AVP)
	}
	if len(ae.Path) != 0 {
		t.Fatalf("Path = %v, want none: Cards is at the top level", ae.Path)
	}

	// The same one level down: the broken group is Opt-Leaf inside Cards.
	cards := append((&Cards{MinU32: []uint32{1}}).AppendAVPs(nil), avp(AVPOptLeaf, []byte{0, 0, 0, 50, 0xc0, 0, 0, 64})...)
	msg = message(testRequestHeader, avp(AVPSessionId, []byte("s")), avp(AVPAllTypes, at.AppendAVPs(nil)), avp(AVPCards, cards))
	got = TestRequest{}
	if err := got.Unmarshal(msg); err != nil {
		t.Fatal(err)
	}
	ae = wantResult(t, got.Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPOptLeaf || !reflect.DeepEqual(ae.Path, []uint32{AVPCards}) {
		t.Fatalf("error = %+v, want Opt-Leaf in group Cards", ae)
	}
}

func TestTopLevelOverrunFailsUnmarshal(t *testing.T) {
	// The AVP section of the message itself cannot be split.
	msg := message(testAnswerHeader, []byte{0, 0, 0, 40, 0xc0, 0, 0, 64, 0, 1, 0x86, 0x9f})
	var got TestAnswer
	wantResult(t, got.Unmarshal(msg), diam.ResultInvalidAVPLength)
}

func TestNestedPathTwoLevels(t *testing.T) {
	in := &NestedRequest{NestedSession: "s", Outer: Outer{Middle: Middle{
		InnerU32: diam.Some(uint32(1)),
		Unknown:  []diam.RawAVP{rawU32(AVPInnerU32, vendor, diam.AVPFlagMandatory, 2)},
	}}}
	b, _ := in.Marshal()
	m, err := diam.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	ae := wantResult(t, m.Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPInnerU32 || !bytes.Equal(ae.AVP.Data, []byte{0, 0, 0, 2}) {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	if !reflect.DeepEqual(ae.Path, []uint32{AVPOuter, AVPMiddle}) {
		t.Fatalf("Path = %v, want [%d %d]", ae.Path, AVPOuter, AVPMiddle)
	}

	// A type from another file nests the same way.
	in = &NestedRequest{NestedSession: "s", Outer: Outer{Middle: Middle{OptLeaf: &Leaf{LeafValue: 3}}}}
	b, _ = in.Marshal()
	var got NestedRequest
	if err := got.Unmarshal(b); err != nil || got.Validate() != nil {
		t.Fatalf("valid nested message: %v / %v", err, got.Validate())
	}
	if got.Outer.Middle.OptLeaf == nil || got.Outer.Middle.OptLeaf.LeafValue != 3 {
		t.Fatalf("decoded = %+v", got.Outer.Middle)
	}
}

func TestSessionIdNotFirst(t *testing.T) {
	sid := avp(AVPSessionId, []byte("s"))
	opt := avp(AVPOptU32, []byte{0, 0, 0, 1})
	plain := diam.AppendRaw(nil, diam.RawAVP{Code: AVPPlainU32, Data: []byte{0, 0, 0, 2}})

	var got TestAnswer
	if err := got.Unmarshal(message(testAnswerHeader, sid, opt, plain)); err != nil || got.Validate() != nil {
		t.Fatalf("Session-Id first: %v / %v", err, got.Validate())
	}

	if err := got.Unmarshal(message(testAnswerHeader, opt, plain, sid)); err != nil {
		t.Fatal(err)
	}
	if got.SessionId != "s" {
		t.Fatal("a misplaced Session-Id must still be decoded")
	}
	ae := wantResult(t, got.Validate(), diam.ResultInvalidAVPValue)
	if ae.AVP.Code != AVPSessionId || string(ae.AVP.Data) != "s" {
		t.Fatalf("AVP = %+v, want the Session-Id", ae.AVP)
	}

	// An unknown AVP in front counts too.
	unknown := diam.AppendRaw(nil, diam.RawAVP{Code: 9999, Data: []byte{1}})
	if err := got.Unmarshal(message(testAnswerHeader, unknown, sid)); err != nil {
		t.Fatal(err)
	}
	wantResult(t, got.Validate(), diam.ResultInvalidAVPValue)

	// Reusing the struct for a well-formed message clears the finding.
	if err := got.Unmarshal(message(testAnswerHeader, sid)); err != nil || got.Validate() != nil {
		t.Fatalf("after reuse: %v / %v", err, got.Validate())
	}
}

func TestMOverride(t *testing.T) {
	in := &Outer{Middle: Middle{InnerU32: diam.Some(uint32(1))}, SoftU32: diam.Some(uint32(2))}
	top := avps(t, in.AppendAVPs(nil))
	if len(top) != 2 || top[0].Code != AVPMiddle || top[1].Code != AVPInnerU32 {
		t.Fatalf("Outer AVPs = %+v", top)
	}
	if top[1].Flags != diam.AVPFlagVendor {
		t.Fatalf("flags with (diam.m) = false: %#x, want the M-bit cleared", top[1].Flags)
	}
	inner := avps(t, top[0].Data)
	if len(inner) != 1 || inner[0].Code != AVPInnerU32 || inner[0].Flags != diam.AVPFlagVendor|diam.AVPFlagMandatory {
		t.Fatalf("the same AVP without an override: %+v, want the dictionary M-bit", inner)
	}
}

func TestValidateOrder(t *testing.T) {
	sid := avp(AVPSessionId, []byte("s"))

	// A decode error is reported before an excess occurrence, whatever their
	// order on the wire.
	msg := message(testAnswerHeader, sid, sid, avp(AVPOptU32, []byte{1}))
	var got TestAnswer
	if err := got.Unmarshal(msg); err != nil {
		t.Fatal(err)
	}
	wantResult(t, got.Validate(), diam.ResultInvalidAVPLength)

	// Excess before missing: Min-U32 is absent and Opt-U32 occurs twice.
	opt := avp(AVPOptU32, []byte{0, 0, 0, 1})
	var c Cards
	if err := c.DecodeAVPs(append(append([]byte(nil), opt...), opt...)); err != nil {
		t.Fatal(err)
	}
	wantResult(t, c.Validate(), diam.ResultAVPOccursTooMany)

	// Missing before misplaced, misplaced before a nested violation, and a
	// nested violation before an unknown mandatory AVP.
	unknownM := diam.AppendRaw(nil, diam.RawAVP{Code: 9998, Flags: diam.AVPFlagMandatory, Data: []byte{1}})
	emptyOuter := avp(AVPOuter, nil) // Middle is missing inside
	var nr NestedRequest
	if err := nr.Unmarshal(message(nestedRequestHeader, unknownM, emptyOuter, avp(AVPNestedSession, []byte("s")))); err != nil {
		t.Fatal(err)
	}
	ae := wantResult(t, nr.Validate(), diam.ResultInvalidAVPValue)
	if ae.AVP.Code != AVPNestedSession {
		t.Fatalf("first violation = %+v, want the misplaced session AVP", ae)
	}
	if err := nr.Unmarshal(message(nestedRequestHeader, avp(AVPNestedSession, []byte("s")), unknownM, emptyOuter)); err != nil {
		t.Fatal(err)
	}
	ae = wantResult(t, nr.Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPMiddle || !reflect.DeepEqual(ae.Path, []uint32{AVPOuter}) {
		t.Fatalf("second violation = %+v, want Middle missing in Outer", ae)
	}
}

func TestCrossPackageGroup(t *testing.T) {
	in := &NestedRequest{NestedSession: "s", Outer: Outer{Middle: Middle{
		ProxyInfo: []base.ProxyInfo{{ProxyHost: "dra.example.org", ProxyState: []byte{1}}},
	}}}
	b, _ := in.Marshal()
	var got NestedRequest
	if err := got.Unmarshal(b); err != nil || got.Validate() != nil {
		t.Fatalf("valid message: %v / %v", err, got.Validate())
	}
	if pi := got.Outer.Middle.ProxyInfo; len(pi) != 1 || pi[0].ProxyHost != "dra.example.org" {
		t.Fatalf("ProxyInfo = %+v", pi)
	}

	// A Proxy-Info without its required Proxy-State, three groups deep. It
	// is an IETF AVP (no vendor) inside vendor-specific groups.
	host := diam.AppendRaw(nil, diam.RawAVP{Code: base.AVPProxyHost, Flags: diam.AVPFlagMandatory, Data: []byte("dra")})
	pi := diam.AppendRaw(nil, diam.RawAVP{Code: base.AVPProxyInfo, Flags: diam.AVPFlagMandatory, Data: host})
	msg := message(nestedRequestHeader, avp(AVPNestedSession, []byte("s")), avp(AVPOuter, avp(AVPMiddle, pi)))
	if err := got.Unmarshal(msg); err != nil {
		t.Fatal(err)
	}
	ae := wantResult(t, got.Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != base.AVPProxyState || !reflect.DeepEqual(ae.Path, []uint32{AVPOuter, AVPMiddle, base.AVPProxyInfo}) {
		t.Fatalf("error = %+v, want Proxy-State missing in Outer/Middle/Proxy-Info", ae)
	}
}
