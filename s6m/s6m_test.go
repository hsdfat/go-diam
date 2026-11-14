package s6m

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
		// Table 6.2.2/1 and §6.1.8: code 8388641, application 16777310.
		if h.CommandCode != 8388641 || h.AppID != 16777310 || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
			t.Fatalf("%s: header = %+v", name, h)
		}
		if isRequest := i == 0; (h.Flags&diam.FlagRequest != 0) != isRequest {
			t.Fatalf("%s: R-bit = %v", name, !isRequest)
		}
		// SIR and SIA carry PXY (§6.2.3-6.2.4).
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
	var sir SubscriberInformationRequest
	if err := sir.Unmarshal(encode(t, samples()[0])); err != nil {
		t.Fatal(err)
	}
	if sir.UserIdentifier.ExternalIdentifier.V != extID || sir.ServiceId.V != ServiceIdDeviceTrigger || sir.SirFlags != 1 ||
		!bytes.Equal(sir.ScsIdentity.V, scsNumber) || sir.ServiceParameters.ApplicationPortIdentifier.V != 9200 ||
		sir.ServiceParameters.T4Parameters.PriorityIndication.V != PriorityIndicationPriority ||
		sir.VendorSpecificApplicationId == nil {
		t.Fatalf("SIR = %+v", sir)
	}

	var sia SubscriberInformationAnswer
	if err := sia.Unmarshal(encode(t, samples()[1])); err != nil {
		t.Fatal(err)
	}
	sn := sia.ServiceData.T4Data.ServingNode
	if len(sia.UserIdentifier) != 2 || !bytes.Equal(sia.UserIdentifier[0].Lmsi.V, lmsi) || sn.IpSmGwRealm.V != realm ||
		!bytes.Equal(sn.MmeNumberForMtSms.V, mmeNumber) || sia.ServiceData.T4Data.AdditionalServingNode[0].MmeName.V != "mme2.example.org" ||
		sia.Load[0].LoadValue.V != 1000 || sia.OcOlr.OcSequenceNumber != 9 {
		t.Fatalf("SIA = %+v", sia)
	}
}

// mS6m is the M-bit each 3GPP AVP S6m carries gets from its defining
// specification (table 6.4.1/2 has no M-bit column): "M,V" for table
// 6.4.1/1 (3100-3112), TS 29.329 for MSISDN, TS 29.173 for LMSI,
// Serving-Node, MME-Name, MSC-Number, Additional-Serving-Node (M) and
// MME-Realm, SGSN-Name, SGSN-Realm (must not), TS 29.272 for SGSN-Number
// (M) and MME-Number-for-MT-SMS (must not), TS 29.338 for SM-RP-SMEA,
// TS 29.368 for Priority-Indication and Application-Port-Identifier.
// Supported-Features (628) is per message. Restated from the tables, not
// read from the schema.
var mS6m = map[uint32]bool{
	629: false, 630: false, 701: true, 1489: true, 1645: false,
	2400: true, 2401: true, 2402: true, 2403: true, 2406: true, 2408: false, 2409: false, 2410: false,
	3006: true, 3010: true,
	3100: true, 3101: true, 3102: true, 3103: true, 3104: true, 3105: true, 3106: true, 3107: true,
	3108: true, 3109: true, 3110: true, 3111: true, 3112: true,
	3309: true,
}

// mIETF is the M-bit of the IETF AVPs S6m re-uses: none of RFC 7683, 7944
// and 8583 requires it.
var mIETF = map[uint32]bool{
	301: false, 621: false, 622: false, 623: false, 624: false, 625: false, 626: false, 627: false,
	649: false, 650: false, 651: false, 652: false,
}

var groupCodes = map[[2]uint32]bool{
	{284, 0}: true, {297, 0}: true, {279, 0}: true, {260, 0}: true, {621, 0}: true, {623, 0}: true, {650, 0}: true,
	{628, vendor3GPP}: true, {2401, vendor3GPP}: true, {2406, vendor3GPP}: true, {3102, vendor3GPP}: true,
	{3105, vendor3GPP}: true, {3106, vendor3GPP}: true, {3107, vendor3GPP}: true, {3108, vendor3GPP}: true,
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
				// TS 29.229 §7.2.1 (applied by §6.1.9): set in a request, never in an answer.
				checked[k] = true
				if !top || a.Mandatory() != request {
					t.Errorf("%s: Supported-Features: M-bit = %v in a request=%v (top level %v)", name, a.Mandatory(), request, top)
				}
			case a.Vendor == vendor3GPP:
				if want, ok := mS6m[a.Code]; ok {
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
		walk(reflect.TypeOf(m).Elem().Name(), avpsOf(t, encode(t, m)), i == 0, true, false)
	}
	for c := range mIETF {
		if !checked[[2]uint32{c, 0}] {
			t.Errorf("IETF AVP %d is not in the samples", c)
		}
	}
	for c := range mS6m {
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
		{0, AVPSirFlags, vendor3GPP},
		{0, base.AVPDestinationRealm, 0},
		{0, base.AVPAuthSessionState, 0},
		{1, base.AVPOriginHost, 0},
		{1, base.AVPAuthSessionState, 0},
	} {
		b := dropAVP(t, encode(t, samples()[c.sample]), c.code, c.vendor)
		ae := wantResult(t, decode(t, b).Validate(), diam.ResultMissingAVP)
		if ae.AVP.Code != c.code || ae.AVP.Vendor != c.vendor {
			t.Fatalf("sample %d: AVP = %+v, want code %d vendor %d", c.sample, ae.AVP, c.code, c.vendor)
		}
	}
}

