package s6t

import (
	"bytes"
	"errors"
	"net/netip"
	"reflect"
	"slices"
	"testing"

	"github.com/hsdfat/go-diam/base"
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

// avp3 is a 3GPP AVP; m is its M-bit.
func avp3(code uint32, m bool, data []byte) diam.RawAVP {
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
	// TS 29.336 table 8.2.2-1.
	wantCodes := []uint32{8388718, 8388718, 8388719, 8388719, 8388726, 8388726}
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
		// §8.1.8: application id 16777345.
		if h.CommandCode != wantCodes[i] || h.AppID != 16777345 || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
			t.Fatalf("%s: header = %+v", name, h)
		}
		// RIR is a request although §8.2.5 writes its header without REQ.
		if isRequest := i%2 == 0; (h.Flags&diam.FlagRequest != 0) != isRequest {
			t.Fatalf("%s: R-bit = %v", name, !isRequest)
		}
		// Every S6t command header carries PXY (§8.2.3-8.2.8).
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
		if u := reflect.ValueOf(m).Elem().FieldByName("Unknown"); u.Len() != 0 {
			t.Fatalf("%s: Unknown = %v", name, u.Interface())
		}
	}
}

func TestDecodedValues(t *testing.T) {
	var cir ConfigurationInformationRequest
	if err := cir.Unmarshal(encode(t, samples()[0])); err != nil {
		t.Fatal(err)
	}
	mec := cir.MonitoringEventConfiguration[0]
	if cir.UserIdentifier.ExternalIdentifier.V != groupID || cir.UserIdentifier.TypeOfExternalIdentifier.V != 1 ||
		mec.ScefReferenceIdExt.V != 1<<40 || !slices.Equal(mec.ScefReferenceIdForDeletion, []uint32{38, 39}) ||
		!mec.MonitoringDuration.V.Equal(expiry) || mec.UeReachabilityConfiguration.MaximumLatency.V != 120 ||
		mec.PlmnIdRequested.V != PlmnIdRequestedFalse || mec.IncludeIdentifiers.ExternalIdentifier[0] != "device-3@iot.example.org" ||
		cir.MonitoringEventConfiguration[1].MonitoringType != 2 ||
		cir.AeseCommunicationPattern[0].CommunicationPatternSet[0].ScheduledCommunicationTime[1].DayOfWeekMask.V != 0x41 ||
		cir.AdditionalIdentifiers.ExternalIdentifier[0] != "group-2@iot.example.org" || cir.SuggestedNetworkConfiguration.ScefReferenceId != 41 ||
		cir.VendorSpecificApplicationId == nil || cir.VendorSpecificApplicationId.AuthApplicationId.V != 16777345 {
		t.Fatalf("CIR = %+v", cir)
	}

	var cia ConfigurationInformationAnswer
	if err := cia.Unmarshal(encode(t, samples()[1])); err != nil {
		t.Fatal(err)
	}
	mer := cia.MonitoringEventReport[0]
	mme := mer.EpsLocationInformation.MmeLocationInformation
	if cia.NumberOfUes.V != 12 || mer.TerminalInformation.Imei.V != "3534900698733" || mme.UserCsgInformation.CsgId != 7 ||
		!bytes.Equal(mme.EnodebId.V, []byte{0, 1, 2}) || mer.IdleStatusIndication.EdrxCycleLength.RatType != RatTypeEutranNbIot ||
		!mer.IdleStatusIndication.IdleStatusTimestamp.V.Equal(stamp) ||
		mer.PdnConnectivityStatusReport[0].ServedPartyIpAddress[1] != netip.MustParseAddr("2001:db8::") ||
		mer.ServiceReport[0].ServiceResult.ServiceResultCode.V != 5512 || cia.SupportedServices[1].NodeType.V != 1 ||
		cia.EnhancedCoverageRestrictionData.EnhancedCoverageRestriction.AllowedPlmnList == nil ||
		cia.ImsiGroupId[0].GroupServiceId != 3 || cia.Load[0].LoadValue.V != 1000 {
		t.Fatalf("CIA = %+v", cia)
	}

	var rir ReportingInformationRequest
	if err := rir.Unmarshal(encode(t, samples()[2])); err != nil {
		t.Fatal(err)
	}
	item := rir.GroupReport[0].GroupReportItem[0]
	if item.UserIdentifier.ExternalIdentifier.V != extID || item.UpdatedNetworkConfiguration.DlBufferingSuggestedPacketCount.V != -1 ||
		!item.ReportingTimeStamp.V.Equal(stamp) || item.S6THssCause.V != 0x02 || rir.RirFlags.V != 0x05 ||
		rir.MonitoringEventReport[0].EpsLocationInformation.SgsnLocationInformation.RoutingAreaIdentity.V == nil {
		t.Fatalf("RIR = %+v", rir)
	}

	var nia NiddInformationAnswer
	if err := nia.Unmarshal(encode(t, samples()[5])); err != nil {
		t.Fatal(err)
	}
	if nia.ExperimentalResult.ExperimentalResultCode != 5510 || !nia.NiddAuthorizationResponse.GrantedValidityTime.V.Equal(granted) ||
		nia.NiaFlags.V != 1 || nia.GroupUserIdentifier[0].UserName.V != imsi {
		t.Fatalf("NIA = %+v", nia)
	}
}

