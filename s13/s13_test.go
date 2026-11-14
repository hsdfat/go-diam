package s13

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6a"
)

const vendor3GPP = 10415

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

// split cuts an AVP sequence into AVPs.
func split(t *testing.T, data []byte) []diam.RawAVP {
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

func join(avps []diam.RawAVP) []byte {
	var b []byte
	for _, a := range avps {
		b = diam.AppendRaw(b, a)
	}
	return b
}

func encode(t *testing.T, m diam.Message) []byte {
	t.Helper()
	b, err := m.AppendMarshal(nil)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// avpsOf returns the top-level AVPs of an encoded message.
func avpsOf(t *testing.T, msg []byte) []diam.RawAVP {
	t.Helper()
	_, data, err := diam.ParseHeader(msg)
	if err != nil {
		t.Fatal(err)
	}
	return split(t, data)
}

// withAVPs re-frames msg with another AVP sequence.
func withAVPs(t *testing.T, msg []byte, avps []diam.RawAVP) []byte {
	t.Helper()
	h, _, err := diam.ParseHeader(msg)
	if err != nil {
		t.Fatal(err)
	}
	b := diam.AppendHeader(nil, &h)
	b = append(b, join(avps)...)
	b, err = diam.FinishMessage(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// dropAVP removes every top-level AVP (code, vendor) from msg.
func dropAVP(t *testing.T, msg []byte, code, vendor uint32) []byte {
	t.Helper()
	return withAVPs(t, msg, slices.DeleteFunc(avpsOf(t, msg), func(a diam.RawAVP) bool { return a.Code == code && a.Vendor == vendor }))
}

// doubleAVP repeats the first top-level AVP of code in msg.
func doubleAVP(t *testing.T, msg []byte, code uint32) []byte {
	t.Helper()
	avps := avpsOf(t, msg)
	for _, a := range avps {
		if a.Code == code {
			return withAVPs(t, msg, append(avps, a))
		}
	}
	t.Fatalf("AVP %d not in the message", code)
	return nil
}

func decode(t *testing.T, b []byte) diam.Message {
	t.Helper()
	m, err := diam.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRoundTrip(t *testing.T) {
	ss := samples()
	if len(ss) != 2 {
		t.Fatalf("%d samples, want 2", len(ss))
	}
	for i, in := range ss {
		name := reflect.TypeOf(in).Elem().Name()
		b1 := encode(t, in)
		m := decode(t, b1)
		if reflect.TypeOf(m) != reflect.TypeOf(in) {
			t.Fatalf("%s: Decode returned %T", name, m)
		}
		if err := m.Validate(); err != nil {
			t.Fatalf("%s: Validate: %v", name, err)
		}
		h := m.DiamHeader()
		// Table 7.2.2/2: code 324; §7.1.8: application id 16777252.
		if h.CommandCode != 324 || h.AppID != 16777252 || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
			t.Fatalf("%s: header = %+v", name, h)
		}
		if isRequest := i%2 == 0; (h.Flags&diam.FlagRequest != 0) != isRequest {
			t.Fatalf("%s: R-bit = %v", name, !isRequest)
		}
		// Both headers carry PXY (§7.2.19, §7.2.20).
		if h.Flags&diam.FlagProxiable == 0 {
			t.Fatalf("%s: P-bit clear", name)
		}
		if b2 := encode(t, m); !bytes.Equal(b1, b2) {
			t.Fatalf("%s: re-encode differs\n%x\n%x", name, b1, b2)
		}
		if !reflect.DeepEqual(m, decode(t, encode(t, m))) {
			t.Fatalf("%s: decode -> encode -> decode is not stable", name)
		}
		if first := avpsOf(t, b1)[0]; first.Code != base.AVPSessionId {
			t.Fatalf("%s: first AVP is %d, want Session-Id", name, first.Code)
		}
	}
}

func TestDecodedValues(t *testing.T) {
	var ecr MeIdentityCheckRequest
	if err := ecr.Unmarshal(encode(t, samples()[0])); err != nil {
		t.Fatal(err)
	}
	if ecr.TerminalInformation.Imei.V != imei || ecr.TerminalInformation.SoftwareVersion.V != svn ||
		ecr.TerminalInformation.Tgpp2Meid.Set || ecr.UserName.V != imsi || ecr.Drmp.V != s6a.DrmpPriority5 ||
		ecr.VendorSpecificApplicationId == nil || ecr.VendorSpecificApplicationId.AuthApplicationId.V != 16777252 {
		t.Fatalf("ECR = %+v", ecr)
	}

	var eca MeIdentityCheckAnswer
	if err := eca.Unmarshal(encode(t, samples()[1])); err != nil {
		t.Fatal(err)
	}
	if eca.ResultCode.V != 2001 || eca.EquipmentStatus.V != s6a.EquipmentStatusTrackinglisted || eca.FailedAvp == nil {
		t.Fatalf("ECA = %+v", eca)
	}
}

// mS13 is the M-bit table 7.3.1/1 gives the S13 AVPs of vendor 3GPP: "M, V"
// for Terminal-Information (1401), IMEI (1402), Software-Version (1403),
// Equipment-Status (1445) and 3GPP2-MEID (1471). It is restated here from
// the table, not read from the schema.
var mS13 = map[uint32]bool{1401: true, 1402: true, 1403: true, 1445: true, 1471: true}

// mIETF: DRMP is "Must not set" in table 7.3.1/2.
var mIETF = map[uint32]bool{301: false}

// groupCodes are the grouped AVPs of the samples, so the flag check can walk
// into them without consulting the schema.
var groupCodes = map[[2]uint32]bool{
	{284, 0}: true, {297, 0}: true, {279, 0}: true, {260, 0}: true, {1401, vendor3GPP}: true,
}

func TestFlagsFollowSpec(t *testing.T) {
	checked := map[[2]uint32]bool{}
	var walk func(name string, avps []diam.RawAVP, inFailed bool)
	walk = func(name string, avps []diam.RawAVP, inFailed bool) {
		for _, a := range avps {
			k := [2]uint32{a.Code, a.Vendor}
			if (a.Vendor != 0) != (a.Flags&diam.AVPFlagVendor != 0) {
				t.Errorf("%s: AVP %v: V-bit does not match the vendor", name, k)
			}
			if a.Flags&diam.AVPFlagProtected != 0 {
				t.Errorf("%s: AVP %v: P-bit set", name, k)
			}
			want, ok := mS13[a.Code]
			if a.Vendor == 0 {
				want, ok = mIETF[a.Code]
			}
			if ok && !inFailed {
				checked[k] = true
				if a.Mandatory() != want {
					t.Errorf("%s: AVP %v: M-bit = %v, table 7.3.1 says %v", name, k, a.Mandatory(), want)
				}
			}
			if groupCodes[k] {
				walk(name, split(t, a.Data), inFailed || k == [2]uint32{base.AVPFailedAvp, 0})
			}
		}
	}
	ss := samples()
	// 3GPP2-MEID is in Terminal-Information's grammar but not used on S13
	// (table 6.2.1.1/1); it is checked here in an extra message.
	extra := ss[0].(*MeIdentityCheckRequest)
	extra.TerminalInformation.Tgpp2Meid = diam.Some([]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde})
	for _, m := range append(ss, extra) {
		walk(reflect.TypeOf(m).Elem().Name(), avpsOf(t, encode(t, m)), false)
	}
	for c := range mS13 {
		if !checked[[2]uint32{c, vendor3GPP}] {
			t.Errorf("3GPP AVP %d is not in the samples", c)
		}
	}
	for c := range mIETF {
		if !checked[[2]uint32{c, 0}] {
			t.Errorf("IETF AVP %d is not in the samples", c)
		}
	}
}

func TestMissingRequired(t *testing.T) {
	for _, c := range []struct {
		sample       int
		code, vendor uint32
	}{
		{0, s6a.AVPTerminalInformation, vendor3GPP},
		{0, base.AVPAuthSessionState, 0},
		{0, base.AVPOriginHost, 0},
		{0, base.AVPOriginRealm, 0},
		{0, base.AVPDestinationRealm, 0},
		{1, base.AVPAuthSessionState, 0},
		{1, base.AVPOriginHost, 0},
		{1, base.AVPOriginRealm, 0},
	} {
		b := dropAVP(t, encode(t, samples()[c.sample]), c.code, c.vendor)
		ae := wantResult(t, decode(t, b).Validate(), diam.ResultMissingAVP)
		if ae.AVP.Code != c.code || ae.AVP.Vendor != c.vendor {
			t.Fatalf("sample %d: AVP = %+v, want code %d vendor %d", c.sample, ae.AVP, c.code, c.vendor)
		}
	}
	// The optional AVPs of both grammars may all be left out, User-Name
	// (the IMSI) included.
	for i, opt := range [][][2]uint32{
		{{base.AVPUserName, 0}, {base.AVPDestinationHost, 0}, {base.AVPVendorSpecificApplicationId, 0}, {s6a.AVPDrmp, 0}},
		{{s6a.AVPEquipmentStatus, vendor3GPP}, {base.AVPResultCode, 0}, {base.AVPFailedAvp, 0}, {base.AVPVendorSpecificApplicationId, 0}},
	} {
		b := encode(t, samples()[i])
		for _, k := range opt {
			b = dropAVP(t, b, k[0], k[1])
		}
		if err := decode(t, b).Validate(); err != nil {
			t.Fatalf("sample %d without its optional AVPs: %v", i, err)
		}
	}
}

// Terminal-Information (§7.3.3) has only optional members. Table 6.2.1.1/1
// and §6.2.1.2 (IMEI present, no 3GPP2-MEID on S13) are procedure rules, so
// neither an empty Terminal-Information nor one with 3GPP2-MEID fails
// Validate; 3GPP2-MEID decodes into its field, not into Unknown.
func TestTerminalInformationMembers(t *testing.T) {
	ecr := samples()[0].(*MeIdentityCheckRequest)
	ecr.TerminalInformation = s6a.TerminalInformation{}
	got := decode(t, encode(t, ecr)).(*MeIdentityCheckRequest)
	if err := got.Validate(); err != nil {
		t.Fatalf("empty Terminal-Information: %v", err)
	}

	meid := []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde}
	ecr.TerminalInformation = s6a.TerminalInformation{Imei: diam.Some(imei), Tgpp2Meid: diam.Some(meid)}
	got = decode(t, encode(t, ecr)).(*MeIdentityCheckRequest)
	if err := got.Validate(); err != nil {
		t.Fatalf("Terminal-Information with 3GPP2-MEID: %v", err)
	}
	if !bytes.Equal(got.TerminalInformation.Tgpp2Meid.V, meid) || len(got.TerminalInformation.Unknown) != 0 {
		t.Fatalf("Terminal-Information = %+v", got.TerminalInformation)
	}
}

// §6.2.1.3: an unknown IMEI is answered with Experimental-Result
// DIAMETER_ERROR_EQUIPMENT_UNKNOWN (5422, §7.4.3.5) and no Equipment-Status.
// Table 6.2.1.1/2 makes Equipment-Status conditional on DIAMETER_SUCCESS;
// that is not grammar, so an answer either way passes Validate.
func TestEquipmentUnknown(t *testing.T) {
	eca := &MeIdentityCheckAnswer{
		Header: hdr, SessionId: "mme;1;9", ExperimentalResult: &base.ExperimentalResult{VendorId: vendor3GPP, ExperimentalResultCode: 5422},
		AuthSessionState: nsm, OriginHost: eir, OriginRealm: realm,
	}
	got := decode(t, encode(t, eca)).(*MeIdentityCheckAnswer)
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if got.ExperimentalResult.ExperimentalResultCode != 5422 || got.EquipmentStatus.Set || got.ResultCode.Set {
		t.Fatalf("ECA = %+v", got)
	}

	success := samples()[1].(*MeIdentityCheckAnswer)
	success.EquipmentStatus = diam.Opt[s6a.EquipmentStatus]{}
	if err := decode(t, encode(t, success)).Validate(); err != nil {
		t.Fatalf("DIAMETER_SUCCESS without Equipment-Status: %v", err)
	}
}

func TestTooMany(t *testing.T) {
	for _, c := range []struct {
		sample int
		code   uint32
	}{
		{0, s6a.AVPTerminalInformation}, // { Terminal-Information }
		{0, base.AVPUserName},           // [ User-Name ]
		{0, s6a.AVPDrmp},                // [ DRMP ]
		{1, s6a.AVPEquipmentStatus},     // [ Equipment-Status ]
		{1, base.AVPFailedAvp},          // [ Failed-AVP ] (current release)
	} {
		ae := wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[c.sample]), c.code)).Validate(), diam.ResultAVPOccursTooMany)
		if ae.AVP.Code != c.code {
			t.Fatalf("sample %d: AVP = %+v, want %d", c.sample, ae.AVP, c.code)
		}
	}
	// [ IMEI ] twice inside Terminal-Information.
	ecr := samples()[0].(*MeIdentityCheckRequest)
	ecr.TerminalInformation.Unknown = []diam.RawAVP{{Code: s6a.AVPImei, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte(imei)}}
	ae := wantResult(t, decode(t, encode(t, ecr)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != s6a.AVPImei || !slices.Equal(ae.Path, []uint32{s6a.AVPTerminalInformation}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

func TestBadValues(t *testing.T) {
	replace := func(msg []byte, code uint32, data []byte) []byte {
		avps := avpsOf(t, msg)
		for i := range avps {
			if avps[i].Code == code {
				avps[i].Data = data
			}
		}
		return withAVPs(t, msg, avps)
	}
	// Equipment-Status is Enumerated (four octets).
	ae := wantResult(t, decode(t, replace(encode(t, samples()[1]), s6a.AVPEquipmentStatus, []byte{0, 1})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != s6a.AVPEquipmentStatus {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// DRMP is Enumerated (four octets).
	ae = wantResult(t, decode(t, replace(encode(t, samples()[0]), s6a.AVPDrmp, []byte{5})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != s6a.AVPDrmp {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// Terminal-Information is Grouped: a member that overruns the group.
	ae = wantResult(t, decode(t, replace(encode(t, samples()[0]), s6a.AVPTerminalInformation, []byte{0, 0, 5, 0x82, 0xc0, 0, 0, 0x40})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != s6a.AVPTerminalInformation {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
}

func TestMisplacedFixed(t *testing.T) {
	// Session-Id is < > in both commands.
	for i := range samples() {
		msg := encode(t, samples()[i])
		avps := avpsOf(t, msg)
		avps[0], avps[1] = avps[1], avps[0]
		ae := wantResult(t, decode(t, withAVPs(t, msg, avps)).Validate(), diam.ResultInvalidAVPValue)
		if ae.AVP.Code != base.AVPSessionId {
			t.Fatalf("sample %d: AVP = %+v", i, ae.AVP)
		}
	}
}

func TestUnknownAVPs(t *testing.T) {
	ecr := encode(t, samples()[0])
	quiet := diam.RawAVP{Code: 9999, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte("later release")}
	m := decode(t, withAVPs(t, ecr, append(avpsOf(t, ecr), quiet)))
	if err := m.Validate(); err != nil {
		t.Fatalf("unknown AVP without M-bit: %v", err)
	}
	if got := m.(*MeIdentityCheckRequest).Unknown; len(got) != 1 || got[0].Code != 9999 {
		t.Fatalf("Unknown = %+v", got)
	}

	loud := quiet
	loud.Flags |= diam.AVPFlagMandatory
	ae := wantResult(t, decode(t, withAVPs(t, ecr, append(avpsOf(t, ecr), loud))).Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != 9999 {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// The same inside Terminal-Information, a group with *[ AVP ].
	req := samples()[0].(*MeIdentityCheckRequest)
	req.TerminalInformation.Unknown = []diam.RawAVP{loud}
	ae = wantResult(t, decode(t, encode(t, req)).Validate(), diam.ResultAVPUnsupported)
	if !slices.Equal(ae.Path, []uint32{s6a.AVPTerminalInformation}) {
		t.Fatalf("Path = %v", ae.Path)
	}
}

// Release 8 peers send only what TS 29.272 V8.14.0 knew (no DRMP); their
// messages must validate.
func TestRelease8Messages(t *testing.T) {
	for _, m := range []diam.Message{
		&MeIdentityCheckRequest{
			SessionId: "mme;1", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: mme, OriginRealm: realm,
			DestinationRealm: realm, TerminalInformation: s6a.TerminalInformation{Imei: diam.Some(imei[:14])},
		},
		&MeIdentityCheckAnswer{
			SessionId: "mme;1", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm,
			OriginHost: eir, OriginRealm: realm, EquipmentStatus: diam.Some(s6a.EquipmentStatusPermittedlisted),
		},
	} {
		if err := decode(t, encode(t, m)).Validate(); err != nil {
			t.Fatalf("Rel-8 %T: %v", m, err)
		}
	}
}

func TestNewAnswerFor(t *testing.T) {
	req := samples()[0].(*MeIdentityCheckRequest)
	req.Header.Flags = diam.FlagRequest | diam.FlagProxiable | diam.FlagRetransmit
	ans := NewMeIdentityCheckAnswerFor(req)
	if h := ans.Header; h.Flags != diam.FlagProxiable || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
		t.Fatalf("answer header = %+v", h)
	}
	if ans.SessionId != req.SessionId {
		t.Fatalf("Session-Id = %q, want %q", ans.SessionId, req.SessionId)
	}
}

func TestEnumValuesFollowSpec(t *testing.T) {
	// Equipment-Status (§7.3.51); the type is s6a's.
	for name, c := range map[string][2]int32{
		"PERMITTEDLISTED":  {int32(s6a.EquipmentStatusPermittedlisted), 0},
		"PROHIBITEDLISTED": {int32(s6a.EquipmentStatusProhibitedlisted), 1},
		"TRACKINGLISTED":   {int32(s6a.EquipmentStatusTrackinglisted), 2},
	} {
		if c[0] != c[1] {
			t.Errorf("Equipment-Status %s = %d, want %d", name, c[0], c[1])
		}
	}
	if got := s6a.EquipmentStatusTrackinglisted.String(); got != "TRACKINGLISTED" {
		t.Errorf("String() = %q", got)
	}
}

// S13 defines no AVP codes of its own; these are the S6a constants it uses,
// restated from TS 29.272 table 7.3.1/1 and RFC 7944 §9.1. The dictionary
// cross-check in internal/interop covers them against go-diameter too.
func TestAVPCodes(t *testing.T) {
	for name, c := range map[string][2]uint32{
		"Terminal-Information": {s6a.AVPTerminalInformation, 1401},
		"IMEI":                 {s6a.AVPImei, 1402},
		"Software-Version":     {s6a.AVPSoftwareVersion, 1403},
		"Equipment-Status":     {s6a.AVPEquipmentStatus, 1445},
		"3GPP2-MEID":           {s6a.AVPTgpp2Meid, 1471},
		"DRMP":                 {s6a.AVPDrmp, 301},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}
