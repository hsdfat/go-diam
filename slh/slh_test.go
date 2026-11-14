package slh

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
		// Table 6.2.2/1: code 8388622; §6.1.8: application id 16777291.
		if h.CommandCode != 8388622 || h.AppID != 16777291 || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
			t.Fatalf("%s: header = %+v", name, h)
		}
		if isRequest := i%2 == 0; (h.Flags&diam.FlagRequest != 0) != isRequest {
			t.Fatalf("%s: R-bit = %v", name, !isRequest)
		}
		// Both headers carry PXY (§6.2.3, §6.2.4).
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
	var rir LcsRoutingInfoRequest
	if err := rir.Unmarshal(encode(t, samples()[0])); err != nil {
		t.Fatal(err)
	}
	if rir.UserName.V != imsi || !bytes.Equal(rir.Msisdn.V, msisdn) || !bytes.Equal(rir.GmlcNumber.V, gmlcNumber) ||
		len(rir.SupportedFeatures) != 1 || rir.SupportedFeatures[0].FeatureListId != 1 ||
		rir.VendorSpecificApplicationId == nil || rir.VendorSpecificApplicationId.AuthApplicationId.V != 16777291 {
		t.Fatalf("RIR = %+v", rir)
	}

	var ria LcsRoutingInfoAnswer
	if err := ria.Unmarshal(encode(t, samples()[1])); err != nil {
		t.Fatal(err)
	}
	sn := ria.ServingNode
	if ria.ResultCode.V != 2001 || !bytes.Equal(ria.Lmsi.V, lmsi) || sn == nil || sn.MmeName.V != mme ||
		sn.SgsnName.V != sgsn || !bytes.Equal(sn.MscNumber.V, mscNumber) || sn.GmlcAddress.V != vGMLC ||
		sn.LcsCapabilitiesSets.V != 0x10 || sn.TgppAaaServerName.Set {
		t.Fatalf("RIA = %+v, Serving-Node %+v", ria, sn)
	}
	if len(ria.AdditionalServingNode) != 2 || ria.AdditionalServingNode[0].TgppAaaServerName.V != aaa ||
		ria.AdditionalServingNode[1].SgsnRealm.V != realm || ria.GmlcAddress.V != hGMLC || ria.PprAddress.V != ppr ||
		ria.RiaFlags.V != 1 || ria.FailedAvp == nil {
		t.Fatalf("RIA = %+v", ria)
	}
}

// mSLh is the M-bit of the 3GPP AVPs on SLh, restated from the tables, not
// read from the schema: table 6.4.1/1 gives "M, V" for LMSI to PPR-Address
// (2400-2407) and "V, M must not" for MME-Realm, SGSN-Name, SGSN-Realm and
// RIA-Flags (2408-2411). For the re-used AVPs of table 6.4.1/2 the rule is
// the defining specification's: MSISDN (701) M, V in TS 29.329 table
// 6.3.1; SGSN-Number (1489) and GMLC-Number (1474) M, V in TS 29.272 table
// 7.3.1/1; 3GPP-AAA-Server-Name (318) M, V in TS 29.273 table 8.2.3.0/1;
// Feature-List-ID (629) and Feature-List (630) M must not in TS 29.229
// table 6.3.0.1. Supported-Features (628) is "May" there and sent clear.
var mSLh = map[uint32]bool{
	2400: true, 2401: true, 2402: true, 2403: true, 2404: true, 2405: true, 2406: true, 2407: true,
	2408: false, 2409: false, 2410: false, 2411: false,
	701: true, 1489: true, 1474: true, 318: true, 628: false, 629: false, 630: false,
}

