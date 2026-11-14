package zh

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/cx"
	"github.com/hsdfat/go-diam/diam"
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
		// §6.1: the Cx code 303; §6.0: application id 16777221.
		if h.CommandCode != 303 || h.AppID != 16777221 || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
			t.Fatalf("%s: header = %+v", name, h)
		}
		if isRequest := i%2 == 0; (h.Flags&diam.FlagRequest != 0) != isRequest {
			t.Fatalf("%s: R-bit = %v", name, !isRequest)
		}
		// Both headers carry PXY (§4.2 steps 1 and 3).
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

// The Zh commands share code 303 with Cx; the application id keeps them
// apart, in both directions.
func TestNotCx(t *testing.T) {
	if _, ok := decode(t, encode(t, samples()[0])).(*MultimediaAuthRequest); !ok {
		t.Fatal("Zh MAR does not decode as zh.MultimediaAuthRequest")
	}
	cxMar := &cx.MultimediaAuthRequest{SessionId: "s", VendorSpecificApplicationId: vsai, AuthSessionState: nsm,
		OriginHost: bsf, OriginRealm: realm, DestinationRealm: realm, UserName: impi, PublicIdentity: impu,
		ServerName: "sip:scscf.ims.example.org"}
	if _, ok := decode(t, encode(t, cxMar)).(*cx.MultimediaAuthRequest); !ok {
		t.Fatal("Cx MAR does not decode as cx.MultimediaAuthRequest")
	}
}

func TestDecodedValues(t *testing.T) {
	var mar MultimediaAuthRequest
	if err := mar.Unmarshal(encode(t, samples()[0])); err != nil {
		t.Fatal(err)
	}
	if mar.UserName.V != impi || mar.PublicIdentity.V != impu || !mar.GussTimestamp.V.Equal(gussStamp) ||
		mar.SipAuthDataItem == nil || len(mar.SipAuthDataItem.SipAuthorization.V) != 30 ||
		mar.VendorSpecificApplicationId.AuthApplicationId.V != 16777221 {
		t.Fatalf("MAR = %+v", mar)
	}

	var maa MultimediaAuthAnswer
	if err := maa.Unmarshal(encode(t, samples()[1])); err != nil {
		t.Fatal(err)
	}
	if maa.ResultCode.V != 2001 || !bytes.Equal(maa.GbaUserSecSettings.V, guss) ||
		maa.Sha256Indication.V != Sha256IndicationUiccSupportSha256 || maa.SipAuthDataItem == nil ||
		!bytes.Equal(maa.SipAuthDataItem.IntegrityKey.V, key16) || maa.SipAuthDataItem.SipItemNumber.V != 1 {
		t.Fatalf("MAA = %+v", maa)
	}
}

// m3GPP is the M-bit of the 3GPP AVPs on Zh, restated from the tables, not
// read from the schema: table 6.1 gives "M, V" for GBA-UserSecSettings (400)
// and SHA-256-Indication (421) and "V, M must not" for GUSS-Timestamp
// (409); the Cx AVPs are "used as they are" (§6.3), so TS 29.229 table
// 6.3.0.1 gives "M, V" for Public-Identity (601), SIP-Authentication-Scheme
// to SIP-Item-Number (608-613), Confidentiality-Key (625) and Integrity-Key
// (626), and "V, M must not" for SIP-Digest-Authenticate (635).
var m3GPP = map[uint32]bool{
	400: true, 409: false, 421: true,
	601: true, 608: true, 609: true, 610: true, 612: true, 613: true, 625: true, 626: true, 635: false,
}

// mIETF: the Digest AVPs inside SIP-Digest-Authenticate, "M" in TS 29.229
// table 6.3.0.1.
var mIETF = map[uint32]bool{104: true, 110: true, 111: true, 121: true}

// groupCodes are the grouped AVPs of the samples, so the flag check can walk
// into them without consulting the schema.
var groupCodes = map[[2]uint32]bool{
	{284, 0}: true, {297, 0}: true, {279, 0}: true, {260, 0}: true,
	{612, vendor3GPP}: true, {635, vendor3GPP}: true,
}