// mS6t is the M-bit TS 29.336 gives each AVP of vendor 3GPP that S6t
// carries: "M,V" for 3102 and 3111 (table 6.4.1/1) and 3113-3154 (table
// 8.4.1-1), "V" with M "must not" for 3155-3189; for the re-used AVPs the
// M-bit column of table 8.4.1-2 ("Must not set") or, where it is empty,
// the defining specification's rule. Supported-Features (628) is per
// message; see TestFlagsFollowSpec. It is restated here from the tables,
// not read from the schema.
var mS6t = map[uint32]bool{
	// TS 29.229 table 6.3.0.1 ("May"); TS 29.329 table 6.3.1; TS 32.299 table 7.2
	629: false, 630: false, 701: true, 857: true,
	// TS 29.212 table 5.3.1
	1032: false,
	// TS 29.272 table 7.3.1/1
	1402: true, 1403: true, 1407: true, 1437: true, 1471: true,
	1496: false, 1600: false, 1601: false, 1602: false, 1603: false, 1604: false, 1605: false, 1607: false,
	1610: false, 1611: false, 1676: false, 1677: false, 1678: false, 1691: false, 1692: false,
	// table 8.4.1-2 ("Must not set")
	848: false, 1401: false, 1456: false, 1619: false, 1674: false, 1675: false, 1681: false, 1682: false,
	4013: false, 4209: false, 4322: false, 4324: false, 4325: false,
	// TS 32.299 table 7.2
	2317: true, 2318: true, 2319: true,
	// table 6.4.1/1
	3102: true, 3111: true,
	// TS 29.338 table 5.3.3.1; TS 29.217 table 5.3.1; TS 29.128 table 6.4.1
	3329: false, 4008: true, 4323: false,
}

func init() {
	for c := uint32(3113); c <= 3189; c++ {
		mS6t[c] = c <= 3154
	}
}

// mIETF is the M-bit of the IETF AVPs S6t re-uses: table 8.4.1-2 for DRMP,
// OC-Supported-Features, OC-OLR and Load ("Must not set"); RFC 5778 for
// Service-Selection (M); RFC 7683 and RFC 8583 for the AVPs inside OC-OLR
// and Load; and clear for the RFC 5777 time AVPs, which have no rule.
var mIETF = map[uint32]bool{
	301: false, 493: true, 561: false, 562: false, 563: false,
	621: false, 622: false, 623: false, 624: false, 625: false, 626: false, 627: false,
	649: false, 650: false, 651: false, 652: false,
}