func TestTooMany(t *testing.T) {
	// { SIR-Flags } twice.
	ae := wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[0]), AVPSirFlags)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPSirFlags {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// [ Service-Data ] at most once.
	ae = wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[1]), AVPServiceData)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPServiceData {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// [ Failed-AVP ] (current release) twice in an SIA.
	ae = wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[1]), base.AVPFailedAvp)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != base.AVPFailedAvp {
		t.Fatalf("AVP = %+v, want Failed-AVP", ae.AVP)
	}
	// [ Serving-Node ] twice inside T4-Data, inside Service-Data.
	sia := samples()[1].(*SubscriberInformationAnswer)
	sn := sia.ServiceData.T4Data.ServingNode
	sia.ServiceData.T4Data.Unknown = []diam.RawAVP{avp3(AVPServingNode, true, sn.AppendAVPs(nil))}
	ae = wantResult(t, decode(t, encode(t, sia)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPServingNode || !slices.Equal(ae.Path, []uint32{AVPServiceData, AVPT4Data}) {
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
	// SIR-Flags is Unsigned32.
	ae := wantResult(t, decode(t, replace(encode(t, samples()[0]), AVPSirFlags, []byte{0, 1})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPSirFlags {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// Service-ID is Enumerated.
	ae = wantResult(t, decode(t, replace(encode(t, samples()[0]), AVPServiceId, []byte{0})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPServiceId {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// HSS-Cause is Unsigned32, two groups deep.
	t4 := diam.AppendRaw(nil, avp3(AVPHssCause, true, []byte{1}))
	sd := diam.AppendRaw(nil, avp3(AVPT4Data, true, t4))
	sia := &SubscriberInformationAnswer{SessionId: "s", AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
		Unknown: []diam.RawAVP{avp3(AVPServiceData, true, sd)}}
	ae = wantResult(t, decode(t, encode(t, sia)).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPHssCause || !slices.Equal(ae.Path, []uint32{AVPServiceData, AVPT4Data}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

func TestMisplacedFixed(t *testing.T) {
	sir := encode(t, samples()[0])
	avps := avpsOf(t, sir)
	avps[0], avps[1] = avps[1], avps[0]
	ae := wantResult(t, decode(t, withAVPs(t, sir, avps)).Validate(), diam.ResultInvalidAVPValue)
	if ae.AVP.Code != base.AVPSessionId {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
}

func TestUnknownAVPs(t *testing.T) {
	sir := encode(t, samples()[0])
	quiet := diam.RawAVP{Code: 9999, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte("later release")}
	m := decode(t, withAVPs(t, sir, append(avpsOf(t, sir), quiet)))
	if err := m.Validate(); err != nil {
		t.Fatalf("unknown AVP without M-bit: %v", err)
	}
	loud := quiet
	loud.Flags |= diam.AVPFlagMandatory
	ae := wantResult(t, decode(t, withAVPs(t, sir, append(avpsOf(t, sir), loud))).Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != 9999 {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// The same inside User-Identifier, a group with *[ AVP ].
	req := samples()[0].(*SubscriberInformationRequest)
	req.UserIdentifier.Unknown = []diam.RawAVP{loud}
	ae = wantResult(t, decode(t, encode(t, req)).Validate(), diam.ResultAVPUnsupported)
	if !slices.Equal(ae.Path, []uint32{AVPUserIdentifier}) {
		t.Fatalf("Path = %v", ae.Path)
	}
}

// §6.2.2: Vendor-Specific-Application-Id is not sent by S6m nodes and a
// receiver ignores it; a message with or without it validates.
func TestVendorSpecificApplicationIdIgnored(t *testing.T) {
	for _, m := range samples() {
		got := decode(t, dropAVP(t, encode(t, m), base.AVPVendorSpecificApplicationId, 0))
		if err := got.Validate(); err != nil {
			t.Fatalf("%T without Vendor-Specific-Application-Id: %v", m, err)
		}
	}
}

// Release 11 peers send only what TS 29.336 V11.1.0 knew.
func TestRelease11Messages(t *testing.T) {
	for _, m := range []diam.Message{
		&SubscriberInformationRequest{
			SessionId: "iwf;1", AuthSessionState: nsm, OriginHost: iwf, OriginRealm: realm, DestinationRealm: realm,
			UserIdentifier: UserIdentifier{Msisdn: diam.Some(msisdn)}, ServiceId: diam.Some(ServiceIdDeviceTrigger),
			ServiceParameters: &ServiceParameters{T4Parameters: &T4Parameters{PriorityIndication: diam.Some(PriorityIndicationNonPriority)}},
			SirFlags:          1,
		},
		&SubscriberInformationAnswer{
			SessionId: "iwf;1", ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			UserIdentifier: []UserIdentifier{{UserName: diam.Some(imsi)}},
			ServiceData: &ServiceData{T4Data: &T4Data{ServingNode: &ServingNode{
				SgsnNumber: diam.Some(sgsnNumber), IpSmGwNumber: diam.Some(ipsmNumber), IpSmGwName: diam.Some("ipsmgw.example.org"),
			}}},
		},
		&SubscriberInformationAnswer{
			SessionId: "iwf;2", ExperimentalResult: unauthorized, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
		},
	} {
		if err := decode(t, encode(t, m)).Validate(); err != nil {
			t.Fatalf("Rel-11 %T: %v", m, err)
		}
	}
}

func TestNewAnswerFor(t *testing.T) {
	req := samples()[0].(*SubscriberInformationRequest)
	req.Header.Flags = diam.FlagRequest | diam.FlagProxiable | diam.FlagRetransmit
	ans := NewSubscriberInformationAnswerFor(req)
	if ans.Header.Flags != diam.FlagProxiable || ans.Header.HopByHop != 0x11 || ans.Header.EndToEnd != 0x22 || ans.SessionId != req.SessionId {
		t.Fatalf("SIA = %+v", ans)
	}
}

func TestEnumValuesFollowSpec(t *testing.T) {
	for name, c := range map[string][2]int32{
		"Service-ID DEVICE_TRIGGER (§6.4.3)":          {int32(ServiceIdDeviceTrigger), 0},
		"Service-ID SMS_MO":                           {int32(ServiceIdSmsMo), 1},
		"Priority-Indication Priority (TS 29.368)":    {int32(PriorityIndicationPriority), 1},
		"Priority-Indication Non-Priority":            {int32(PriorityIndicationNonPriority), 0},
		"DRMP PRIORITY_15 (RFC 7944 §9.1)":            {int32(DrmpPriority15), 15},
		"OC-Report-Type REALM_REPORT (RFC 7683 §7.6)": {int32(OcReportTypeRealmReport), 1},
		"Load-Type PEER (RFC 8583 §7.2)":              {int32(LoadTypePeer), 1},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}

// Codes restated from TS 29.336 table 6.4.1/1 and the specifications of the
// re-used AVPs. The dictionary cross-check in internal/interop covers the
// AVPs go-diameter knows; this covers the ones it does not.
func TestAVPCodes(t *testing.T) {
	for name, c := range map[string][2]uint32{
		"IP-SM-GW-Number":             {AVPIpSmGwNumber, 3100},
		"IP-SM-GW-Name":               {AVPIpSmGwName, 3101},
		"User-Identifier":             {AVPUserIdentifier, 3102},
		"Service-ID":                  {AVPServiceId, 3103},
		"SCS-Identity":                {AVPScsIdentity, 3104},
		"Service-Parameters":          {AVPServiceParameters, 3105},
		"T4-Parameters":               {AVPT4Parameters, 3106},
		"Service-Data":                {AVPServiceData, 3107},
		"T4-Data":                     {AVPT4Data, 3108},
		"HSS-Cause":                   {AVPHssCause, 3109},
		"SIR-Flags":                   {AVPSirFlags, 3110},
		"External-Identifier":         {AVPExternalIdentifier, 3111},
		"IP-SM-GW-Realm":              {AVPIpSmGwRealm, 3112},
		"LMSI":                        {AVPLmsi, 2400},
		"Serving-Node":                {AVPServingNode, 2401},
		"MME-Name":                    {AVPMmeName, 2402},
		"MSC-Number":                  {AVPMscNumber, 2403},
		"Additional-Serving-Node":     {AVPAdditionalServingNode, 2406},
		"MME-Realm":                   {AVPMmeRealm, 2408},
		"SGSN-Name":                   {AVPSgsnName, 2409},
		"SGSN-Realm":                  {AVPSgsnRealm, 2410},
		"SGSN-Number":                 {AVPSgsnNumber, 1489},
		"MME-Number-for-MT-SMS":       {AVPMmeNumberForMtSms, 1645},
		"Priority-Indication":         {AVPPriorityIndication, 3006},
		"Application-Port-Identifier": {AVPApplicationPortIdentifier, 3010},
		"SM-RP-SMEA":                  {AVPSmRpSmea, 3309},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}
