package s6a

import (
	"bytes"
	"errors"
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

// without returns the AVP sequence data with every AVP of code removed.
func without(t *testing.T, data []byte, code uint32) []byte {
	t.Helper()
	avps := split(t, data)
	return join(slices.DeleteFunc(avps, func(a diam.RawAVP) bool { return a.Code == code }))
}

// grp is a 3GPP grouped AVP with the M-bit, as most S6a groups are.
func grp(code uint32, data []byte) diam.RawAVP {
	return diam.RawAVP{Code: code, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: data}
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
	// TS 29.272 table 7.2.2/1.
	wantCodes := []uint32{316, 316, 317, 317, 318, 318, 319, 319, 320, 320, 321, 321, 322, 322, 323, 323}
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
		if h.CommandCode != wantCodes[i] || h.AppID != 16777251 || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
			t.Fatalf("%s: header = %+v", name, h)
		}
		if isRequest := i%2 == 0; (h.Flags&diam.FlagRequest != 0) != isRequest {
			t.Fatalf("%s: R-bit = %v", name, !isRequest)
		}
		// Every S6a/S6d command header carries PXY (§7.2.3-7.2.18).
		if h.Flags&diam.FlagProxiable == 0 {
			t.Fatalf("%s: P-bit clear", name)
		}
		if b2 := encode(t, m); !bytes.Equal(b1, b2) {
			t.Fatalf("%s: re-encode differs\n%x\n%x", name, b1, b2)
		}
		if first := avpsOf(t, b1)[0]; first.Code != base.AVPSessionId {
			t.Fatalf("%s: first AVP is %d, want Session-Id", name, first.Code)
		}
	}
}

func TestDecodedValues(t *testing.T) {
	var ula UpdateLocationAnswer
	if err := ula.Unmarshal(encode(t, samples()[1])); err != nil {
		t.Fatal(err)
	}
	sd := ula.SubscriptionData
	if sd == nil || sd.ApnConfigurationProfile == nil || len(sd.ApnConfigurationProfile.ApnConfiguration) != 2 {
		t.Fatalf("Subscription-Data = %+v", sd)
	}
	ac := sd.ApnConfigurationProfile.ApnConfiguration[0]
	if ac.ServiceSelection != "internet" || ac.PdnType != PdnTypeIpv4v6 ||
		ac.EpsSubscribedQosProfile.QosClassIdentifier != QosClassIdentifierQci9 ||
		ac.EpsSubscribedQosProfile.AllocationRetentionPriority.PriorityLevel != 8 ||
		ac.Mip6AgentInfo.MipHomeAgentHost.DestinationHost != "pgw.example.org" ||
		ac.Mip6AgentInfo.MipHomeAgentAddress[1].String() != "2001:db8::10" ||
		ac.SpecificApnInfo[0].Mip6AgentInfo.MipHomeAgentAddress[0].String() != "192.0.2.10" {
		t.Fatalf("APN-Configuration = %+v", ac)
	}
	if got := sd.LcsInfo.LcsPrivacyexception[0].ServiceType[0].Servicetypeidentity; got != 3 {
		t.Fatalf("ServiceTypeIdentity = %d", got)
	}
	if !sd.CsgSubscriptionData[0].ExpirationDate.V.Equal(when) || sd.DlBufferingSuggestedPacketCount.V != -1 {
		t.Fatalf("CSG-Subscription-Data / DL-Buffering = %+v / %+v", sd.CsgSubscriptionData, sd.DlBufferingSuggestedPacketCount)
	}
	if ula.OcOlr == nil || ula.OcOlr.OcSequenceNumber != 9 || ula.OcOlr.OcReportType != OcReportTypeRealmReport {
		t.Fatalf("OC-OLR = %+v", ula.OcOlr)
	}

	var ida InsertSubscriberDataAnswer
	if err := ida.Unmarshal(encode(t, samples()[7])); err != nil {
		t.Fatal(err)
	}
	mer := ida.MonitoringEventReport[0]
	if mer.ScefReferenceId != 11 || mer.IdleStatusIndication.EdrxCycleLength.RatType != RatTypeEutran ||
		mer.EpsLocationInformation.MmeLocationInformation.UserCsgInformation.CsgAccessMode != CsgAccessModeHybridMode {
		t.Fatalf("Monitoring-Event-Report = %+v", mer)
	}
}