// groupCodes are the grouped AVPs, so the flag check can walk into them
// without consulting the schema.
var groupCodes = map[[2]uint32]bool{
	{284, 0}: true, {297, 0}: true, {279, 0}: true, {260, 0}: true, {621, 0}: true, {623, 0}: true, {650, 0}: true,
	{628, vendor3GPP}: true, {1401, vendor3GPP}: true, {1496, vendor3GPP}: true, {1600, vendor3GPP}: true,
	{1601, vendor3GPP}: true, {1675, vendor3GPP}: true, {1691, vendor3GPP}: true, {2319, vendor3GPP}: true,
	{3102, vendor3GPP}: true, {3113, vendor3GPP}: true, {3114, vendor3GPP}: true, {3118, vendor3GPP}: true,
	{3120, vendor3GPP}: true, {3121, vendor3GPP}: true, {3122, vendor3GPP}: true, {3123, vendor3GPP}: true,
	{3129, vendor3GPP}: true, {3135, vendor3GPP}: true, {3142, vendor3GPP}: true, {3143, vendor3GPP}: true,
	{3146, vendor3GPP}: true, {3150, vendor3GPP}: true, {3151, vendor3GPP}: true, {3152, vendor3GPP}: true,
	{3155, vendor3GPP}: true, {3156, vendor3GPP}: true, {3157, vendor3GPP}: true, {3158, vendor3GPP}: true,
	{3161, vendor3GPP}: true, {3165, vendor3GPP}: true, {3166, vendor3GPP}: true, {3169, vendor3GPP}: true,
	{3170, vendor3GPP}: true, {3171, vendor3GPP}: true, {3173, vendor3GPP}: true, {3177, vendor3GPP}: true,
	{3178, vendor3GPP}: true, {3180, vendor3GPP}: true, {3181, vendor3GPP}: true, {3184, vendor3GPP}: true,
	{3188, vendor3GPP}: true, {3189, vendor3GPP}: true, {4322, vendor3GPP}: true,
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
				// TS 29.229 §7.2.1 (applied by §8.1.9): set in a request, never in an answer.
				checked[k] = true
				if !top || a.Mandatory() != request {
					t.Errorf("%s: Supported-Features: M-bit = %v in a request=%v (top level %v)", name, a.Mandatory(), request, top)
				}
			case a.Vendor == vendor3GPP:
				if want, ok := mS6t[a.Code]; ok {
					checked[k] = true
					if a.Mandatory() != want {
						t.Errorf("%s: AVP %d: M-bit = %v, want %v", name, a.Code, a.Mandatory(), want)
					}
				}
			case a.Vendor == 0:
				if want, ok := mIETF[a.Code]; ok {
					checked[k] = true
					if a.Mandatory() != want {
						t.Errorf("%s: AVP %d: M-bit = %v, want %v", name, a.Code, a.Mandatory(), want)
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
	// Every AVP of tables 8.4.1-1 and 8.4.1-2 is in the samples.
	for c := range mIETF {
		if !checked[[2]uint32{c, 0}] {
			t.Errorf("IETF AVP %d is not in the samples", c)
		}
	}
	for c := range mS6t {
		if !checked[[2]uint32{c, vendor3GPP}] {
			t.Errorf("3GPP AVP %d is not in the samples", c)
		}
	}
	if !checked[[2]uint32{628, vendor3GPP}] {
		t.Error("Supported-Features is not in the samples")
	}
}

func TestMissingRequired(t *testing.T) {
	for _, c := range []struct {
		sample       int
		code, vendor uint32
	}{
		{0, AVPUserIdentifier, vendor3GPP},
		{0, base.AVPDestinationRealm, 0},
		{0, base.AVPAuthSessionState, 0},
		{1, base.AVPOriginRealm, 0},
		// §8.1.6: HSS-initiated requests carry Destination-Host.
		{2, base.AVPDestinationHost, 0},
		{3, base.AVPOriginHost, 0},
		{4, AVPUserIdentifier, vendor3GPP},
		{5, base.AVPAuthSessionState, 0},
	} {
		b := dropAVP(t, encode(t, samples()[c.sample]), c.code, c.vendor)
		ae := wantResult(t, decode(t, b).Validate(), diam.ResultMissingAVP)
		if ae.AVP.Code != c.code || ae.AVP.Vendor != c.vendor {
			t.Fatalf("sample %d: AVP = %+v, want code %d vendor %d", c.sample, ae.AVP, c.code, c.vendor)
		}
	}
}

// Required members of the S6t grouped AVPs: 5005 with the path.
func TestMissingInNestedGroup(t *testing.T) {
	// { SCEF-ID } in Monitoring-Event-Configuration (§8.4.2), inside a CIR.
	mec := diam.AppendRaw(nil, avp3(AVPMonitoringType, true, []byte{0, 0, 0, 1}))
	cir := &ConfigurationInformationRequest{SessionId: "s", AuthSessionState: nsm, OriginHost: scef, OriginRealm: realm,
		DestinationRealm: realm, UserIdentifier: UserIdentifier{UserName: diam.Some(imsi)},
		Unknown: []diam.RawAVP{avp3(AVPMonitoringEventConfiguration, true, mec)}}
	ae := wantResult(t, decode(t, encode(t, cir)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPScefId || !slices.Equal(ae.Path, []uint32{AVPMonitoringEventConfiguration}) {
		t.Fatalf("AVPError = %+v", ae)
	}

	// { Granted-Validity-Time } in APN-Validity-Time (§8.4.65), inside
	// NIDD-Authorization-Update, inside an NIR.
	avt := diam.AppendRaw(nil, diam.RawAVP{Code: AVPServiceSelection, Flags: diam.AVPFlagMandatory, Data: []byte(apn)})
	upd := diam.AppendRaw(nil, avp3(AVPApnValidityTime, false, avt))
	nir := &NiddInformationRequest{SessionId: "s", AuthSessionState: nsm, OriginHost: scef, OriginRealm: realm,
		DestinationRealm: realm, UserIdentifier: UserIdentifier{UserName: diam.Some(imsi)},
		Unknown: []diam.RawAVP{avp3(AVPNiddAuthorizationUpdate, false, upd)}}
	ae = wantResult(t, decode(t, encode(t, nir)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPGrantedValidityTime || !slices.Equal(ae.Path, []uint32{AVPNiddAuthorizationUpdate, AVPApnValidityTime}) {
		t.Fatalf("AVPError = %+v", ae)
	}

	// { User-Identifier } in Group-Report-Item (§8.4.62), inside Group-Report, inside an RIR.
	item := diam.AppendRaw(nil, avp3(AVPMonitoringType, true, []byte{0, 0, 0, 1}))
	gr := diam.AppendRaw(nil, avp3(AVPScefReferenceId, true, []byte{0, 0, 0, 7}))
	gr = diam.AppendRaw(gr, avp3(AVPGroupReportItem, false, item))
	rir := &ReportingInformationRequest{SessionId: "s", AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
		DestinationHost: scef, DestinationRealm: realm, Unknown: []diam.RawAVP{avp3(AVPGroupReport, false, gr)}}
	ae = wantResult(t, decode(t, encode(t, rir)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPUserIdentifier || !slices.Equal(ae.Path, []uint32{AVPGroupReport, AVPGroupReportItem}) {
		t.Fatalf("AVPError = %+v", ae)
	}

	// { Enhanced-Coverage-Restriction } in Enhanced-Coverage-Restriction-Data (§8.4.52), inside a CIA.
	ecrd := diam.AppendRaw(nil, avp3(AVPVisitedPlmnId, true, plmn))
	cia := &ConfigurationInformationAnswer{SessionId: "s", AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
		Unknown: []diam.RawAVP{avp3(AVPEnhancedCoverageRestrictionData, false, ecrd)}}
	ae = wantResult(t, decode(t, encode(t, cia)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPEnhancedCoverageRestriction || !slices.Equal(ae.Path, []uint32{AVPEnhancedCoverageRestrictionData}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

func TestTooMany(t *testing.T) {
	// { User-Identifier } twice in a CIR.
	ae := wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[0]), AVPUserIdentifier)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPUserIdentifier {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// [ CIR-Flags ] at most once.
	ae = wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[0]), AVPCirFlags)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPCirFlags {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// [ Failed-AVP ] (current release) twice in an RIA.
	ae = wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[3]), base.AVPFailedAvp)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != base.AVPFailedAvp {
		t.Fatalf("AVP = %+v, want Failed-AVP", ae.AVP)
	}
	// *2 [ Served-Party-IP-Address ] in PDN-Connectivity-Status-Report (§8.4.77).
	cia := samples()[1].(*ConfigurationInformationAnswer)
	mer := &cia.MonitoringEventReport[0]
	mer.PdnConnectivityStatusReport = slices.Clone(mer.PdnConnectivityStatusReport) // shared with the other samples
	rep := &mer.PdnConnectivityStatusReport[0]
	rep.Unknown = []diam.RawAVP{{Code: AVPServedPartyIpAddress, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor,
		Data: []byte{0, 1, 192, 0, 2, 11}}}
	ae = wantResult(t, decode(t, encode(t, cia)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPServedPartyIpAddress ||
		!slices.Equal(ae.Path, []uint32{AVPMonitoringEventReport, AVPPdnConnectivityStatusReport}) {
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
	// CIR-Flags is Unsigned32: two octets is a bad length.
	ae := wantResult(t, decode(t, replace(encode(t, samples()[0]), AVPCirFlags, []byte{0, 1})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPCirFlags {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// Number-of-UEs is Unsigned32.
	ae = wantResult(t, decode(t, replace(encode(t, samples()[1]), AVPNumberOfUes, []byte{1, 2, 3})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPNumberOfUes {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// SCEF-Reference-ID-Ext is Unsigned64, inside Monitoring-Event-Report-Status.
	st := join([]diam.RawAVP{
		avp3(AVPScefReferenceId, true, []byte{0, 0, 0, 1}),
		avp3(AVPScefReferenceIdExt, false, []byte{0, 0, 0, 1}),
		avp3(AVPScefId, true, []byte(scef)),
	})
	ria := &ReportingInformationAnswer{SessionId: "s", AuthSessionState: nsm, OriginHost: scef, OriginRealm: realm,
		Unknown: []diam.RawAVP{avp3(AVPMonitoringEventReportStatus, false, st)}}
	ae = wantResult(t, decode(t, encode(t, ria)).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPScefReferenceIdExt || !slices.Equal(ae.Path, []uint32{AVPMonitoringEventReportStatus}) {
		t.Fatalf("AVPError = %+v", ae)
	}
	// Served-Party-IP-Address with an unknown address family (RFC 6733 §4.3.1).
	rep := join([]diam.RawAVP{
		{Code: AVPServiceSelection, Flags: diam.AVPFlagMandatory, Data: []byte(apn)},
		avp3(AVPPdnConnectivityStatusType, false, []byte{0, 0, 0, 0}),
		avp3(AVPServedPartyIpAddress, false, []byte{0, 99, 1, 2, 3, 4}),
	})
	mer := join([]diam.RawAVP{avp3(AVPScefReferenceId, true, []byte{0, 0, 0, 1}), avp3(AVPPdnConnectivityStatusReport, false, rep)})
	rir := &ReportingInformationRequest{SessionId: "s", AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
		DestinationHost: scef, DestinationRealm: realm, Unknown: []diam.RawAVP{avp3(AVPMonitoringEventReport, true, mer)}}
	ae = wantResult(t, decode(t, encode(t, rir)).Validate(), diam.ResultInvalidAVPValue)
	if ae.AVP.Code != AVPServedPartyIpAddress ||
		!slices.Equal(ae.Path, []uint32{AVPMonitoringEventReport, AVPPdnConnectivityStatusReport}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

func TestMisplacedFixed(t *testing.T) {
	// Session-Id is < > in every command.
	cir := encode(t, samples()[0])
	avps := avpsOf(t, cir)
	avps[0], avps[1] = avps[1], avps[0]
	ae := wantResult(t, decode(t, withAVPs(t, cir, avps)).Validate(), diam.ResultInvalidAVPValue)
	if ae.AVP.Code != base.AVPSessionId {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
}

func TestUnknownAVPs(t *testing.T) {
	cir := encode(t, samples()[0])
	quiet := diam.RawAVP{Code: 9999, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte("later release")}
	m := decode(t, withAVPs(t, cir, append(avpsOf(t, cir), quiet)))
	if err := m.Validate(); err != nil {
		t.Fatalf("unknown AVP without M-bit: %v", err)
	}
	if got := m.(*ConfigurationInformationRequest).Unknown; len(got) != 1 || got[0].Code != 9999 {
		t.Fatalf("Unknown = %+v", got)
	}

	loud := quiet
	loud.Flags |= diam.AVPFlagMandatory
	ae := wantResult(t, decode(t, withAVPs(t, cir, append(avpsOf(t, cir), loud))).Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != 9999 {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// The same inside Monitoring-Event-Report, a group with *[ AVP ].
	cia := samples()[1].(*ConfigurationInformationAnswer)
	cia.MonitoringEventReport[0].Unknown = []diam.RawAVP{loud}
	ae = wantResult(t, decode(t, encode(t, cia)).Validate(), diam.ResultAVPUnsupported)
	if !slices.Equal(ae.Path, []uint32{AVPMonitoringEventReport}) {
		t.Fatalf("Path = %v", ae.Path)
	}
}

// §8.2.2: Vendor-Specific-Application-Id is not sent by S6t nodes, and a
// receiver ignores it. It is decoded into its field, so a message from an
// agent that adds it still validates, and one without it is the usual case.
func TestVendorSpecificApplicationIdIgnored(t *testing.T) {
	for _, m := range samples() {
		b := dropAVP(t, encode(t, m), base.AVPVendorSpecificApplicationId, 0)
		got := decode(t, b)
		if err := got.Validate(); err != nil {
			t.Fatalf("%T without Vendor-Specific-Application-Id: %v", m, err)
		}
		if v := reflect.ValueOf(got).Elem().FieldByName("VendorSpecificApplicationId"); !v.IsNil() {
			t.Fatalf("%T: Vendor-Specific-Application-Id = %v", m, v.Interface())
		}
	}
}

// Release 13 peers send only what TS 29.336 V13.2.0 knew; their messages
// must validate, including the two M-bit AVPs the current release dropped.
func TestRelease13Messages(t *testing.T) {
	failed := &base.FailedAvp{Unknown: []diam.RawAVP{{Code: base.AVPOriginStateId, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 9}}}}
	for _, m := range []diam.Message{
		&ConfigurationInformationRequest{
			SessionId: "scef;1", AuthSessionState: nsm, OriginHost: scef, OriginRealm: realm, DestinationRealm: realm,
			UserIdentifier: UserIdentifier{Msisdn: diam.Some(msisdn)},
			MonitoringEventConfiguration: []MonitoringEventConfiguration{{
				ScefReferenceId: diam.Some(uint32(1)), ScefId: scef, MonitoringType: 0, MaximumNumberOfReports: diam.Some(uint32(1)),
				LocationInformationConfiguration: &LocationInformationConfiguration{MonteLocationType: diam.Some(uint32(1))},
			}},
			AeseCommunicationPattern: []AeseCommunicationPattern{{ScefId: scef, ScefReferenceId: diam.Some(uint32(2)),
				CommunicationPatternSet: []CommunicationPatternSet{{PeriodicTime: diam.Some(uint32(60)), ReferenceIdValidityTime: diam.Some(expiry)}}}},
		},
		// V13.2.0 §8.4.24: [ Service-Result ] { SCEF-Reference-ID } [ SCEF-ID ].
		&ConfigurationInformationAnswer{
			SessionId: "scef;1", ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			MonitoringEventConfigStatus: []MonitoringEventConfigStatus{{
				ServiceResult: &ServiceResult{ServiceResultCode: diam.Some(uint32(2001))}, ScefReferenceId: 1, ScefId: diam.Some(scef),
			}},
			SupportedServices: []SupportedServices{{SupportedMonitoringEvents: diam.Some(uint64(0x3f))}},
		},
		// V13.2.0 §8.2.5: RIR with *[ Failed-AVP ].
		&ReportingInformationRequest{
			SessionId: "hss;1", AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, DestinationHost: scef, DestinationRealm: realm,
			MonitoringEventReport: []MonitoringEventReport{{ScefReferenceId: 1, ReachabilityInformation: diam.Some(uint32(0))}},
			FailedAvp:             failed,
		},
		&ReportingInformationAnswer{
			SessionId: "hss;1", ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: scef, OriginRealm: realm,
		},
	} {
		got := decode(t, encode(t, m))
		if err := got.Validate(); err != nil {
			t.Fatalf("Rel-13 %T: %v", m, err)
		}
		if u := reflect.ValueOf(got).Elem().FieldByName("Unknown"); u.Len() != 0 {
			t.Fatalf("Rel-13 %T: Unknown = %v", m, u.Interface())
		}
	}
}

func TestNewAnswerFor(t *testing.T) {
	ss := samples()
	for i := 0; i < len(ss); i += 2 {
		req := ss[i]
		req.DiamHeader().Flags = diam.FlagRequest | diam.FlagProxiable | diam.FlagRetransmit
		var ans diam.Message
		switch r := req.(type) {
		case *ConfigurationInformationRequest:
			ans = NewConfigurationInformationAnswerFor(r)
		case *ReportingInformationRequest:
			ans = NewReportingInformationAnswerFor(r)
		case *NiddInformationRequest:
			ans = NewNiddInformationAnswerFor(r)
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
		"PLMN-ID-Requested TRUE (§8.4.68)":                 {int32(PlmnIdRequestedTrue), 0},
		"PLMN-ID-Requested FALSE":                          {int32(PlmnIdRequestedFalse), 1},
		"PDN-Type IPv4v6 (TS 29.272 §7.3.62)":              {int32(PdnTypeIpv4v6), 2},
		"Non-IP-PDN-Type-Indicator TRUE (§7.3.204)":        {int32(NonIpPdnTypeIndicatorTrue), 1},
		"Current-Location-Retrieved (§7.3.125)":            {int32(CurrentLocationRetrievedActiveLocationRetrieval), 0},
		"CSG-Access-Mode HYBRID_MODE (TS 32.299 §7.2.46A)": {int32(CsgAccessModeHybridMode), 1},
		"CSG-Membership-Indication CSG_MEMBER (§7.2.46B)":  {int32(CsgMembershipIndicationCsgMember), 1},
		"RAT-Type EUTRAN-NB-IoT (TS 29.212 §5.3.31)":       {int32(RatTypeEutranNbIot), 1005},
		"DRMP PRIORITY_15 (RFC 7944 §9.1)":                 {int32(DrmpPriority15), 15},
		"OC-Report-Type REALM_REPORT (RFC 7683 §7.6)":      {int32(OcReportTypeRealmReport), 1},
		"Load-Type PEER (RFC 8583 §7.2)":                   {int32(LoadTypePeer), 1},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}

// Codes the generated constants must carry, restated from TS 29.336 tables
// 6.4.1/1 and 8.4.1-1 and the specifications of the re-used AVPs. The
// dictionary cross-check in internal/interop covers the AVPs go-diameter
// knows; this covers the ones it does not.
func TestAVPCodes(t *testing.T) {
	for name, c := range map[string][2]uint32{
		"User-Identifier":                          {AVPUserIdentifier, 3102},
		"External-Identifier":                      {AVPExternalIdentifier, 3111},
		"AESE-Communication-Pattern":               {AVPAeseCommunicationPattern, 3113},
		"Communication-Pattern-Set":                {AVPCommunicationPatternSet, 3114},
		"Periodic-Communication-Indicator":         {AVPPeriodicCommunicationIndicator, 3115},
		"Communication-Duration-Time":              {AVPCommunicationDurationTime, 3116},
		"Periodic-Time":                            {AVPPeriodicTime, 3117},
		"Scheduled-Communication-Time":             {AVPScheduledCommunicationTime, 3118},
		"Stationary-Indication":                    {AVPStationaryIndication, 3119},
		"AESE-Communication-Pattern-Config-Status": {AVPAeseCommunicationPatternConfigStatus, 3120},
		"AESE-Error-Report":                        {AVPAeseErrorReport, 3121},
		"Monitoring-Event-Configuration":           {AVPMonitoringEventConfiguration, 3122},
		"Monitoring-Event-Report":                  {AVPMonitoringEventReport, 3123},
		"SCEF-Reference-ID":                        {AVPScefReferenceId, 3124},
		"SCEF-ID":                                  {AVPScefId, 3125},
		"SCEF-Reference-ID-for-Deletion":           {AVPScefReferenceIdForDeletion, 3126},
		"Monitoring-Type":                          {AVPMonitoringType, 3127},
		"Maximum-Number-of-Reports":                {AVPMaximumNumberOfReports, 3128},
		"UE-Reachability-Configuration":            {AVPUeReachabilityConfiguration, 3129},
		"Monitoring-Duration":                      {AVPMonitoringDuration, 3130},
		"Maximum-Detection-Time":                   {AVPMaximumDetectionTime, 3131},
		"Reachability-Type":                        {AVPReachabilityType, 3132},
		"Maximum-Latency":                          {AVPMaximumLatency, 3133},
		"Maximum-Response-Time":                    {AVPMaximumResponseTime, 3134},
		"Location-Information-Configuration":       {AVPLocationInformationConfiguration, 3135},
		"MONTE-Location-Type":                      {AVPMonteLocationType, 3136},
		"Accuracy":                                 {AVPAccuracy, 3137},
		"Association-Type":                         {AVPAssociationType, 3138},
		"Roaming-Information":                      {AVPRoamingInformation, 3139},
		"Reachability-Information":                 {AVPReachabilityInformation, 3140},
		"IMEI-Change":                              {AVPImeiChange, 3141},
		"Monitoring-Event-Config-Status":           {AVPMonitoringEventConfigStatus, 3142},
		"Supported-Services":                       {AVPSupportedServices, 3143},
		"Supported-Monitoring-Events":              {AVPSupportedMonitoringEvents, 3144},
		"CIR-Flags":                                {AVPCirFlags, 3145},
		"Service-Result":                           {AVPServiceResult, 3146},
		"Service-Result-Code":                      {AVPServiceResultCode, 3147},
		"Reference-ID-Validity-Time":               {AVPReferenceIdValidityTime, 3148},
		"Event-Handling":                           {AVPEventHandling, 3149},
		"NIDD-Authorization-Request":               {AVPNiddAuthorizationRequest, 3150},
		"NIDD-Authorization-Response":              {AVPNiddAuthorizationResponse, 3151},
		"Service-Report":                           {AVPServiceReport, 3152},
		"Node-Type":                                {AVPNodeType, 3153},
		"S6t-HSS-Cause":                            {AVPS6tHssCause, 3154},
		"Enhanced-Coverage-Restriction":            {AVPEnhancedCoverageRestriction, 3155},
		"Enhanced-Coverage-Restriction-Data":       {AVPEnhancedCoverageRestrictionData, 3156},
		"Restricted-PLMN-List":                     {AVPRestrictedPlmnList, 3157},
		"Allowed-PLMN-List":                        {AVPAllowedPlmnList, 3158},
		"Requested-Validity-Time":                  {AVPRequestedValidityTime, 3159},
		"Granted-Validity-Time":                    {AVPGrantedValidityTime, 3160},
		"NIDD-Authorization-Update":                {AVPNiddAuthorizationUpdate, 3161},
		"Loss-Of-Connectivity-Reason":              {AVPLossOfConnectivityReason, 3162},
		"Group-Reporting-Guard-Timer":              {AVPGroupReportingGuardTimer, 3163},
		"CIA-Flags":                                {AVPCiaFlags, 3164},
		"Group-Report":                             {AVPGroupReport, 3165},
		"Group-Report-Item":                        {AVPGroupReportItem, 3166},
		"RIR-Flags":                                {AVPRirFlags, 3167},
		"Type-Of-External-Identifier":              {AVPTypeOfExternalIdentifier, 3168},
		"APN-Validity-Time":                        {AVPApnValidityTime, 3169},
		"Suggested-Network-Configuration":          {AVPSuggestedNetworkConfiguration, 3170},
		"Monitoring-Event-Report-Status":           {AVPMonitoringEventReportStatus, 3171},
		"PLMN-ID-Requested":                        {AVPPlmnIdRequested, 3172},
		"AdditionalIdentifiers":                    {AVPAdditionalidentifiers, 3173},
		"NIR-Flags":                                {AVPNirFlags, 3174},
		"Reporting-Time-Stamp":                     {AVPReportingTimeStamp, 3175},
		"NIA-Flags":                                {AVPNiaFlags, 3176},
		"Group-User-Identifier":                    {AVPGroupUserIdentifier, 3177},
		"MTC-Provider-Info":                        {AVPMtcProviderInfo, 3178},
		"MTC-Provider-ID":                          {AVPMtcProviderId, 3179},
		"PDN-Connectivity-Status-Configuration":    {AVPPdnConnectivityStatusConfiguration, 3180},
		"PDN-Connectivity-Status-Report":           {AVPPdnConnectivityStatusReport, 3181},
		"PDN-Connectivity-Status-Type":             {AVPPdnConnectivityStatusType, 3182},
		"Traffic-Profile":                          {AVPTrafficProfile, 3183},
		"Updated-Network-Configuration":            {AVPUpdatedNetworkConfiguration, 3184},
		"Battery-Indicator":                        {AVPBatteryIndicator, 3185},
		"SCEF-Reference-ID-Ext":                    {AVPScefReferenceIdExt, 3186},
		"SCEF-Reference-ID-for-Deletion-Ext":       {AVPScefReferenceIdForDeletionExt, 3187},
		"Exclude-Identifiers":                      {AVPExcludeIdentifiers, 3188},
		"Include-Identifiers":                      {AVPIncludeIdentifiers, 3189},
		"Maximum-UE-Availability-Time":             {AVPMaximumUeAvailabilityTime, 3329},
		"eNodeB-ID":                                {AVPEnodebId, 4008},
		"Extended-eNodeB-ID":                       {AVPExtendedEnodebId, 4013},
		"Number-of-UEs":                            {AVPNumberOfUes, 4209},
		"Idle-Status-Indication":                   {AVPIdleStatusIndication, 4322},
		"Idle-Status-Timestamp":                    {AVPIdleStatusTimestamp, 4323},
		"Active-Time":                              {AVPActiveTime, 4324},
		"Reachability-Cause":                       {AVPReachabilityCause, 4325},
		"Service-Selection":                        {AVPServiceSelection, 493},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}
