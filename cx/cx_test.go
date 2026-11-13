package cx

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
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

// without returns the AVP sequence data with every AVP of code removed.
func without(t *testing.T, data []byte, code uint32) []byte {
	t.Helper()
	return join(slices.DeleteFunc(split(t, data), func(a diam.RawAVP) bool { return a.Code == code }))
}

// grp is a 3GPP grouped AVP; m is its M-bit.
func grp(code uint32, m bool, data []byte) diam.RawAVP {
	a := diam.RawAVP{Code: code, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: data}
	if m {
		a.Flags |= diam.AVPFlagMandatory
	}
	return a
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

func decode(t *testing.T, b []byte) diam.Message {
	t.Helper()
	m, err := diam.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRoundTrip(t *testing.T) {
	// TS 29.229 table 6.1.1.
	wantCodes := []uint32{300, 300, 301, 301, 302, 302, 303, 303, 304, 304, 305, 305}
	ss := samples()
	if len(ss) != len(wantCodes) {
		t.Fatalf("%d samples, want %d", len(ss), len(wantCodes))
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
		// §6: application id 16777216.
		if h.CommandCode != wantCodes[i] || h.AppID != 16777216 || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
			t.Fatalf("%s: header = %+v", name, h)
		}
		if isRequest := i%2 == 0; (h.Flags&diam.FlagRequest != 0) != isRequest {
			t.Fatalf("%s: R-bit = %v", name, !isRequest)
		}
		// Every Cx/Dx command header carries PXY (§6.1.1-6.1.12).
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
	var sar ServerAssignmentRequest
	if err := sar.Unmarshal(encode(t, samples()[2])); err != nil {
		t.Fatal(err)
	}
	ri := sar.ScscfRestorationInfo.RestorationInfo[0]
	if string(ri.Path) != "<sip:pcscf.ims.example.org;lr>" || ri.SubscriptionInfo == nil ||
		string(ri.PCscfSubscriptionInfo.CallIdSipHeader) != "sub-2" ||
		!sar.ScscfRestorationInfo.RegistrationTimeOut.V.Equal(when) {
		t.Fatalf("SCSCF-Restoration-Info = %+v", sar.ScscfRestorationInfo)
	}
	if got := sar.FailedPcscf.PcscfIpAddress; len(got) != 2 || got[1].String() != "2001:db8::40" {
		t.Fatalf("Failed-PCSCF = %+v", sar.FailedPcscf)
	}
	if sar.ServerAssignmentType != ServerAssignmentTypeRegistration || sar.VendorSpecificApplicationId.AuthApplicationId.V != 16777216 {
		t.Fatalf("SAR = %+v", sar)
	}

	var maa MultimediaAuthAnswer
	if err := maa.Unmarshal(encode(t, samples()[7])); err != nil {
		t.Fatal(err)
	}
	items := maa.SipAuthDataItem
	if len(items) != 3 || len(items[0].SipAuthenticate.V) != 32 || items[1].SipDigestAuthenticate.DigestQop != "auth" ||
		items[1].SipDigestAuthenticate.AlternateDigestAlgorithm.V != "SHA-256" ||
		items[2].FramedInterfaceId.V != 0x0200_5eff_fe00_5301 || string(items[2].LineIdentifier[1]) != "line-2" {
		t.Fatalf("SIP-Auth-Data-Item = %+v", items)
	}

	var saa ServerAssignmentAnswer
	if err := saa.Unmarshal(encode(t, samples()[3])); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saa.UserData.V, userData) || saa.ChargingInformation.SecondaryChargingCollectionFunctionName.V != "aaa://cdf2.ims.example.org" ||
		saa.AllowedWafWwsfIdentities.WebrtcWebServerFunctionName[1] != "wwsf2.example.org" ||
		saa.PriviledgedSenderIndication.V != PriviledgedSenderIndicationPriviledgedSender {
		t.Fatalf("SAA = %+v", saa)
	}
	if saa.OcOlr == nil || saa.OcOlr.OcSequenceNumber != 9 || saa.Load[0].LoadValue.V != 1000 {
		t.Fatalf("OC-OLR / Load = %+v / %+v", saa.OcOlr, saa.Load)
	}
}

// m3GPP is the M-bit table 6.3.0.1 gives each Cx AVP of vendor 3GPP: "Must"
// for 600-626 and Originating-Request (633), "Must not" for the others
// (Supported-Features, 628, is "May"; see TestFlagsFollowSpec). It is
// restated here from the table, not read from the schema.
func m3GPP(code uint32) bool {
	return code >= 600 && code <= 626 || code == 633
}

// mOther is the M-bit of the IETF and ETSI AVPs TS 29.229 re-uses: table
// 6.3.0.1 for Digest-*, DRMP, OC-*, Load; RFC 7155's flag table for
// Framed-*; RFC 7683 and RFC 8583 for the AVPs inside OC-OLR and Load; none
// for Line-Identifier (§6.3.42 gives no flag rule; see cx.proto).
var mOther = map[[2]uint32]bool{
	{8, 0}: true, {96, 0}: true, {97, 0}: true,
	{104, 0}: true, {110, 0}: true, {111, 0}: true, {121, 0}: true,
	{301, 0}: false,
	{621, 0}: false, {622, 0}: false, {623, 0}: false, {624, 0}: false, {625, 0}: false, {626, 0}: false, {627, 0}: false,
	{649, 0}: false, {650, 0}: false, {651, 0}: false, {652, 0}: false,
	{500, vendorETSI}: false,
}

// groupCodes are the grouped AVPs, so the flag check can walk into them
// without consulting the schema.
var groupCodes = map[[2]uint32]bool{
	{284, 0}: true, {297, 0}: true, {279, 0}: true, {260, 0}: true, {621, 0}: true, {623, 0}: true, {650, 0}: true,
}

func init() {
	for _, c := range []uint32{603, 612, 615, 618, 628, 631, 632, 635, 639, 642, 647, 649, 651, 656, 660, 664} {
		groupCodes[[2]uint32{c, vendor3GPP}] = true
	}
}

func TestFlagsFollowSpec(t *testing.T) {
	checked := map[[2]uint32]bool{}
	var walk func(name string, avps []diam.RawAVP, request, top, inFailed bool)
	walk = func(name string, avps []diam.RawAVP, request, top, inFailed bool) {
		for _, a := range avps {
			k := [2]uint32{a.Code, a.Vendor}
			if (a.Vendor != 0) != (a.Flags&diam.AVPFlagVendor != 0) {
				t.Errorf("%s: AVP %v: V-bit does not match the vendor", name, k)
			}
			if a.Flags&diam.AVPFlagProtected != 0 {
				t.Errorf("%s: AVP %v: P-bit set", name, k)
			}
			switch {
			case inFailed:
			case k == [2]uint32{628, vendor3GPP}:
				// §7.2.1: set in a request, never in an answer.
				checked[k] = true
				if !top || a.Mandatory() != request {
					t.Errorf("%s: Supported-Features: M-bit = %v in a request=%v (top level %v)", name, a.Mandatory(), request, top)
				}
			case a.Vendor == vendor3GPP && a.Code >= 600 && a.Code <= 666:
				checked[k] = true
				if a.Mandatory() != m3GPP(a.Code) {
					t.Errorf("%s: AVP %d: M-bit = %v, table 6.3.0.1 says %v", name, a.Code, a.Mandatory(), m3GPP(a.Code))
				}
			default:
				if want, ok := mOther[k]; ok {
					checked[k] = true
					if a.Mandatory() != want {
						t.Errorf("%s: AVP %v: M-bit = %v, want %v", name, k, a.Mandatory(), want)
					}
				}
			}
			if groupCodes[k] {
				walk(name, split(t, a.Data), request, false, inFailed || a.Code == base.AVPFailedAvp && a.Vendor == 0)
			}
		}
	}
	for i, m := range samples() {
		walk(reflect.TypeOf(m).Elem().Name(), avpsOf(t, encode(t, m)), i%2 == 0, true, false)
	}
	for k := range mOther {
		if !checked[k] {
			t.Errorf("re-used AVP %v is not in the samples", k)
		}
	}
	// Every 3GPP AVP of table 6.3.0.1 is in the samples, except
	// Supported-Applications (631), which no command carries. 627 and 636
	// are not Cx AVPs.
	for c := uint32(600); c <= 666; c++ {
		switch c {
		case 627, 636, 631:
			continue
		}
		if !checked[[2]uint32{c, vendor3GPP}] {
			t.Errorf("3GPP AVP %d is not in the samples", c)
		}
	}
}

// Supported-Applications is not carried by any Cx command (§7.3.1 uses it
// in the error answer); its group still round-trips with its flags.
func TestSupportedApplications(t *testing.T) {
	in := SupportedApplications{AuthApplicationId: []uint32{16777216}, AcctApplicationId: []uint32{3},
		VendorSpecificApplicationId: []base.VendorSpecificApplicationId{vsai}}
	a := grp(AVPSupportedApplications, false, in.AppendAVPs(nil))
	var out SupportedApplications
	if err := out.DecodeAVPs(a.Data); err != nil {
		t.Fatal(err)
	}
	if err := out.Validate(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.AuthApplicationId, in.AuthApplicationId) || out.VendorSpecificApplicationId[0].VendorId != 10415 {
		t.Fatalf("Supported-Applications = %+v", out)
	}
}

func TestMissingRequired(t *testing.T) {
	uar := encode(t, samples()[0])
	for _, c := range []struct {
		code, vendor uint32
	}{
		{AVPPublicIdentity, vendor3GPP},
		{AVPVisitedNetworkIdentifier, vendor3GPP},
		{base.AVPUserName, 0},
		// §6.1 NOTE: still { } in every command.
		{base.AVPVendorSpecificApplicationId, 0},
	} {
		avps := slices.DeleteFunc(avpsOf(t, uar), func(a diam.RawAVP) bool { return a.Code == c.code && a.Vendor == c.vendor })
		ae := wantResult(t, decode(t, withAVPs(t, uar, avps)).Validate(), diam.ResultMissingAVP)
		if ae.AVP.Code != c.code || ae.AVP.Vendor != c.vendor {
			t.Fatalf("AVP = %+v, want code %d vendor %d", ae.AVP, c.code, c.vendor)
		}
	}

	// MAR: { SIP-Auth-Data-Item }, { SIP-Number-Auth-Items }, { Server-Name }.
	mar := encode(t, samples()[6])
	for _, code := range []uint32{AVPSipAuthDataItem, AVPSipNumberAuthItems, AVPServerName} {
		avps := slices.DeleteFunc(avpsOf(t, mar), func(a diam.RawAVP) bool { return a.Code == code })
		if ae := wantResult(t, decode(t, withAVPs(t, mar, avps)).Validate(), diam.ResultMissingAVP); ae.AVP.Code != code {
			t.Fatalf("AVP = %+v, want %d", ae.AVP, code)
		}
	}
}

// A Digest-HA1 missing two groups deep in an MAA: 5005 with the path.
func TestMissingInNestedGroup(t *testing.T) {
	d := digestItem()
	sdaData := without(t, d.SipDigestAuthenticate.AppendAVPs(nil), AVPDigestHa1)
	item := diam.AppendRaw(nil, grp(AVPSipDigestAuthenticate, false, sdaData))
	maa := &MultimediaAuthAnswer{SessionId: "s", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
		Unknown: []diam.RawAVP{grp(AVPSipAuthDataItem, true, item)}}
	ae := wantResult(t, decode(t, encode(t, maa)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPDigestHa1 || ae.AVP.Vendor != 0 {
		t.Fatalf("AVP = %+v, want Digest-HA1", ae.AVP)
	}
	if want := []uint32{AVPSipAuthDataItem, AVPSipDigestAuthenticate}; !slices.Equal(ae.Path, want) {
		t.Fatalf("Path = %v, want %v", ae.Path, want)
	}

	// { Reason-Code } inside Deregistration-Reason (§6.3.16).
	rtr := &RegistrationTerminationRequest{SessionId: "s", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: hss,
		OriginRealm: realm, DestinationRealm: realm, UserName: impi}
	b := encode(t, rtr)
	avps := slices.DeleteFunc(avpsOf(t, b), func(a diam.RawAVP) bool { return a.Code == AVPDeregistrationReason })
	avps = append(avps, grp(AVPDeregistrationReason, true, nil))
	ae = wantResult(t, decode(t, withAVPs(t, b, avps)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPReasonCode || !slices.Equal(ae.Path, []uint32{AVPDeregistrationReason}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

// 1*{ Restoration-Info } (§6.3.46): an SCSCF-Restoration-Info without one is 5005.
func TestRestorationInfoNeedsOne(t *testing.T) {
	saa := samples()[3].(*ServerAssignmentAnswer)
	saa.ScscfRestorationInfo = []ScscfRestorationInfo{{UserName: impi}}
	ae := wantResult(t, decode(t, encode(t, saa)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPRestorationInfo || !slices.Equal(ae.Path, []uint32{AVPScscfRestorationInfo}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

func TestTooMany(t *testing.T) {
	// { Public-Identity } twice in a UAR.
	uar := encode(t, samples()[0])
	avps := avpsOf(t, uar)
	for _, a := range avps {
		if a.Code == AVPPublicIdentity {
			avps = append(avps, a)
			break
		}
	}
	ae := wantResult(t, decode(t, withAVPs(t, uar, avps)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPPublicIdentity {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// [ Failed-AVP ] (Release 14 on) twice in a PPA.
	ppa := encode(t, samples()[11])
	avps = avpsOf(t, ppa)
	for _, a := range avps {
		if a.Code == base.AVPFailedAvp {
			avps = append(avps, a)
			break
		}
	}
	if ae := wantResult(t, decode(t, withAVPs(t, ppa, avps)).Validate(), diam.ResultAVPOccursTooMany); ae.AVP.Code != base.AVPFailedAvp {
		t.Fatalf("AVP = %+v, want Failed-AVP", ae.AVP)
	}

	// [ Digest-Realm ] is { } in SIP-Digest-Authenticate: a second one inside PPR.
	ppr := samples()[10].(*PushProfileRequest)
	ppr.SipAuthDataItem.SipDigestAuthenticate.Unknown = []diam.RawAVP{
		{Code: AVPDigestRealm, Flags: diam.AVPFlagMandatory, Data: []byte(realm)},
	}
	ae = wantResult(t, decode(t, encode(t, ppr)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPDigestRealm || !slices.Equal(ae.Path, []uint32{AVPSipAuthDataItem, AVPSipDigestAuthenticate}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

func TestBadValues(t *testing.T) {
	uar := encode(t, samples()[0])
	replace := func(code uint32, data []byte) []byte {
		avps := avpsOf(t, uar)
		for i := range avps {
			if avps[i].Code == code {
				avps[i].Data = data
			}
		}
		return withAVPs(t, uar, avps)
	}
	// UAR-Flags is Unsigned32: two octets is a bad length.
	ae := wantResult(t, decode(t, replace(AVPUarFlags, []byte{0, 1})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPUarFlags {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// PCSCF-IP-Address with address family 8 (E.164) inside Failed-PCSCF.
	fp := diam.AppendRaw(nil, diam.RawAVP{Code: AVPPcscfIpAddress, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte{0, 8, '1', '2'}})
	sar := samples()[2].(*ServerAssignmentRequest)
	sar.FailedPcscf = nil
	sar.Unknown = []diam.RawAVP{grp(AVPFailedPcscf, false, fp)}
	ae = wantResult(t, decode(t, encode(t, sar)).Validate(), diam.ResultInvalidAVPValue)
	if ae.AVP.Code != AVPPcscfIpAddress || !slices.Equal(ae.Path, []uint32{AVPFailedPcscf}) {
		t.Fatalf("AVPError = %+v", ae)
	}

	// Framed-Interface-Id is Unsigned64.
	item := diam.AppendRaw(nil, diam.RawAVP{Code: AVPFramedInterfaceId, Flags: diam.AVPFlagMandatory, Data: []byte{1, 2, 3, 4}})
	maa := &MultimediaAuthAnswer{SessionId: "s", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
		Unknown: []diam.RawAVP{grp(AVPSipAuthDataItem, true, item)}}
	ae = wantResult(t, decode(t, encode(t, maa)).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPFramedInterfaceId || !slices.Equal(ae.Path, []uint32{AVPSipAuthDataItem}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

func TestMisplacedFixed(t *testing.T) {
	// Session-Id is < > in every command.
	uar := encode(t, samples()[0])
	avps := avpsOf(t, uar)
	avps[0], avps[1] = avps[1], avps[0]
	ae := wantResult(t, decode(t, withAVPs(t, uar, avps)).Validate(), diam.ResultInvalidAVPValue)
	if ae.AVP.Code != base.AVPSessionId {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// OC-OLR ::= < OC-Sequence-Number > < OC-Report-Type > ... (RFC 7683 §7.3).
	olr := split(t, ocOlr.AppendAVPs(nil))
	olr[0], olr[1] = olr[1], olr[0]
	lia := &LocationInfoAnswer{SessionId: "s", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
		Unknown: []diam.RawAVP{{Code: AVPOcOlr, Data: join(olr)}}}
	ae = wantResult(t, decode(t, encode(t, lia)).Validate(), diam.ResultInvalidAVPValue)
	if !slices.Equal(ae.Path, []uint32{AVPOcOlr}) {
		t.Fatalf("Path = %v", ae.Path)
	}
}

func TestUnknownAVPs(t *testing.T) {
	uar := encode(t, samples()[0])
	quiet := diam.RawAVP{Code: 9999, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte("later release")}
	m := decode(t, withAVPs(t, uar, append(avpsOf(t, uar), quiet)))
	if err := m.Validate(); err != nil {
		t.Fatalf("unknown AVP without M-bit: %v", err)
	}
	if got := m.(*UserAuthorizationRequest).Unknown; len(got) != 1 || got[0].Code != 9999 {
		t.Fatalf("Unknown = %+v", got)
	}

	loud := quiet
	loud.Flags |= diam.AVPFlagMandatory
	ae := wantResult(t, decode(t, withAVPs(t, uar, append(avpsOf(t, uar), loud))).Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != 9999 {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// The same inside SIP-Auth-Data-Item, a group with *[ AVP ].
	mar := samples()[6].(*MultimediaAuthRequest)
	mar.SipAuthDataItem.Unknown = []diam.RawAVP{loud}
	ae = wantResult(t, decode(t, encode(t, mar)).Validate(), diam.ResultAVPUnsupported)
	if !slices.Equal(ae.Path, []uint32{AVPSipAuthDataItem}) {
		t.Fatalf("Path = %v", ae.Path)
	}

	// Wildcarded-IMPU (TS 29.329 code 636) is not a Cx AVP: without the
	// M-bit, as TS 29.329 sends it, it is kept and ignored.
	impuW := diam.RawAVP{Code: 636, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte("sip:!.*!@ims.example.org")}
	saa := encode(t, samples()[3])
	m = decode(t, withAVPs(t, saa, append(avpsOf(t, saa), impuW)))
	if err := m.Validate(); err != nil {
		t.Fatalf("Wildcarded-IMPU without M-bit: %v", err)
	}
}

// Release 8 peers send only what TS 29.229 V8.15.0 knew; their messages
// must validate.
func TestRelease8Messages(t *testing.T) {
	for _, m := range []diam.Message{
		&UserAuthorizationRequest{
			SessionId: "icscf;1", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: icscf, OriginRealm: realm,
			DestinationRealm: realm, UserName: impi, PublicIdentity: impu, VisitedNetworkIdentifier: []byte("visited.example.net"),
		},
		&ServerAssignmentAnswer{
			SessionId: "scscf;1", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm,
			OriginHost: hss, OriginRealm: realm, UserName: diam.Some(impi), UserData: diam.Some(userData),
		},
		&MultimediaAuthRequest{
			SessionId: "scscf;2", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: scscf, OriginRealm: realm,
			DestinationRealm: realm, UserName: impi, PublicIdentity: impu,
			SipAuthDataItem: SipAuthDataItem{SipAuthenticationScheme: diam.Some(akaScheme)}, SipNumberAuthItems: 1, ServerName: scscfURI,
		},
		// V8.15.0 §6.1.9: { Destination-Host } after Origin-Realm.
		&RegistrationTerminationRequest{
			SessionId: "hss;1", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			DestinationHost: diam.Some(scscf), DestinationRealm: realm, UserName: impi,
			DeregistrationReason: DeregistrationReason{ReasonCode: ReasonCodePermanentTermination},
		},
	} {
		if err := decode(t, encode(t, m)).Validate(); err != nil {
			t.Fatalf("Rel-8 %T: %v", m, err)
		}
	}
}

// RTR: TS 29.229 V18.1.0 omits Destination-Host, TS 29.228 V18.0.0 table
// 6.1.3.1 makes it M. Both readings validate, and the AVP is decoded into
// its field, not left in Unknown (where its M-bit would give 5001).
func TestRTRDestinationHost(t *testing.T) {
	rtr := samples()[8].(*RegistrationTerminationRequest)
	got := decode(t, encode(t, rtr)).(*RegistrationTerminationRequest)
	if got.DestinationHost.V != scscf || len(got.Unknown) != 0 {
		t.Fatalf("Destination-Host = %+v, Unknown = %+v", got.DestinationHost, got.Unknown)
	}
	b := encode(t, rtr)
	avps := slices.DeleteFunc(avpsOf(t, b), func(a diam.RawAVP) bool { return a.Code == base.AVPDestinationHost })
	if err := decode(t, withAVPs(t, b, avps)).Validate(); err != nil {
		t.Fatalf("RTR without Destination-Host: %v", err)
	}
	// It is written after Origin-Realm and before Destination-Realm.
	var order []uint32
	for _, a := range avpsOf(t, b) {
		if a.Vendor == 0 && (a.Code == base.AVPOriginRealm || a.Code == base.AVPDestinationHost || a.Code == base.AVPDestinationRealm) {
			order = append(order, a.Code)
		}
	}
	if want := []uint32{base.AVPOriginRealm, base.AVPDestinationHost, base.AVPDestinationRealm}; !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestNewAnswerFor(t *testing.T) {
	ss := samples()
	for i := 0; i < len(ss); i += 2 {
		req := ss[i]
		req.DiamHeader().Flags = diam.FlagRequest | diam.FlagProxiable | diam.FlagRetransmit
		var ans diam.Message
		switch r := req.(type) {
		case *UserAuthorizationRequest:
			ans = NewUserAuthorizationAnswerFor(r)
		case *ServerAssignmentRequest:
			ans = NewServerAssignmentAnswerFor(r)
		case *LocationInfoRequest:
			ans = NewLocationInfoAnswerFor(r)
		case *MultimediaAuthRequest:
			ans = NewMultimediaAuthAnswerFor(r)
		case *RegistrationTerminationRequest:
			ans = NewRegistrationTerminationAnswerFor(r)
		case *PushProfileRequest:
			ans = NewPushProfileAnswerFor(r)
		default:
			t.Fatalf("no answer constructor for %T", req)
		}
		if reflect.TypeOf(ans) != reflect.TypeOf(ss[i+1]) {
			t.Fatalf("answer to %T is %T", req, ans)
		}
		h := ans.DiamHeader()
		if h.Flags != diam.FlagProxiable || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
			t.Fatalf("%T header = %+v", ans, h)
		}
		got := reflect.ValueOf(ans).Elem().FieldByName("SessionId").String()
		want := reflect.ValueOf(req).Elem().FieldByName("SessionId").String()
		if got != want {
			t.Fatalf("%T Session-Id = %q, want %q", ans, got, want)
		}
	}
}

func TestEnumValuesFollowSpec(t *testing.T) {
	for name, c := range map[string][2]int32{
		"Server-Assignment-Type NO_ASSIGNMENT (§6.3.15)":                    {int32(ServerAssignmentTypeNoAssignment), 0},
		"Server-Assignment-Type UNREGISTERED_USER":                          {int32(ServerAssignmentTypeUnregisteredUser), 3},
		"Server-Assignment-Type ADMINISTRATIVE_DEREGISTRATION":              {int32(ServerAssignmentTypeAdministrativeDeregistration), 8},
		"Server-Assignment-Type DEREGISTRATION_TOO_MUCH_DATA":               {int32(ServerAssignmentTypeDeregistrationTooMuchData), 11},
		"Server-Assignment-Type RESTORATION":                                {int32(ServerAssignmentTypeRestoration), 14},
		"Reason-Code REMOVE_S-CSCF (§6.3.17)":                               {int32(ReasonCodeRemoveSCscf), 3},
		"User-Authorization-Type REGISTRATION_AND_CAPABILITIES (§6.3.24)":   {int32(UserAuthorizationTypeRegistrationAndCapabilities), 2},
		"User-Data-Already-Available USER_DATA_ALREADY_AVAILABLE (§6.3.26)": {int32(UserDataAlreadyAvailableUserDataAlreadyAvailable), 1},
		"Originating-Request ORIGINATING (§6.3.34)":                         {int32(OriginatingRequestOriginating), 0},
		"Loose-Route-Indication LOOSE_ROUTE_REQUIRED (§6.3.45)":             {int32(LooseRouteIndicationLooseRouteRequired), 1},
		"Multiple-Registration-Indication MULTIPLE_REGISTRATION (§6.3.51)":  {int32(MultipleRegistrationIndicationMultipleRegistration), 1},
		"Session-Priority PRIORITY-4 (§6.3.56)":                             {int32(SessionPriorityPriority4), 4},
		"Priviledged-Sender-Indication PRIVILEDGED_SENDER (§6.3.58)":        {int32(PriviledgedSenderIndicationPriviledgedSender), 1},
		"DRMP PRIORITY_15 (RFC 7944 §9.1)":                                  {int32(DrmpPriority15), 15},
		"OC-Report-Type REALM_REPORT (RFC 7683 §7.6)":                       {int32(OcReportTypeRealmReport), 1},
		"Load-Type PEER (RFC 8583 §7.2)":                                    {int32(LoadTypePeer), 1},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
	if got := ServerAssignmentTypeReRegistration.String(); got != "RE_REGISTRATION" {
		t.Errorf("ServerAssignmentTypeReRegistration.String() = %q", got)
	}
	if got := ServerAssignmentType(99).String(); got != "99" {
		t.Errorf("unknown Server-Assignment-Type String() = %q", got)
	}
}

// Codes the generated constants must carry, restated from TS 29.229 table
// 6.3.0.1, §6.3.42 and the RFCs of the re-used AVPs. The dictionary
// cross-check in internal/interop covers the AVPs go-diameter knows; this
// covers the ones it does not.
func TestAVPCodes(t *testing.T) {
	for name, c := range map[string][2]uint32{
		"Public-Identity":                             {AVPPublicIdentity, 601},
		"SIP-Authentication-Context":                  {AVPSipAuthenticationContext, 611},
		"Charging-Information":                        {AVPChargingInformation, 618},
		"Primary-Event-Charging-Function-Name":        {AVPPrimaryEventChargingFunctionName, 619},
		"Secondary-Event-Charging-Function-Name":      {AVPSecondaryEventChargingFunctionName, 620},
		"Primary-Charging-Collection-Function-Name":   {AVPPrimaryChargingCollectionFunctionName, 621},
		"Secondary-Charging-Collection-Function-Name": {AVPSecondaryChargingCollectionFunctionName, 622},
		"User-Authorization-Type":                     {AVPUserAuthorizationType, 623},
		"User-Data-Already-Available":                 {AVPUserDataAlreadyAvailable, 624},
		"Supported-Applications":                      {AVPSupportedApplications, 631},
		"Associated-Identities":                       {AVPAssociatedIdentities, 632},
		"Originating-Request":                         {AVPOriginatingRequest, 633},
		"Wildcarded-Public-Identity":                  {AVPWildcardedPublicIdentity, 634},
		"SIP-Digest-Authenticate":                     {AVPSipDigestAuthenticate, 635},
		"UAR-Flags":                                   {AVPUarFlags, 637},
		"Loose-Route-Indication":                      {AVPLooseRouteIndication, 638},
		"SCSCF-Restoration-Info":                      {AVPScscfRestorationInfo, 639},
		"Path":                                        {AVPPath, 640},
		"Contact":                                     {AVPContact, 641},
		"Subscription-Info":                           {AVPSubscriptionInfo, 642},
		"Call-ID-SIP-Header":                          {AVPCallIdSipHeader, 643},
		"Record-Route":                                {AVPRecordRoute, 646},
		"Associated-Registered-Identities":            {AVPAssociatedRegisteredIdentities, 647},
		"Multiple-Registration-Indication":            {AVPMultipleRegistrationIndication, 648},
		"Restoration-Info":                            {AVPRestorationInfo, 649},
		"Identity-with-Emergency-Registration":        {AVPIdentityWithEmergencyRegistration, 651},
		"Priviledged-Sender-Indication":               {AVPPriviledgedSenderIndication, 652},
		"LIA-Flags":                                   {AVPLiaFlags, 653},
		"Initial-CSeq-Sequence-Number":                {AVPInitialCseqSequenceNumber, 654},
		"SAR-Flags":                                   {AVPSarFlags, 655},
		"Allowed-WAF-WWSF-Identities":                 {AVPAllowedWafWwsfIdentities, 656},
		"WebRTC-Authentication-Function-Name":         {AVPWebrtcAuthenticationFunctionName, 657},
		"WebRTC-Web-Server-Function-Name":             {AVPWebrtcWebServerFunctionName, 658},
		"RTR-Flags":                                   {AVPRtrFlags, 659},
		"P-CSCF-Subscription-Info":                    {AVPPCscfSubscriptionInfo, 660},
		"Registration-Time-Out":                       {AVPRegistrationTimeOut, 661},
		"Alternate-Digest-Algorithm":                  {AVPAlternateDigestAlgorithm, 662},
		"Alternate-Digest-HA1":                        {AVPAlternateDigestHa1, 663},
		"Failed-PCSCF":                                {AVPFailedPcscf, 664},
		"PCSCF-FQDN":                                  {AVPPcscfFqdn, 665},
		"PCSCF-IP-Address":                            {AVPPcscfIpAddress, 666},
		"Line-Identifier (ETSI)":                      {AVPLineIdentifier, 500},
		"Digest-Realm":                                {AVPDigestRealm, 104},
		"Digest-QoP":                                  {AVPDigestQop, 110},
		"Digest-Algorithm":                            {AVPDigestAlgorithm, 111},
		"Digest-HA1":                                  {AVPDigestHa1, 121},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}