// m3GPP is the M-bit TS 29.272 table 7.3.1/1 gives each S6a AVP: set for
// 1400-1490 except User-Id (1444) and for the S7a AVPs 1639-1641, clear for
// the AVPs added from Release 9 on. It is restated here from the table, not
// read from the schema.
func m3GPP(code uint32) bool {
	switch {
	case code == 1444:
		return false
	case code >= 1400 && code <= 1490, code >= 1639 && code <= 1641:
		return true
	}
	return false
}

// mReused is the S6a M-bit of the re-used AVPs in the samples: table 7.3.1/2
// where it gives one, otherwise the defining specification.
var mReused = map[[2]uint32]bool{
	{301, 0}: false, {334, 0}: true, {348, 0}: true, {486, 0}: true, {493, 0}: true,
	{561, 0}: false, {562, 0}: false, {563, 0}: false,
	{621, 0}: false, {622, 0}: false, {623, 0}: false, {624, 0}: false, {625, 0}: false, {626, 0}: false, {627, 0}: false,
	{649, 0}: false, {650, 0}: false, {651, 0}: false, {652, 0}: false,
	{13, 10415}: false, {515, 10415}: true, {516, 10415}: true, {554, 10415}: false, {555, 10415}: false,
	{600, 10415}: false, {625, 10415}: true, {626, 10415}: true, {628, 10415}: false, {629, 10415}: false, {630, 10415}: false,
	{701, 10415}: true, {848, 10415}: true, {1028, 10415}: true, {1032, 10415}: true, {1034, 10415}: false, {1046, 10415}: false,
	{1047, 10415}: false, {1048, 10415}: false, {1227, 10415}: true, {1538, 10415}: false, {2317, 10415}: true, {2318, 10415}: true,
	{2319, 10415}: false, {2405, 10415}: false, {3111, 10415}: false, {3113, 10415}: false, {3114, 10415}: false,
	{3115, 10415}: true, {3117, 10415}: true, {3118, 10415}: true, {3122, 10415}: false, {3123, 10415}: false,
	{3124, 10415}: false, {3125, 10415}: false, {3127, 10415}: false, {3128, 10415}: false, {3129, 10415}: false,
	{3130, 10415}: false, {3132, 10415}: false, {3134, 10415}: false, {3135, 10415}: false, {3136, 10415}: true,
	{3137, 10415}: true, {3140, 10415}: false, {3142, 10415}: false, {3143, 10415}: false, {3144, 10415}: false,
	{3146, 10415}: true, {3147, 10415}: true, {3148, 10415}: false, {3152, 10415}: true, {3153, 10415}: true,
	{3178, 10415}: false, {3179, 10415}: false, {3180, 10415}: false, {3181, 10415}: false, {3182, 10415}: false,
	{3329, 10415}: false, {3701, 10415}: false, {3702, 10415}: true, {4008, 10415}: false, {4322, 10415}: false,
	{4323, 10415}: false, {4324, 10415}: false,
}

// groupCodes are the grouped AVPs in the samples, so the flag check can walk
// into them without consulting the schema.
var groupCodes = map[[2]uint32]bool{
	{284, 0}: true, {297, 0}: true, {279, 0}: true, {260, 0}: true, {348, 0}: true, {486, 0}: true, {621, 0}: true,
	{623, 0}: true, {650, 0}: true,
}

