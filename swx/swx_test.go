package swx

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/cx"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6a"
)

const (
	vendor3GPP = 10415
	vendorETSI = 13019
)

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

// commands is §8.2.2 restated: command code and the PXY bit of each sample,
// in sample order.
var commands = []struct {
	code      uint32
	proxiable bool
}{
	{303, true}, {303, true}, // MAR, MAA
	{305, false}, {305, true}, // PPR "305, REQ" (no PXY), PPA "305, PXY"
	{301, true}, {301, true}, // SAR, SAA
	{304, true}, {304, true}, // RTR, RTA
}

func TestRoundTrip(t *testing.T) {
	ss := samples()
	if len(ss) != len(commands) {
		t.Fatalf("%d samples, want %d", len(ss), len(commands))
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
		// §8.2.1: application id 16777265.
		if h.CommandCode != commands[i].code || h.AppID != 16777265 || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
			t.Fatalf("%s: header = %+v", name, h)
		}
		if isRequest := i%2 == 0; (h.Flags&diam.FlagRequest != 0) != isRequest {
			t.Fatalf("%s: R-bit = %v", name, !isRequest)
		}
		if (h.Flags&diam.FlagProxiable != 0) != commands[i].proxiable {
			t.Fatalf("%s: P-bit = %v, §8.2.2 says %v", name, !commands[i].proxiable, commands[i].proxiable)
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

// The SWx commands share their codes with Cx (TS 29.229 §6.1); the
// application id keeps them apart.
func TestNotCx(t *testing.T) {
	b := encode(t, samples()[0])
	if _, ok := decode(t, b).(*MultimediaAuthRequest); !ok {
		t.Fatal("an SWx MAR does not decode as swx.MultimediaAuthRequest")
	}
	cxMar := &cx.MultimediaAuthRequest{
		Header: hdr, SessionId: "s", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: aaa, OriginRealm: realm,
		DestinationRealm: realm, UserName: nai, PublicIdentity: "sip:a@example.org", ServerName: "sip:s.example.org",
	}
	if _, ok := decode(t, encode(t, cxMar)).(*cx.MultimediaAuthRequest); !ok {
		t.Fatal("a Cx MAR does not decode as cx.MultimediaAuthRequest")
	}
}

func TestDecodedValues(t *testing.T) {
	var saa ServerAssignmentAnswer
	if err := saa.Unmarshal(encode(t, samples()[5])); err != nil {
		t.Fatal(err)
	}
	p := saa.Non3GppUserData
	if p == nil || p.SubscriptionId.SubscriptionIdData.V != "15551234567" || p.Non3GppIpAccessApn.V != Non3GppIpAccessApnNon3gppApnsEnable ||
		!slices.Equal(p.RatType, []s6a.RatType{s6a.RatTypeVirtual}) || len(p.ApnConfiguration) != 1 ||
		p.ApnConfiguration[0].ServiceSelection != apn || p.TraceInfo.TraceData == nil ||
		p.TwanAccessInfo[0].WlanIdentifier.Hessid.V != "00-10-A4-23-19-C0" || p.EmergencyInfo.Mip6AgentInfo == nil ||
		p.Mip6FeatureVector.V != 0x0000400000000000 || saa.TgppAaaServerName.V != aaa || len(p.Unknown) != 0 {
		t.Fatalf("SAA = %+v\nprofile = %+v", saa, p)
	}

	var maa MultimediaAuthAnswer
	if err := maa.Unmarshal(encode(t, samples()[1])); err != nil {
		t.Fatal(err)
	}
	if len(maa.SipAuthDataItem) != 1 || maa.SipAuthDataItem[0].SipAuthenticationScheme.V != "EAP-AKA'" ||
		len(maa.SipAuthDataItem[0].ConfidentialityKey.V) != 16 || maa.OcOlr.OcSequenceNumber != 9 || len(maa.Load) != 1 {
		t.Fatalf("MAA = %+v", maa)
	}

	var ppa PushProfileAnswer
	if err := ppa.Unmarshal(encode(t, samples()[3])); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ppa.AccessNetworkInfo.LogicalAccessId.V, []byte("port-7")) || ppa.AccessNetworkInfo.Bssid.V != "00-10-A4-23-19-C1" ||
		ppa.LocalTimeZone.TimeZone != "+04" {
		t.Fatalf("PPA = %+v", ppa)
	}

	var sar ServerAssignmentRequest
	if err := sar.Unmarshal(encode(t, samples()[4])); err != nil {
		t.Fatal(err)
	}
	if sar.ServerAssignmentType != cx.ServerAssignmentTypePgwUpdate || len(sar.ActiveApn) != 1 || sar.EmergencyServices.V != 1 {
		t.Fatalf("SAR = %+v", sar)
	}
}

// mSWx is the M-bit of every AVP the SWx samples carry outside the S6a
// grouped AVPs, restated here from the specifications, not read from the
// schema: table 8.2.3.0/1 for the SWx AVPs; table 5.2.3.1/1 for SSID,
// HESSID, Access-Network-Info (1526) and Emergency-Services; for the re-used
// AVPs (table 8.2.3.0/2) their defining specification: TS 29.229 table
// 6.3.0.1 (600-626), TS 29.212 table 5.3.0.1 (RAT-Type: M should not),
// TS 29.272 table 7.3.1/1, TS 32.299 table 7.2.0.1 (BSSID), RFC 4006,
// RFC 5778, RFC 5580, ES 283 034 table 10; and "Must not set" for DRMP,
// OC-*, Load and MPS-Priority. Supported-Features (628) is per message.
var mSWx = map[[2]uint32]bool{
	{318, vendor3GPP}: true, {600, vendor3GPP}: true, {607, vendor3GPP}: true, {608, vendor3GPP}: true,
	{609, vendor3GPP}: true, {610, vendor3GPP}: true, {612, vendor3GPP}: true, {613, vendor3GPP}: true,
	{614, vendor3GPP}: true, {615, vendor3GPP}: true, {616, vendor3GPP}: true, {617, vendor3GPP}: true,
	{625, vendor3GPP}: true, {626, vendor3GPP}: true, {629, vendor3GPP}: false, {630, vendor3GPP}: false,
	{1032, vendor3GPP}: false,
	{1500, vendor3GPP}: true, {1501, vendor3GPP}: true, {1502, vendor3GPP}: true, {1503, vendor3GPP}: false,
	{1504, vendor3GPP}: true, {1505, vendor3GPP}: false, {1508, vendor3GPP}: false, {1509, vendor3GPP}: false,
	{1510, vendor3GPP}: false, {1511, vendor3GPP}: false, {1512, vendor3GPP}: false, {1518, vendor3GPP}: false,
	{1524, vendor3GPP}: false, {1525, vendor3GPP}: false, {1526, vendor3GPP}: false, {1541, vendor3GPP}: false,
	{2716, vendor3GPP}: true,
	{1401, vendor3GPP}: true, {1402, vendor3GPP}: true, {1403, vendor3GPP}: true, {1423, vendor3GPP}: true,
	{1427, vendor3GPP}: true, {1430, vendor3GPP}: true, {1435, vendor3GPP}: true, {1458, vendor3GPP}: true,
	{1459, vendor3GPP}: true, {1538, vendor3GPP}: false, {1612, vendor3GPP}: false, {1616, vendor3GPP}: false,
	{1649, vendor3GPP}: false, {1680, vendor3GPP}: false, {1687, vendor3GPP}: false, {1704, vendor3GPP}: false,
	{13, vendor3GPP}: false,
	{124, 0}:         true, {126, 0}: false, {127, 0}: false, {128, 0}: false, {301, 0}: false, {443, 0}: true,
	{444, 0}: true, {450, 0}: true, {486, 0}: true, {493, 0}: true, {621, 0}: false, {622, 0}: false,
	{623, 0}: false, {624, 0}: false, {625, 0}: false, {626, 0}: false, {627, 0}: false, {649, 0}: false,
	{650, 0}: false, {651, 0}: false, {652, 0}: false, {27, 0}: true,
	{302, vendorETSI}: false,
}

// groupCodes are the grouped AVPs the flag check walks into. The S6a
// grouped AVPs (APN-Configuration, Trace-Data, Active-APN, ...) carry the
// S6a flag rules of their members; see TestS6aGroupsKeepS6aFlags.
var groupCodes = map[[2]uint32]bool{
	{612, vendor3GPP}: true, {615, vendor3GPP}: true, {628, vendor3GPP}: true, {1401, vendor3GPP}: true,
	{1500, vendor3GPP}: true, {1505, vendor3GPP}: true, {1509, vendor3GPP}: true, {1510, vendor3GPP}: true,
	{1526, vendor3GPP}: true, {443, 0}: true, {621, 0}: true, {623, 0}: true, {650, 0}: true, {260, 0}: true,
	{297, 0}: true,
}

func TestFlagsFollowSpec(t *testing.T) {
	checked := map[[2]uint32]bool{}
	var walk func(name string, avps []diam.RawAVP, request, top bool)
	walk = func(name string, avps []diam.RawAVP, request, top bool) {
		for _, a := range avps {
			k := [2]uint32{a.Code, a.Vendor}
			if (a.Vendor != 0) != (a.Flags&diam.AVPFlagVendor != 0) {
				t.Errorf("%s: AVP %v: V-bit does not match the vendor", name, k)
			}
			if a.Flags&diam.AVPFlagProtected != 0 {
				t.Errorf("%s: AVP %v: P-bit set", name, k)
			}
			if k == [2]uint32{628, vendor3GPP} {
				// TS 29.229 §7.2.1: set in a request, never in an answer.
				checked[k] = true
				if !top || a.Mandatory() != request {
					t.Errorf("%s: Supported-Features: M-bit = %v in a request=%v (top level %v)", name, a.Mandatory(), request, top)
				}
			} else if want, ok := mSWx[k]; ok {
				checked[k] = true
				if a.Mandatory() != want {
					t.Errorf("%s: AVP %v: M-bit = %v, want %v", name, k, a.Mandatory(), want)
				}
			}
			if groupCodes[k] {
				walk(name, split(t, a.Data), request, false)
			}
		}
	}
	for i, m := range samples() {
		walk(reflect.TypeOf(m).Elem().Name(), avpsOf(t, encode(t, m)), i%2 == 0, true)
	}
	for k := range mSWx {
		if !checked[k] {
			t.Errorf("AVP %v is not in the samples", k)
		}
	}
}

// Grouped AVPs re-used from s6a carry the S6a flag rules of their members
// (see swx.proto). The one that differs on SWx: Visited-Network-Identifier
// inside Active-APN is sent with the M-bit clear (TS 29.272 table 7.3.1/2),
// while at the top level of SAR it is set (TS 29.229 table 6.3.0.1).
func TestS6aGroupsKeepS6aFlags(t *testing.T) {
	sar := samples()[4].(*ServerAssignmentRequest)
	sar.ActiveApn[0].VisitedNetworkIdentifier = diam.Some([]byte("mnc002.mcc001.3gppnetwork.org"))
	var top, nested *diam.RawAVP
	for _, a := range avpsOf(t, encode(t, sar)) {
		switch a.Code {
		case AVPVisitedNetworkIdentifier:
			top = &a
		case s6a.AVPActiveApn:
			for _, m := range split(t, a.Data) {
				if m.Code == AVPVisitedNetworkIdentifier {
					nested = &m
				}
			}
		}
	}
	if top == nil || nested == nil {
		t.Fatal("Visited-Network-Identifier missing")
	}
	if !top.Mandatory() || nested.Mandatory() {
		t.Fatalf("M-bit: top level %v (want true), inside Active-APN %v (want false)", top.Mandatory(), nested.Mandatory())
	}
	if err := decode(t, encode(t, sar)).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMissingRequired(t *testing.T) {
	type key struct {
		sample       int
		code, vendor uint32
	}
	common := func(i int, request bool) []key {
		k := []key{{i, base.AVPVendorSpecificApplicationId, 0}, {i, base.AVPAuthSessionState, 0}, {i, base.AVPOriginHost, 0}, {i, base.AVPOriginRealm, 0}}
		if request {
			k = append(k, key{i, base.AVPDestinationRealm, 0})
		}
		return k
	}
	var cases []key
	for i := range commands {
		cases = append(cases, common(i, i%2 == 0)...)
	}
	cases = append(cases,
		key{0, base.AVPUserName, 0}, key{0, cx.AVPSipAuthDataItem, vendor3GPP}, key{0, cx.AVPSipNumberAuthItems, vendor3GPP},
		key{1, base.AVPUserName, 0},
		key{2, base.AVPDestinationHost, 0}, key{2, base.AVPUserName, 0},
		key{4, base.AVPUserName, 0}, key{4, cx.AVPServerAssignmentType, vendor3GPP},
		key{5, base.AVPUserName, 0},
		key{6, base.AVPDestinationHost, 0}, key{6, base.AVPUserName, 0}, key{6, cx.AVPDeregistrationReason, vendor3GPP},
	)
	for _, c := range cases {
		b := dropAVP(t, encode(t, samples()[c.sample]), c.code, c.vendor)
		ae := wantResult(t, decode(t, b).Validate(), diam.ResultMissingAVP)
		if ae.AVP.Code != c.code || ae.AVP.Vendor != c.vendor {
			t.Fatalf("sample %d: AVP = %+v, want code %d vendor %d", c.sample, ae.AVP, c.code, c.vendor)
		}
	}
	// { Reason-Code } inside Deregistration-Reason: 5005 with the path.
	rtr := samples()[6].(*RegistrationTerminationRequest)
	b := encode(t, rtr)
	avps := avpsOf(t, b)
	for i := range avps {
		if avps[i].Code == cx.AVPDeregistrationReason {
			avps[i].Data = join(slices.DeleteFunc(split(t, avps[i].Data), func(a diam.RawAVP) bool { return a.Code == cx.AVPReasonCode }))
		}
	}
	ae := wantResult(t, decode(t, withAVPs(t, b, avps)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != cx.AVPReasonCode || !slices.Equal(ae.Path, []uint32{cx.AVPDeregistrationReason}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

// Every member of the SWx grouped AVPs is optional, so the empty
// Non-3GPP-User-Data of §8.1.2.3.2 and the other groups without members
// pass Validate; the "shall be included" rules of §8.2.3.1, §8.2.3.13 and
// §5.2.3.24 are procedure rules.
func TestEmptyGroups(t *testing.T) {
	ppr := samples()[2].(*PushProfileRequest)
	ppr.Non3GppUserData = &Non3GppUserData{TraceInfo: &TraceInfo{}, SubscriptionId: &SubscriptionId{},
		TwanAccessInfo: []TwanAccessInfo{{WlanIdentifier: &WlanIdentifier{}}}}
	if err := decode(t, encode(t, ppr)).Validate(); err != nil {
		t.Fatal(err)
	}
	ppr.Non3GppUserData = &Non3GppUserData{}
	if err := decode(t, encode(t, ppr)).Validate(); err != nil {
		t.Fatal(err)
	}
	ppa := samples()[3].(*PushProfileAnswer)
	ppa.AccessNetworkInfo = &AccessNetworkInfo{}
	if err := decode(t, encode(t, ppa)).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestTooMany(t *testing.T) {
	for _, c := range []struct {
		sample int
		code   uint32
	}{
		{0, AVPRatType},                 // [ RAT-Type ]
		{0, AVPAnTrusted},               // [ AN-Trusted ]
		{0, cx.AVPSipAuthDataItem},      // { SIP-Auth-Data-Item }
		{0, AVPAaaFailureIndication},    // [ AAA-Failure-Indication ]
		{1, AVPTgppAaaServerName},       // [ 3GPP-AAA-Server-Name ]
		{1, AVPOcOlr},                   // [ OC-OLR ]
		{2, AVPNon3gppUserData},         // [ Non-3GPP-User-Data ]
		{2, AVPPprFlags},                // [ PPR-Flags ]
		{3, AVPAccessNetworkInfo},       // [ Access-Network-Info ]
		{3, s6a.AVPLocalTimeZone},       // [ Local-Time-Zone ]
		{4, cx.AVPServerAssignmentType}, // { Server-Assignment-Type }
		{4, s6a.AVPEmergencyServices},   // [ Emergency-Services ]
		{5, AVPDrmp},                    // [ DRMP ]
		{6, cx.AVPDeregistrationReason}, // { Deregistration-Reason }
	} {
		ae := wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[c.sample]), c.code)).Validate(), diam.ResultAVPOccursTooMany)
		if ae.AVP.Code != c.code {
			t.Fatalf("sample %d: AVP = %+v, want %d", c.sample, ae.AVP, c.code)
		}
	}
	// [ SSID ] twice inside Access-Network-Info.
	ppa := samples()[3].(*PushProfileAnswer)
	ppa.AccessNetworkInfo.Unknown = []diam.RawAVP{{Code: AVPSsid, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte("x")}}
	ae := wantResult(t, decode(t, encode(t, ppa)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPSsid || !slices.Equal(ae.Path, []uint32{AVPAccessNetworkInfo}) {
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
	for _, c := range []struct {
		sample int
		code   uint32
		data   []byte
	}{
		{0, AVPAnTrusted, []byte{0, 1}},                                  // Enumerated: four octets
		{0, AVPAaaFailureIndication, []byte{1}},                          // Unsigned32: four octets
		{2, AVPPprFlags, []byte{0, 0, 0, 0, 0}},                          // Unsigned32: four octets
		{4, cx.AVPServerAssignmentType, []byte{0, 0, 13}},                // Enumerated: four octets
		{5, AVPNon3gppUserData, []byte{0, 0, 5, 0xdd, 0xc0, 0, 0, 0x40}}, // Grouped: a member that overruns the group
	} {
		ae := wantResult(t, decode(t, replace(encode(t, samples()[c.sample]), c.code, c.data)).Validate(), diam.ResultInvalidAVPLength)
		if ae.AVP.Code != c.code {
			t.Fatalf("sample %d: AVP = %+v, want %d", c.sample, ae.AVP, c.code)
		}
	}
	// MIP6-Feature-Vector is Unsigned64, two levels deep.
	saa := samples()[5].(*ServerAssignmentAnswer)
	saa.Non3GppUserData = &Non3GppUserData{Unknown: []diam.RawAVP{{Code: AVPMip6FeatureVector, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 1}}}}
	ae := wantResult(t, decode(t, encode(t, saa)).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPMip6FeatureVector || !slices.Equal(ae.Path, []uint32{AVPNon3gppUserData}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

func TestMisplacedFixed(t *testing.T) {
	// Session-Id is < > in every command.
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

	// The same inside Non-3GPP-User-Data, a group with *[ AVP ].
	saa := samples()[5].(*ServerAssignmentAnswer)
	saa.Non3GppUserData.Unknown = []diam.RawAVP{loud}
	ae = wantResult(t, decode(t, encode(t, saa)).Validate(), diam.ResultAVPUnsupported)
	if !slices.Equal(ae.Path, []uint32{AVPNon3gppUserData}) {
		t.Fatalf("Path = %v", ae.Path)
	}

	// A TS 29.229 member of SIP-Auth-Data-Item that the SWx grammar
	// (§8.2.3.9) does not list goes to Unknown; SIP-Authentication-Context
	// has the M-bit (TS 29.229 table 6.3.0.1), so it fails with 5001.
	maa := samples()[1].(*MultimediaAuthAnswer)
	maa.SipAuthDataItem[0].Unknown = []diam.RawAVP{{Code: cx.AVPSipAuthenticationContext, Vendor: vendor3GPP,
		Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte("ctx")}}
	ae = wantResult(t, decode(t, encode(t, maa)).Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != cx.AVPSipAuthenticationContext || !slices.Equal(ae.Path, []uint32{cx.AVPSipAuthDataItem}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

// An RFC 4006 Subscription-Id carries both members; §8.2.3.2 makes both
// optional. Either passes Validate.
func TestSubscriptionIdMembers(t *testing.T) {
	saa := samples()[5].(*ServerAssignmentAnswer)
	saa.Non3GppUserData.SubscriptionId = &SubscriptionId{SubscriptionIdType: diam.Some(SubscriptionIdTypeEndUserNai)}
	got := decode(t, encode(t, saa)).(*ServerAssignmentAnswer)
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if got.Non3GppUserData.SubscriptionId.SubscriptionIdType.V != SubscriptionIdTypeEndUserNai || got.Non3GppUserData.SubscriptionId.SubscriptionIdData.Set {
		t.Fatalf("Subscription-Id = %+v", got.Non3GppUserData.SubscriptionId)
	}
}

// Release 8 peers send only what TS 29.273 V8.16.0 knew (no DRMP, OC-*,
// Load, AN-Trusted, PPR-Flags, ...); their messages must validate.
func TestRelease8Messages(t *testing.T) {
	for _, m := range []diam.Message{
		&MultimediaAuthRequest{
			SessionId: "aaa;1", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: aaa, OriginRealm: realm,
			DestinationRealm: realm, UserName: nai, RatType: diam.Some(s6a.RatTypeWlan), Anid: diam.Some("WLAN"),
			SipAuthDataItem: SipAuthDataItem{SipAuthenticationScheme: diam.Some("EAP-AKA'")}, SipNumberAuthItems: 1,
		},
		&MultimediaAuthAnswer{
			SessionId: "aaa;1", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm,
			OriginHost: hss, OriginRealm: realm, UserName: nai, SipNumberAuthItems: diam.Some(uint32(1)), SipAuthDataItem: []SipAuthDataItem{vector},
		},
		&PushProfileRequest{
			SessionId: "hss;1", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			DestinationHost: aaa, DestinationRealm: realm, UserName: nai,
			Non3GppUserData: &Non3GppUserData{Non3GppIpAccess: diam.Some(Non3GppIpAccessNon3gppSubscriptionBarred)},
		},
		&PushProfileAnswer{SessionId: "hss;1", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: aaa, OriginRealm: realm},
		&ServerAssignmentRequest{
			SessionId: "aaa;2", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: aaa, OriginRealm: realm,
			DestinationRealm: realm, UserName: nai, ServerAssignmentType: cx.ServerAssignmentTypeRegistration,
		},
		&ServerAssignmentAnswer{
			SessionId: "aaa;2", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm,
			OriginHost: hss, OriginRealm: realm, UserName: nai, Non3GppUserData: &Non3GppUserData{
				Non3GppIpAccess: diam.Some(Non3GppIpAccessNon3gppSubscriptionAllowed), Non3GppIpAccessApn: diam.Some(Non3GppIpAccessApnNon3gppApnsEnable),
				ContextIdentifier: diam.Some(uint32(1)), ApnConfiguration: []s6a.ApnConfiguration{apnConfig},
			},
		},
		&RegistrationTerminationRequest{
			SessionId: "hss;2", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			DestinationHost: aaa, DestinationRealm: realm, UserName: nai, DeregistrationReason: cx.DeregistrationReason{ReasonCode: cx.ReasonCodePermanentTermination},
		},
		&RegistrationTerminationAnswer{SessionId: "hss;2", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: aaa, OriginRealm: realm},
	} {
		if err := decode(t, encode(t, m)).Validate(); err != nil {
			t.Fatalf("Rel-8 %T: %v", m, err)
		}
	}
}

func TestNewAnswerFor(t *testing.T) {
	check := func(name string, got diam.Header, wantFlags uint8, sid, wantSid string) {
		t.Helper()
		if got.Flags != wantFlags || got.HopByHop != 0x11 || got.EndToEnd != 0x22 {
			t.Fatalf("%s: answer header = %+v", name, got)
		}
		if sid != wantSid {
			t.Fatalf("%s: Session-Id = %q, want %q", name, sid, wantSid)
		}
	}
	all := diam.FlagRequest | diam.FlagProxiable | diam.FlagRetransmit
	mar := samples()[0].(*MultimediaAuthRequest)
	mar.Header.Flags = all
	maa := NewMultimediaAuthAnswerFor(mar)
	check("MAA", maa.Header, diam.FlagProxiable, maa.SessionId, mar.SessionId)
	ppr := samples()[2].(*PushProfileRequest)
	ppr.Header.Flags = diam.FlagRequest
	ppa := NewPushProfileAnswerFor(ppr)
	check("PPA", ppa.Header, 0, ppa.SessionId, ppr.SessionId)
	sar := samples()[4].(*ServerAssignmentRequest)
	sar.Header.Flags = all
	saa := NewServerAssignmentAnswerFor(sar)
	check("SAA", saa.Header, diam.FlagProxiable, saa.SessionId, sar.SessionId)
	rtr := samples()[6].(*RegistrationTerminationRequest)
	rtr.Header.Flags = all
	rta := NewRegistrationTerminationAnswerFor(rtr)
	check("RTA", rta.Header, diam.FlagProxiable, rta.SessionId, rtr.SessionId)
	// The PPA header in §8.2.2.2 has PXY: Marshal sets it whatever the
	// request carried.
	if h := decode(t, encode(t, ppa)).DiamHeader(); h.Flags != diam.FlagProxiable {
		t.Fatalf("encoded PPA flags = %#x", h.Flags)
	}
}

func TestEnumValuesFollowSpec(t *testing.T) {
	for name, c := range map[string][2]int32{
		"NON_3GPP_SUBSCRIPTION_ALLOWED": {int32(Non3GppIpAccessNon3gppSubscriptionAllowed), 0}, // §8.2.3.3
		"NON_3GPP_SUBSCRIPTION_BARRED":  {int32(Non3GppIpAccessNon3gppSubscriptionBarred), 1},
		"Non_3GPP_APNS_ENABLE":          {int32(Non3GppIpAccessApnNon3gppApnsEnable), 0}, // §8.2.3.4
		"Non_3GPP_APNS_DISABLE":         {int32(Non3GppIpAccessApnNon3gppApnsDisable), 1},
		"TRUSTED":                       {int32(AnTrustedTrusted), 0}, // §5.2.3.9
		"UNTRUSTED":                     {int32(AnTrustedUntrusted), 1},
		"END_USER_E164":                 {int32(SubscriptionIdTypeEndUserE164), 0}, // RFC 4006 §8.47
		"END_USER_IMSI":                 {int32(SubscriptionIdTypeEndUserImsi), 1},
		"END_USER_SIP_URI":              {int32(SubscriptionIdTypeEndUserSipUri), 2},
		"END_USER_NAI":                  {int32(SubscriptionIdTypeEndUserNai), 3},
		"END_USER_PRIVATE":              {int32(SubscriptionIdTypeEndUserPrivate), 4},
		// §8.2.3.12: the SWx values of Server-Assignment-Type (cx's type).
		"AAA_USER_DATA_REQUEST": {int32(cx.ServerAssignmentTypeAaaUserDataRequest), 12},
		"PGW_UPDATE":            {int32(cx.ServerAssignmentTypePgwUpdate), 13},
		"RESTORATION":           {int32(cx.ServerAssignmentTypeRestoration), 14},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
	if got := AnTrustedUntrusted.String(); got != "UNTRUSTED" {
		t.Errorf("String() = %q", got)
	}
}

// Codes the generated constants must carry, restated from table
// 8.2.3.0/1, §5.2.3 (table 5.2.3.1/1) and the specifications of the
// re-used AVPs. The dictionary cross-check in internal/interop covers the
// AVPs go-diameter knows; this covers the ones it does not.
func TestAVPCodes(t *testing.T) {
	for name, c := range map[string][2]uint32{
		"3GPP-AAA-Server-Name":        {AVPTgppAaaServerName, 318},
		"Visited-Network-Identifier":  {AVPVisitedNetworkIdentifier, 600},
		"Confidentiality-Key":         {AVPConfidentialityKey, 625},
		"Integrity-Key":               {AVPIntegrityKey, 626},
		"Supported-Features":          {AVPSupportedFeatures, 628},
		"RAT-Type":                    {AVPRatType, 1032},
		"Non-3GPP-User-Data":          {AVPNon3gppUserData, 1500},
		"Non-3GPP-IP-Access":          {AVPNon3gppIpAccess, 1501},
		"Non-3GPP-IP-Access-APN":      {AVPNon3gppIpAccessApn, 1502},
		"AN-Trusted":                  {AVPAnTrusted, 1503},
		"ANID":                        {AVPAnid, 1504},
		"Trace-Info":                  {AVPTraceInfo, 1505},
		"PPR-Flags":                   {AVPPprFlags, 1508},
		"WLAN-Identifier":             {AVPWlanIdentifier, 1509},
		"TWAN-Access-Info":            {AVPTwanAccessInfo, 1510},
		"Access-Authorization-Flags":  {AVPAccessAuthorizationFlags, 1511},
		"TWAN-Default-APN-Context-Id": {AVPTwanDefaultApnContextId, 1512},
		"AAA-Failure-Indication":      {AVPAaaFailureIndication, 1518},
		"SSID":                        {AVPSsid, 1524},
		"HESSID":                      {AVPHessid, 1525},
		"Access-Network-Info":         {AVPAccessNetworkInfo, 1526},
		"ERP-Authorization":           {AVPErpAuthorization, 1541},
		"BSSID":                       {AVPBssid, 2716},
		"MIP6-Feature-Vector":         {AVPMip6FeatureVector, 124},
		"Operator-Name":               {AVPOperatorName, 126},
		"Location-Information":        {AVPLocationInformation, 127},
		"Location-Data":               {AVPLocationData, 128},
		"DRMP":                        {AVPDrmp, 301},
		"Subscription-Id":             {AVPSubscriptionId, 443},
		"Subscription-Id-Data":        {AVPSubscriptionIdData, 444},
		"Subscription-Id-Type":        {AVPSubscriptionIdType, 450},
		"OC-Supported-Features":       {AVPOcSupportedFeatures, 621},
		"OC-OLR":                      {AVPOcOlr, 623},
		"Load":                        {AVPLoad, 650},
		"Logical-Access-ID":           {AVPLogicalAccessId, 302},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}