// groupCodes are the grouped AVPs of the samples, so the flag check can walk
// into them without consulting the schema.
var groupCodes = map[[2]uint32]bool{
	{284, 0}: true, {297, 0}: true, {279, 0}: true, {260, 0}: true,
	{628, vendor3GPP}: true, {2401, vendor3GPP}: true, {2406, vendor3GPP}: true,
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
			if want, ok := mSLh[a.Code]; ok && a.Vendor == vendor3GPP && !inFailed {
				checked[k] = true
				if a.Mandatory() != want {
					t.Errorf("%s: AVP %v: M-bit = %v, want %v", name, k, a.Mandatory(), want)
				}
			}
			if groupCodes[k] {
				walk(name, split(t, a.Data), inFailed || k == [2]uint32{base.AVPFailedAvp, 0})
			}
		}
	}
	for _, m := range samples() {
		walk(reflect.TypeOf(m).Elem().Name(), avpsOf(t, encode(t, m)), false)
	}
	for c := range mSLh {
		if !checked[[2]uint32{c, vendor3GPP}] {
			t.Errorf("3GPP AVP %d is not in the samples", c)
		}
	}
}

func TestMissingRequired(t *testing.T) {
	for _, c := range []struct {
		sample       int
		code, vendor uint32
	}{
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
	// Everything else is optional in both grammars. Table 5.2.1.1/1 wants
	// User-Name or MSISDN, and table 5.2.1.1/2 ties the RIA AVPs to
	// DIAMETER_SUCCESS; those are procedure rules, so a RIR with neither
	// identity and an answer with nothing after Origin-Realm both validate.
	for i, opt := range [][][2]uint32{
		{{base.AVPUserName, 0}, {s6a.AVPMsisdn, vendor3GPP}, {s6a.AVPGmlcNumber, vendor3GPP}, {base.AVPDestinationHost, 0},
			{base.AVPVendorSpecificApplicationId, 0}, {s6a.AVPSupportedFeatures, vendor3GPP}},
		{{base.AVPResultCode, 0}, {base.AVPUserName, 0}, {s6a.AVPMsisdn, vendor3GPP}, {AVPLmsi, vendor3GPP},
			{AVPServingNode, vendor3GPP}, {AVPAdditionalServingNode, vendor3GPP}, {AVPGmlcAddress, vendor3GPP},
			{AVPPprAddress, vendor3GPP}, {AVPRiaFlags, vendor3GPP}, {base.AVPFailedAvp, 0}, {base.AVPVendorSpecificApplicationId, 0}},
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

// §5.2.1.2 step 1: an unauthorized GMLC network is answered with
// Experimental-Result DIAMETER_ERROR_UNAUTHORIZED_REQUESTING_NETWORK (5490,
// §6.3.3.2) and no Result-Code; a user without serving node with
// DIAMETER_ERROR_ABSENT_USER (4201, §6.3.4.1).
func TestErrorAnswers(t *testing.T) {
	for _, code := range []uint32{5490, 4201, 5001} {
		ria := &LcsRoutingInfoAnswer{
			Header: hdr, SessionId: "gmlc;1;9", ExperimentalResult: &base.ExperimentalResult{VendorId: vendor3GPP, ExperimentalResultCode: code},
			AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
		}
		got := decode(t, encode(t, ria)).(*LcsRoutingInfoAnswer)
		if err := got.Validate(); err != nil {
			t.Fatalf("%d: %v", code, err)
		}
		if got.ExperimentalResult.ExperimentalResultCode != code || got.ResultCode.Set || got.ServingNode != nil {
			t.Fatalf("RIA = %+v", got)
		}
	}
}

func TestTooMany(t *testing.T) {
	for _, c := range []struct {
		sample int
		code   uint32
	}{
		{0, base.AVPUserName},  // [ User-Name ]
		{0, s6a.AVPMsisdn},     // [ MSISDN ]
		{0, s6a.AVPGmlcNumber}, // [ GMLC-Number ]
		{1, AVPLmsi},           // [ LMSI ]
		{1, AVPServingNode},    // [ Serving-Node ]
		{1, AVPGmlcAddress},    // [ GMLC-Address ]
		{1, AVPPprAddress},     // [ PPR-Address ]
		{1, AVPRiaFlags},       // [ RIA-Flags ]
		{1, base.AVPFailedAvp}, // [ Failed-AVP ] (current release)
	} {
		ae := wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[c.sample]), c.code)).Validate(), diam.ResultAVPOccursTooMany)
		if ae.AVP.Code != c.code {
			t.Fatalf("sample %d: AVP = %+v, want %d", c.sample, ae.AVP, c.code)
		}
	}
	// [ MME-Name ] twice inside Serving-Node, and [ GMLC-Address ] twice
	// inside an Additional-Serving-Node.
	ria := samples()[1].(*LcsRoutingInfoAnswer)
	ria.ServingNode.Unknown = []diam.RawAVP{{Code: AVPMmeName, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte(mme)}}
	ae := wantResult(t, decode(t, encode(t, ria)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPMmeName || !slices.Equal(ae.Path, []uint32{AVPServingNode}) {
		t.Fatalf("AVPError = %+v", ae)
	}
	ria = samples()[1].(*LcsRoutingInfoAnswer)
	ria.AdditionalServingNode[0].Unknown = []diam.RawAVP{{Code: AVPGmlcAddress, Vendor: vendor3GPP,
		Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte{0, 1, 192, 0, 2, 11}}}
	ae = wantResult(t, decode(t, encode(t, ria)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPGmlcAddress || !slices.Equal(ae.Path, []uint32{AVPAdditionalServingNode}) {
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
	// RIA-Flags is Unsigned32 (four octets).
	ae := wantResult(t, decode(t, replace(encode(t, samples()[1]), AVPRiaFlags, []byte{0, 1})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPRiaFlags {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// PPR-Address is an Address: family 3 is neither IPv4 nor IPv6 (5004).
	ae = wantResult(t, decode(t, replace(encode(t, samples()[1]), AVPPprAddress, []byte{0, 3, 1, 2, 3, 4})).Validate(), diam.ResultInvalidAVPValue)
	if ae.AVP.Code != AVPPprAddress {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// LCS-Capabilities-Sets inside Serving-Node with three octets: 5014 with
	// the path.
	ria := samples()[1].(*LcsRoutingInfoAnswer)
	ria.ServingNode.LcsCapabilitiesSets = diam.Opt[uint32]{}
	ria.ServingNode.Unknown = []diam.RawAVP{{Code: AVPLcsCapabilitiesSets, Vendor: vendor3GPP,
		Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte{0, 0, 1}}}
	ae = wantResult(t, decode(t, encode(t, ria)).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPLcsCapabilitiesSets || !slices.Equal(ae.Path, []uint32{AVPServingNode}) {
		t.Fatalf("AVPError = %+v", ae)
	}
	// Serving-Node is Grouped: a member that overruns the group.
	ae = wantResult(t, decode(t, replace(encode(t, samples()[1]), AVPServingNode, []byte{0, 0, 9, 0x62, 0xc0, 0, 0, 0x40})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPServingNode {
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
	rir := encode(t, samples()[0])
	quiet := diam.RawAVP{Code: 9999, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte("later release")}
	m := decode(t, withAVPs(t, rir, append(avpsOf(t, rir), quiet)))
	if err := m.Validate(); err != nil {
		t.Fatalf("unknown AVP without M-bit: %v", err)
	}
	if got := m.(*LcsRoutingInfoRequest).Unknown; len(got) != 1 || got[0].Code != 9999 {
		t.Fatalf("Unknown = %+v", got)
	}

	loud := quiet
	loud.Flags |= diam.AVPFlagMandatory
	ae := wantResult(t, decode(t, withAVPs(t, rir, append(avpsOf(t, rir), loud))).Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != 9999 {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// The same inside Additional-Serving-Node, a group with *[ AVP ].
	ria := samples()[1].(*LcsRoutingInfoAnswer)
	ria.AdditionalServingNode[1].Unknown = []diam.RawAVP{loud}
	ae = wantResult(t, decode(t, encode(t, ria)).Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != 9999 || !slices.Equal(ae.Path, []uint32{AVPAdditionalServingNode}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

// A peer that sets the M-bit on Supported-Features (as TS 29.229 §7.2.1 does
// for Cx requests) is still understood: the AVP decodes into its field.
func TestSupportedFeaturesMandatory(t *testing.T) {
	msg := encode(t, samples()[0])
	avps := avpsOf(t, msg)
	for i := range avps {
		if avps[i].Code == s6a.AVPSupportedFeatures {
			avps[i].Flags |= diam.AVPFlagMandatory
		}
	}
	got := decode(t, withAVPs(t, msg, avps)).(*LcsRoutingInfoRequest)
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(got.SupportedFeatures) != 1 || len(got.Unknown) != 0 {
		t.Fatalf("RIR = %+v", got)
	}
}

// Release 9 peers send only what TS 29.173 V9.4.0 knew: no SGSN-Name,
// SGSN-Realm or RIA-Flags. Their messages must validate.
func TestRelease9Messages(t *testing.T) {
	for _, m := range []diam.Message{
		&LcsRoutingInfoRequest{
			SessionId: "gmlc;1", AuthSessionState: nsm, OriginHost: gmlc, OriginRealm: realm,
			DestinationRealm: realm, Msisdn: diam.Some(msisdn),
		},
		&LcsRoutingInfoAnswer{
			SessionId: "gmlc;1", ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			UserName: diam.Some(imsi), ServingNode: &ServingNode{MmeName: diam.Some(mme), MmeRealm: diam.Some(realm)},
			AdditionalServingNode: []AdditionalServingNode{{SgsnNumber: diam.Some(sgsnNumber), LcsCapabilitiesSets: diam.Some(uint32(0x10))}},
			GmlcAddress:           diam.Some(hGMLC),
		},
	} {
		if err := decode(t, encode(t, m)).Validate(); err != nil {
			t.Fatalf("Rel-9 %T: %v", m, err)
		}
	}
}

func TestNewAnswerFor(t *testing.T) {
	req := samples()[0].(*LcsRoutingInfoRequest)
	req.Header.Flags = diam.FlagRequest | diam.FlagProxiable | diam.FlagRetransmit
	ans := NewLcsRoutingInfoAnswerFor(req)
	if h := ans.Header; h.Flags != diam.FlagProxiable || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
		t.Fatalf("answer header = %+v", h)
	}
	if ans.SessionId != req.SessionId {
		t.Fatalf("Session-Id = %q, want %q", ans.SessionId, req.SessionId)
	}
}

// AVP codes restated from table 6.4.1/1, and for the re-used AVPs from the
// tables of table 6.4.1/2's specifications. The dictionary cross-check in
// internal/interop covers the ones go-diameter knows.
func TestAVPCodes(t *testing.T) {
	for name, c := range map[string][2]uint32{
		"LMSI":                    {AVPLmsi, 2400},
		"Serving-Node":            {AVPServingNode, 2401},
		"MME-Name":                {AVPMmeName, 2402},
		"MSC-Number":              {AVPMscNumber, 2403},
		"LCS-Capabilities-Sets":   {AVPLcsCapabilitiesSets, 2404},
		"GMLC-Address":            {AVPGmlcAddress, 2405},
		"Additional-Serving-Node": {AVPAdditionalServingNode, 2406},
		"PPR-Address":             {AVPPprAddress, 2407},
		"MME-Realm":               {AVPMmeRealm, 2408},
		"SGSN-Name":               {AVPSgsnName, 2409},
		"SGSN-Realm":              {AVPSgsnRealm, 2410},
		"RIA-Flags":               {AVPRiaFlags, 2411},
		"3GPP-AAA-Server-Name":    {AVPTgppAaaServerName, 318},
		"MSISDN":                  {s6a.AVPMsisdn, 701},
		"SGSN-Number":             {s6a.AVPSgsnNumber, 1489},
		"GMLC-Number":             {s6a.AVPGmlcNumber, 1474},
		"Supported-Features":      {s6a.AVPSupportedFeatures, 628},
		"Feature-List-ID":         {s6a.AVPFeatureListId, 629},
		"Feature-List":            {s6a.AVPFeatureList, 630},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}