func init() {
	for _, c := range []uint32{628, 1034, 2319, 3113, 3114, 3118, 3122, 3123, 3129, 3135, 3142, 3143, 3146, 3152, 3178, 3180, 3181, 3701, 4322,
		1400, 1401, 1408, 1409, 1413, 1414, 1415, 1416, 1429, 1430, 1431, 1435, 1436, 1458, 1467, 1469, 1472, 1473, 1475, 1479, 1483, 1485,
		1486, 1488, 1495, 1496, 1497, 1498, 1600, 1601, 1612, 1622, 1624, 1637, 1649, 1667, 1672, 1673, 1675, 1685, 1687, 1688, 1691, 1694,
		1701, 1705, 1710, 1711, 1712, 1714} {
		groupCodes[[2]uint32{c, vendor3GPP}] = true
	}
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
			if !inFailed && a.Vendor == vendor3GPP && a.Code >= 1400 && a.Code <= 1727 {
				checked[k] = true
				if a.Mandatory() != m3GPP(a.Code) {
					t.Errorf("%s: AVP %d: M-bit = %v, table 7.3.1/1 says %v", name, a.Code, a.Mandatory(), m3GPP(a.Code))
				}
			} else if want, ok := mReused[k]; ok && !inFailed {
				checked[k] = true
				if a.Mandatory() != want {
					t.Errorf("%s: AVP %v: M-bit = %v, want %v", name, k, a.Mandatory(), want)
				}
			}
			if groupCodes[k] {
				walk(name, split(t, a.Data), inFailed || a.Code == base.AVPFailedAvp && a.Vendor == 0)
			}
		}
	}
	for _, m := range samples() {
		walk(reflect.TypeOf(m).Elem().Name(), avpsOf(t, encode(t, m)), false)
	}
	for k := range mReused {
		if !checked[k] {
			t.Errorf("re-used AVP %v is not in the samples", k)
		}
	}
	if len(checked) < 150 {
		t.Errorf("checked %d distinct AVPs, want at least 150", len(checked))
	}
}