// digestAnswer is a MAA for SIP Digest (§4.2 step 2): the vector is a
// SIP-Digest-Authenticate.
func digestAnswer() *MultimediaAuthAnswer {
	return &MultimediaAuthAnswer{
		Header: hdr, SessionId: "bsf;1;2", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)),
		AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, UserName: diam.Some(impi),
		SipAuthDataItem: &cx.SipAuthDataItem{SipAuthenticationScheme: diam.Some("SIP Digest"),
			SipDigestAuthenticate: &cx.SipDigestAuthenticate{DigestRealm: realm, DigestAlgorithm: diam.Some("MD5"),
				DigestQop: "auth", DigestHa1: "939e7578"}},
	}
}

func TestFlagsFollowSpec(t *testing.T) {
	checked := map[[2]uint32]bool{}
	var walk func(name string, avps []diam.RawAVP)
	walk = func(name string, avps []diam.RawAVP) {
		for _, a := range avps {
			k := [2]uint32{a.Code, a.Vendor}
			if (a.Vendor != 0) != (a.Flags&diam.AVPFlagVendor != 0) {
				t.Errorf("%s: AVP %v: V-bit does not match the vendor", name, k)
			}
			if a.Flags&diam.AVPFlagProtected != 0 {
				t.Errorf("%s: AVP %v: P-bit set", name, k)
			}
			want, ok := m3GPP[a.Code]
			if a.Vendor == 0 {
				want, ok = mIETF[a.Code]
			}
			if ok {
				checked[k] = true
				if a.Mandatory() != want {
					t.Errorf("%s: AVP %v: M-bit = %v, want %v", name, k, a.Mandatory(), want)
				}
			}
			if groupCodes[k] {
				walk(name, split(t, a.Data))
			}
		}
	}
	for _, m := range append(samples(), digestAnswer()) {
		walk(reflect.TypeOf(m).Elem().Name(), avpsOf(t, encode(t, m)))
	}
	for c := range m3GPP {
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
		// §6.1: Vendor-Specific-Application-Id stays { } on Zh.
		{0, base.AVPVendorSpecificApplicationId, 0},
		{0, base.AVPAuthSessionState, 0},
		{0, base.AVPOriginHost, 0},
		{0, base.AVPOriginRealm, 0},
		{0, base.AVPDestinationRealm, 0},
		{1, base.AVPVendorSpecificApplicationId, 0},
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
	// Everything else is optional; the identity rules of §4.2 are procedure
	// rules, so a MAR with neither identity and a bare MAA validate.
	for i, opt := range [][][2]uint32{
		{{base.AVPUserName, 0}, {cx.AVPPublicIdentity, vendor3GPP}, {cx.AVPSipAuthDataItem, vendor3GPP},
			{AVPGussTimestamp, vendor3GPP}, {base.AVPDestinationHost, 0}},
		{{base.AVPResultCode, 0}, {base.AVPUserName, 0}, {cx.AVPPublicIdentity, vendor3GPP}, {cx.AVPSipAuthDataItem, vendor3GPP},
			{AVPGbaUsersecsettings, vendor3GPP}, {AVPSha256Indication, vendor3GPP}},
	} {
		b := encode(t, samples()[i])
		for _, k := range opt {
			b = dropAVP(t, b, k[0], k[1])
		}
		if err := decode(t, b).Validate(); err != nil {
			t.Fatalf("sample %d without its optional AVPs: %v", i, err)
		}
	}
	// { Digest-QoP } inside SIP-Digest-Authenticate: 5005 with the path.
	maa := digestAnswer()
	b := encode(t, maa)
	avps := avpsOf(t, b)
	for i := range avps {
		if avps[i].Code == cx.AVPSipAuthDataItem {
			inner := split(t, avps[i].Data)
			for j := range inner {
				if inner[j].Code == cx.AVPSipDigestAuthenticate {
					inner[j].Data = join(slices.DeleteFunc(split(t, inner[j].Data), func(a diam.RawAVP) bool { return a.Code == cx.AVPDigestQop }))
				}
			}
			avps[i].Data = join(inner)
		}
	}
	ae := wantResult(t, decode(t, withAVPs(t, b, avps)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != cx.AVPDigestQop || !slices.Equal(ae.Path, []uint32{cx.AVPSipAuthDataItem, cx.AVPSipDigestAuthenticate}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

// §4.2 step 2: an IMPI or IMPU unknown to the HSS gets
// DIAMETER_ERROR_IDENTITY_UNKNOWN (5401, §6.2.2.1) in Experimental-Result.
func TestIdentityUnknown(t *testing.T) {
	maa := &MultimediaAuthAnswer{
		Header: hdr, SessionId: "bsf;1;9", VendorSpecificApplicationId: vsai,
		ExperimentalResult: &base.ExperimentalResult{VendorId: vendor3GPP, ExperimentalResultCode: 5401},
		AuthSessionState:   nsm, OriginHost: hss, OriginRealm: realm,
	}
	got := decode(t, encode(t, maa)).(*MultimediaAuthAnswer)
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if got.ExperimentalResult.ExperimentalResultCode != 5401 || got.ResultCode.Set || got.SipAuthDataItem != nil {
		t.Fatalf("MAA = %+v", got)
	}
}

// The Zh MAA grammar has no Failed-AVP; go-diam adds [ Failed-AVP ], so an
// answer that carries one decodes into the field and passes Validate.
func TestFailedAVP(t *testing.T) {
	maa := samples()[1].(*MultimediaAuthAnswer)
	inner := diam.RawAVP{Code: base.AVPOriginStateId, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 9}}
	maa.FailedAvp = &base.FailedAvp{Unknown: []diam.RawAVP{inner}}
	got := decode(t, encode(t, maa)).(*MultimediaAuthAnswer)
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if got.FailedAvp == nil || len(got.FailedAvp.Unknown) != 1 || got.FailedAvp.Unknown[0].Code != base.AVPOriginStateId {
		t.Fatalf("Failed-AVP = %+v", got.FailedAvp)
	}
	if len(got.Unknown) != 0 {
		t.Fatalf("Unknown = %+v", got.Unknown)
	}
}

func TestTooMany(t *testing.T) {
	for _, c := range []struct {
		sample int
		code   uint32
	}{
		{0, base.AVPUserName},                    // [ User-Name ]
		{0, cx.AVPPublicIdentity},                // [ Public-Identity ]
		{0, cx.AVPSipAuthDataItem},               // [ SIP-Auth-Data-Item ], not *[ ] as on Cx
		{0, AVPGussTimestamp},                    // [ GUSS-Timestamp ]
		{0, base.AVPVendorSpecificApplicationId}, // { Vendor-Specific-Application-Id }
		{1, cx.AVPSipAuthDataItem},               // [ SIP-Auth-Data-Item ]: one AV
		{1, AVPGbaUsersecsettings},               // [ GBA-UserSecSettings ]
		{1, AVPSha256Indication},                 // [ SHA-256-Indication ]
	} {
		ae := wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[c.sample]), c.code)).Validate(), diam.ResultAVPOccursTooMany)
		if ae.AVP.Code != c.code {
			t.Fatalf("sample %d: AVP = %+v, want %d", c.sample, ae.AVP, c.code)
		}
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
	// GUSS-Timestamp is a Time (four octets).
	ae := wantResult(t, decode(t, replace(encode(t, samples()[0]), AVPGussTimestamp, []byte{1, 2, 3, 4, 5})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPGussTimestamp {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// SHA-256-Indication is Enumerated (four octets).
	ae = wantResult(t, decode(t, replace(encode(t, samples()[1]), AVPSha256Indication, []byte{1})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPSha256Indication {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// SIP-Auth-Data-Item is Grouped: a member that overruns the group.
	ae = wantResult(t, decode(t, replace(encode(t, samples()[1]), cx.AVPSipAuthDataItem, []byte{0, 0, 2, 0x65, 0xc0, 0, 0, 0x40})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != cx.AVPSipAuthDataItem {
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
	mar := encode(t, samples()[0])
	quiet := diam.RawAVP{Code: 9999, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte("later release")}
	m := decode(t, withAVPs(t, mar, append(avpsOf(t, mar), quiet)))
	if err := m.Validate(); err != nil {
		t.Fatalf("unknown AVP without M-bit: %v", err)
	}
	if got := m.(*MultimediaAuthRequest).Unknown; len(got) != 1 || got[0].Code != 9999 {
		t.Fatalf("Unknown = %+v", got)
	}
	loud := quiet
	loud.Flags |= diam.AVPFlagMandatory
	ae := wantResult(t, decode(t, withAVPs(t, mar, append(avpsOf(t, mar), loud))).Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != 9999 {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// Cx AVPs that the Zh MAR does not list (Server-Name, with the M-bit)
	// are unknown here too.
	serverName := diam.RawAVP{Code: cx.AVPServerName, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte("sip:scscf")}
	ae = wantResult(t, decode(t, withAVPs(t, mar, append(avpsOf(t, mar), serverName))).Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != cx.AVPServerName {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
}

// Release 8 peers send only what TS 29.109 V8.9.0 knew: no
// SHA-256-Indication. Their messages must validate.
func TestRelease8Messages(t *testing.T) {
	for _, m := range []diam.Message{
		&MultimediaAuthRequest{
			SessionId: "bsf;1", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: bsf, OriginRealm: realm,
			DestinationRealm: realm, UserName: diam.Some(impi),
		},
		&MultimediaAuthAnswer{
			SessionId: "bsf;1", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm,
			OriginHost: hss, OriginRealm: realm, UserName: diam.Some(impi), SipAuthDataItem: vector, GbaUserSecSettings: diam.Some(guss),
		},
	} {
		if err := decode(t, encode(t, m)).Validate(); err != nil {
			t.Fatalf("Rel-8 %T: %v", m, err)
		}
	}
}

func TestNewAnswerFor(t *testing.T) {
	req := samples()[0].(*MultimediaAuthRequest)
	req.Header.Flags = diam.FlagRequest | diam.FlagProxiable | diam.FlagRetransmit
	ans := NewMultimediaAuthAnswerFor(req)
	if h := ans.Header; h.Flags != diam.FlagProxiable || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
		t.Fatalf("answer header = %+v", h)
	}
	if ans.SessionId != req.SessionId {
		t.Fatalf("Session-Id = %q, want %q", ans.SessionId, req.SessionId)
	}
}

func TestEnumValuesFollowSpec(t *testing.T) {
	// SHA-256-Indication (§6.3.1.22).
	if Sha256IndicationUiccNotSupportSha256 != 0 || Sha256IndicationUiccSupportSha256 != 1 {
		t.Fatal("SHA-256-Indication values differ from §6.3.1.22")
	}
	if got := Sha256IndicationUiccSupportSha256.String(); got != "UICC_SUPPORT_SHA256" {
		t.Errorf("String() = %q", got)
	}
}

// AVP codes restated from table 6.1 and, for the Cx AVPs Zh uses, TS 29.229
// table 6.3.0.1.
func TestAVPCodes(t *testing.T) {
	for name, c := range map[string][2]uint32{
		"GBA-UserSecSettings":     {AVPGbaUsersecsettings, 400},
		"GUSS-Timestamp":          {AVPGussTimestamp, 409},
		"SHA-256-Indication":      {AVPSha256Indication, 421},
		"Public-Identity":         {cx.AVPPublicIdentity, 601},
		"SIP-Auth-Data-Item":      {cx.AVPSipAuthDataItem, 612},
		"SIP-Digest-Authenticate": {cx.AVPSipDigestAuthenticate, 635},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}