func TestMissingRequired(t *testing.T) {
	ulr := encode(t, samples()[0])
	avps := slices.DeleteFunc(avpsOf(t, ulr), func(a diam.RawAVP) bool { return a.Code == base.AVPUserName })
	ae := wantResult(t, decode(t, withAVPs(t, ulr, avps)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != base.AVPUserName || ae.AVP.Vendor != 0 {
		t.Fatalf("AVP = %+v, want User-Name", ae.AVP)
	}

	avps = slices.DeleteFunc(avpsOf(t, ulr), func(a diam.RawAVP) bool { return a.Code == AVPRatType })
	ae = wantResult(t, decode(t, withAVPs(t, ulr, avps)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPRatType || ae.AVP.Vendor != vendor3GPP {
		t.Fatalf("AVP = %+v, want RAT-Type", ae.AVP)
	}
}

// An APN-Configuration without its Service-Selection, three groups deep in
// a ULA: 5005 with the path of enclosing groups.
func TestMissingInNestedGroup(t *testing.T) {
	ac := subscriptionData().ApnConfigurationProfile.ApnConfiguration[1]
	acData := without(t, ac.AppendAVPs(nil), AVPServiceSelection)
	prof := &ApnConfigurationProfile{ContextIdentifier: 2, AllApnConfigurationsIncludedIndicator: AllApnConfigurationsIncludedIndicatorAllApnConfigurationsIncluded}
	profData := append(prof.AppendAVPs(nil), diam.AppendRaw(nil, grp(AVPApnConfiguration, acData))...)
	sdData := diam.AppendRaw(nil, grp(AVPApnConfigurationProfile, profData))

	ula := &UpdateLocationAnswer{SessionId: "s", AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: hss, OriginRealm: realm,
		Unknown: []diam.RawAVP{grp(AVPSubscriptionData, sdData)}}
	ae := wantResult(t, decode(t, encode(t, ula)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPServiceSelection || ae.AVP.Vendor != 0 {
		t.Fatalf("AVP = %+v, want Service-Selection", ae.AVP)
	}
	if want := []uint32{AVPSubscriptionData, AVPApnConfigurationProfile, AVPApnConfiguration}; !slices.Equal(ae.Path, want) {
		t.Fatalf("Path = %v, want %v", ae.Path, want)
	}
}

// 1*{ APN-Configuration } (§7.3.34): a profile without one is 5005.
func TestApnConfigurationProfileNeedsOne(t *testing.T) {
	idr := samples()[6].(*InsertSubscriberDataRequest)
	idr.SubscriptionData = SubscriptionData{ApnConfigurationProfile: &ApnConfigurationProfile{ContextIdentifier: 1}}
	ae := wantResult(t, decode(t, encode(t, idr)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPApnConfiguration || !slices.Equal(ae.Path, []uint32{AVPSubscriptionData, AVPApnConfigurationProfile}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

func TestTooMany(t *testing.T) {
	// *10[ Regional-Subscription-Zone-Code ] (§7.3.2).
	idr := samples()[6].(*InsertSubscriberDataRequest)
	idr.SubscriptionData.RegionalSubscriptionZoneCode = make([][]byte, 11)
	for i := range idr.SubscriptionData.RegionalSubscriptionZoneCode {
		idr.SubscriptionData.RegionalSubscriptionZoneCode[i] = []byte{0, byte(i)}
	}
	ae := wantResult(t, decode(t, encode(t, idr)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPRegionalSubscriptionZoneCode || !bytes.Equal(ae.AVP.Data, []byte{0, 10}) {
		t.Fatalf("AVP = %+v, want the 11th Regional-Subscription-Zone-Code", ae.AVP)
	}

	// *2[ MIP-Home-Agent-Address ] (RFC 5447 §4.2.1).
	nor := samples()[14].(*NotifyRequest)
	nor.Mip6AgentInfo.MipHomeAgentAddress = append(nor.Mip6AgentInfo.MipHomeAgentAddress, nor.Mip6AgentInfo.MipHomeAgentAddress[0])
	ae = wantResult(t, decode(t, encode(t, nor)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPMipHomeAgentAddress || !slices.Equal(ae.Path, []uint32{AVPMip6AgentInfo}) {
		t.Fatalf("AVPError = %+v", ae)
	}

	// A second ULR-Flags.
	ulr := encode(t, samples()[0])
	avps := avpsOf(t, ulr)
	for _, a := range avps {
		if a.Code == AVPUlrFlags {
			avps = append(avps, a)
			break
		}
	}
	wantResult(t, decode(t, withAVPs(t, ulr, avps)).Validate(), diam.ResultAVPOccursTooMany)
}

func TestBadValues(t *testing.T) {
	ulr := encode(t, samples()[0])
	replace := func(code uint32, data []byte) []byte {
		avps := avpsOf(t, ulr)
		for i := range avps {
			if avps[i].Code == code {
				avps[i].Data = data
			}
		}
		return withAVPs(t, ulr, avps)
	}
	// ULR-Flags is Unsigned32: two octets is a bad length.
	ae := wantResult(t, decode(t, replace(AVPUlrFlags, []byte{0, 1})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPUlrFlags {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// GMLC-Address with address family 8 (E.164) is not an IP address.
	ae = wantResult(t, decode(t, replace(AVPGmlcAddress, []byte{0, 8, '1', '2'})).Validate(), diam.ResultInvalidAVPValue)
	if ae.AVP.Code != AVPGmlcAddress {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
}

func TestMisplacedFixed(t *testing.T) {
	// Session-Id is < > in every command.
	ulr := encode(t, samples()[0])
	avps := avpsOf(t, ulr)
	avps[0], avps[1] = avps[1], avps[0]
	ae := wantResult(t, decode(t, withAVPs(t, ulr, avps)).Validate(), diam.ResultInvalidAVPValue)
	if ae.AVP.Code != base.AVPSessionId {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// OC-OLR ::= < OC-Sequence-Number > < OC-Report-Type > ... (RFC 7683 §7.3).
	olr := split(t, ocOlr.AppendAVPs(nil))
	olr[0], olr[1] = olr[1], olr[0]
	noa := &NotifyAnswer{SessionId: "s", AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: hss, OriginRealm: realm,
		Unknown: []diam.RawAVP{{Code: AVPOcOlr, Data: join(olr)}}}
	ae = wantResult(t, decode(t, encode(t, noa)).Validate(), diam.ResultInvalidAVPValue)
	if !slices.Equal(ae.Path, []uint32{AVPOcOlr}) {
		t.Fatalf("Path = %v", ae.Path)
	}
}

func TestUnknownAVPs(t *testing.T) {
	ulr := encode(t, samples()[0])
	quiet := diam.RawAVP{Code: 9999, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte("later release")}
	m := decode(t, withAVPs(t, ulr, append(avpsOf(t, ulr), quiet)))
	if err := m.Validate(); err != nil {
		t.Fatalf("unknown AVP without M-bit: %v", err)
	}
	if got := m.(*UpdateLocationRequest).Unknown; len(got) != 1 || got[0].Code != 9999 {
		t.Fatalf("Unknown = %+v", got)
	}

	loud := quiet
	loud.Flags |= diam.AVPFlagMandatory
	ae := wantResult(t, decode(t, withAVPs(t, ulr, append(avpsOf(t, ulr), loud))).Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != 9999 {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// The same inside Subscription-Data, a group with *[ AVP ].
	idr := samples()[6].(*InsertSubscriberDataRequest)
	idr.SubscriptionData.Unknown = []diam.RawAVP{loud}
	ae = wantResult(t, decode(t, encode(t, idr)).Validate(), diam.ResultAVPUnsupported)
	if !slices.Equal(ae.Path, []uint32{AVPSubscriptionData}) {
		t.Fatalf("Path = %v", ae.Path)
	}
}

// A Release 8 MME sends only what TS 29.272 V8 knew; it must validate.
func TestRelease8Messages(t *testing.T) {
	ulr := &UpdateLocationRequest{
		SessionId: "mme;1", AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: mme, OriginRealm: realm,
		DestinationRealm: realm, UserName: imsi, RatType: RatTypeEutran, UlrFlags: 0x02, VisitedPlmnId: plmn,
	}
	if err := decode(t, encode(t, ulr)).Validate(); err != nil {
		t.Fatalf("Rel-8 ULR: %v", err)
	}
	ula := &UpdateLocationAnswer{
		SessionId: "mme;1", ResultCode: diam.Some(uint32(2001)), AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost: hss, OriginRealm: realm, SubscriptionData: &SubscriptionData{
			Msisdn: diam.Some([]byte{0x21}), ApnConfigurationProfile: &ApnConfigurationProfile{
				ContextIdentifier: 1,
				ApnConfiguration:  []ApnConfiguration{{ContextIdentifier: 1, PdnType: PdnTypeIpv4, ServiceSelection: "internet"}},
			},
		},
	}
	if err := decode(t, encode(t, ula)).Validate(); err != nil {
		t.Fatalf("Rel-8 ULA: %v", err)
	}
}

func TestNewAnswerFor(t *testing.T) {
	ss := samples()
	for i := 0; i < len(ss); i += 2 {
		req := ss[i]
		req.DiamHeader().Flags = diam.FlagRequest | diam.FlagProxiable | diam.FlagRetransmit
		var ans diam.Message
		switch r := req.(type) {
		case *UpdateLocationRequest:
			ans = NewUpdateLocationAnswerFor(r)
		case *CancelLocationRequest:
			ans = NewCancelLocationAnswerFor(r)
		case *AuthenticationInformationRequest:
			ans = NewAuthenticationInformationAnswerFor(r)
		case *InsertSubscriberDataRequest:
			ans = NewInsertSubscriberDataAnswerFor(r)
		case *DeleteSubscriberDataRequest:
			ans = NewDeleteSubscriberDataAnswerFor(r)
		case *PurgeUeRequest:
			ans = NewPurgeUeAnswerFor(r)
		case *ResetRequest:
			ans = NewResetAnswerFor(r)
		case *NotifyRequest:
			ans = NewNotifyAnswerFor(r)
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
		"RAT-Type EUTRAN (TS 29.212 §5.3.31)": {int32(RatTypeEutran), 1004},
		"RAT-Type NR":                         {int32(RatTypeNr), 1006},
		"RAT-Type WLAN":                       {int32(RatTypeWlan), 0},
		"Cancellation-Type SUBSCRIPTION_WITHDRAWAL (§7.3.24)":      {int32(CancellationTypeSubscriptionWithdrawal), 2},
		"Cancellation-Type INITIAL_ATTACH_PROCEDURE":               {int32(CancellationTypeInitialAttachProcedure), 4},
		"Network-Access-Mode ONLY_PACKET (§7.3.21)":                {int32(NetworkAccessModeOnlyPacket), 2},
		"PDN-Type IPv4_OR_IPv6 (§7.3.62)":                          {int32(PdnTypeIpv4OrIpv6), 3},
		"User-State CONNECTED_REACHABLE_FOR_PAGING (§7.3.114)":     {int32(UserStateConnectedReachableForPaging), 4},
		"Error-Diagnostic ODB_VPLMN_APN (§7.3.128)":                {int32(ErrorDiagnosticOdbVplmnApn), 4},
		"UE-SRVCC-Capability UE-SRVCC-SUPPORTED (§7.3.130)":        {int32(UeSrvccCapabilityUeSrvccSupported), 1},
		"SMS-Register-Request NO_PREFERENCE (§7.3.162)":            {int32(SmsRegisterRequestNoPreference), 2},
		"Daylight-Saving-Time PLUS_TWO_HOURS_ADJUSTMENT":           {int32(DaylightSavingTimePlusTwoHoursAdjustment), 2},
		"PLMN-Client TARGET_UE_SUBSCRIBED_SERVICE (§7.3.93)":       {int32(PlmnClientTargetUeSubscribedService), 4},
		"QoS-Class-Identifier QCI_9 (TS 29.212 §5.3.17)":           {int32(QosClassIdentifierQci9), 9},
		"QoS-Class-Identifier QCI_85":                              {int32(QosClassIdentifierQci85), 85},
		"Pre-emption-Capability DISABLED (TS 29.212 §5.3.46)":      {int32(PreEmptionCapabilityPreEmptionCapabilityDisabled), 1},
		"DRMP PRIORITY_15 (RFC 7944 §9.1)":                         {int32(DrmpPriority15), 15},
		"OC-Report-Type REALM_REPORT (RFC 7683 §7.6)":              {int32(OcReportTypeRealmReport), 1},
		"Load-Type PEER (RFC 8583 §7.2)":                           {int32(LoadTypePeer), 1},
		"CSG-Access-Mode Hybrid (TS 32.299 §7.2.46A)":              {int32(CsgAccessModeHybridMode), 1},
		"IAB-Operation-Permission IAB_OPERATION_NOTALLOWED":        {int32(IabOperationPermissionIabOperationNotallowed), 1},
		"All-APN-Configurations-Included-Indicator MODIFIED_ADDED": {int32(AllApnConfigurationsIncludedIndicatorModifiedAddedApnConfigurationsIncluded), 1},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
	if got := RatTypeEutran.String(); got != "EUTRAN" {
		t.Errorf("RatTypeEutran.String() = %q", got)
	}
	if got := RatType(1099).String(); got != "1099" {
		t.Errorf("unknown RAT-Type String() = %q", got)
	}
}

// Codes the generated constants must carry, restated from TS 29.272 table
// 7.3.1/1 and the specifications of the re-used AVPs. The dictionary
// cross-check in internal/interop covers the AVPs go-diameter knows; this
// covers some it does not.
func TestAVPCodes(t *testing.T) {
	for name, c := range map[string][2]uint32{
		"Subscription-Data":             {AVPSubscriptionData, 1400},
		"ULR-Flags":                     {AVPUlrFlags, 1405},
		"Area-Scope":                    {AVPAreaScope, 1624},
		"Job-Type":                      {AVPJobType, 1623},
		"Ext-PDP-Address":               {AVPExtPdpAddress, 1621},
		"SIPTO-Permission":              {AVPSiptoPermission, 1613},
		"Additional-Context-ID":         {AVPAdditionalContextId, 1683},
		"Trace-Reporting-Consumer-Uri":  {AVPTraceReportingConsumerUri, 1727},
		"Monitoring-Event-Report":       {AVPMonitoringEventReport, 3123},
		"Idle-Status-Indication":        {AVPIdleStatusIndication, 4322},
		"Maximum-UE-Availability-Time":  {AVPMaximumUeAvailabilityTime, 3329},
		"Active-Time":                   {AVPActiveTime, 4324},
		"Reachability-Cause":            {AVPReachabilityCause, 4325},
		"OC-Validity-Duration":          {AVPOcValidityDuration, 625},
		"Confidentiality-Key":           {AVPConfidentialityKey, 625},
		"3GPP-Charging-Characteristics": {AVPTgppChargingCharacteristics, 13},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}
