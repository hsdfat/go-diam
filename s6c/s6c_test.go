package s6c

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
	// TS 29.338 table 5.3.2.2/1.
	wantCodes := []uint32{8388647, 8388647, 8388648, 8388648, 8388649, 8388649}
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
		// §4.8: application id 16777312.
		if h.CommandCode != wantCodes[i] || h.AppID != 16777312 || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
			t.Fatalf("%s: header = %+v", name, h)
		}
		if isRequest := i%2 == 0; (h.Flags&diam.FlagRequest != 0) != isRequest {
			t.Fatalf("%s: R-bit = %v", name, !isRequest)
		}
		// Every S6c command header carries PXY (§5.3.2.3-5.3.2.8).
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
	var srr SendRoutingInfoForSmRequest
	if err := srr.Unmarshal(encode(t, samples()[0])); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(srr.Msisdn.V, msisdn) || srr.SmsmiCorrelationId.DestinationSipUri.V != "sip:bob@ims.example.org" ||
		srr.SmRpMti.V != SmRpMtiSmDeliver || srr.SrrFlags.V != 7 || srr.SmDeliveryNotIntended.V != SmDeliveryNotIntendedOnlyImsiRequested {
		t.Fatalf("SRR = %+v", srr)
	}

	var sra SendRoutingInfoForSmAnswer
	if err := sra.Unmarshal(encode(t, samples()[1])); err != nil {
		t.Fatal(err)
	}
	if sra.ServingNode.IpSmGwSbiSupportIndicator.V != IpSmGwSbiSupportIndicatorSupportSbi || sra.Smsf3GppAddress.Smsf3GppName.V != "smsf.example.org" ||
		sra.SmsfNon3GppAddress.SmsfNon3GppSbiSupportIndicator.V != SmsfNon3GppSbiSupportIndicatorNotSupportSbi ||
		!bytes.Equal(sra.UserIdentifier.Lmsi.V, lmsi) || sra.MwdStatus.V != 6 || sra.SmsfNon3GppAbsentUserDiagnosticSm.V != 5 {
		t.Fatalf("SRA = %+v", sra)
	}

	var rdr ReportSmDeliveryStatusRequest
	if err := rdr.Unmarshal(encode(t, samples()[4])); err != nil {
		t.Fatal(err)
	}
	o := rdr.SmDeliveryOutcome
	if o.MmeSmDeliveryOutcome.SmDeliveryCause.V != SmDeliveryCauseAbsentUser || o.IpSmGwSmDeliveryOutcome.SmDeliveryCause.Set ||
		o.SmsfNon3GppSmDeliveryOutcome.AbsentUserDiagnosticSm.V != 2 || rdr.RdrFlags.V != 1 {
		t.Fatalf("RDR = %+v", rdr)
	}
}

// mS6c is the M-bit TS 29.338 gives each 3GPP AVP S6c carries: "M, V" for
// 3308-3322 and "V" with M "must not" for 3323-3348 (table 5.3.3.1/1);
// table 5.3.3.1/2 sets it for MME-Realm and MME-Number-for-MT-SMS; table
// 6.3.3.1/1 for SC-Address, SM-Delivery-Failure-Cause and its members (M)
// and SMSMI-Correlation-ID and its members (V); the defining specification
// for the rest (TS 29.329 MSISDN, TS 29.272 SGSN-Number, TS 29.173 LMSI,
// Serving-Node, MME-Name, MSC-Number, Additional-Serving-Node (M) and
// SGSN-Name, SGSN-Realm (must not), TS 29.336 table 6.4.1/1). Supported-
// Features (628) is per message. Restated from the tables, not read from
// the schema.
var mS6c = map[uint32]bool{
	629: false, 630: false, 701: true, 1489: true, 1645: true,
	2400: true, 2401: true, 2402: true, 2403: true, 2406: true, 2408: true, 2409: false, 2410: false,
	3100: true, 3101: true, 3102: true, 3111: true, 3112: true,
	3300: true, 3303: true, 3304: true, 3305: true,
	3324: false, 3325: false, 3326: false, 3327: false,
}

func init() {
	for c := uint32(3308); c <= 3322; c++ {
		mS6c[c] = true
	}
	for _, c := range []uint32{3323, 3329, 3333, 3334, 3335, 3336, 3337, 3338, 3339, 3340, 3341, 3342, 3343, 3344, 3345, 3346, 3347, 3348} {
		mS6c[c] = false
	}
}

var groupCodes = map[[2]uint32]bool{
	{284, 0}: true, {297, 0}: true, {279, 0}: true, {260, 0}: true,
	{628, vendor3GPP}: true, {2401, vendor3GPP}: true, {2406, vendor3GPP}: true, {3102, vendor3GPP}: true,
	{3303, vendor3GPP}: true, {3316, vendor3GPP}: true, {3317, vendor3GPP}: true, {3318, vendor3GPP}: true,
	{3319, vendor3GPP}: true, {3320, vendor3GPP}: true, {3324, vendor3GPP}: true, {3336, vendor3GPP}: true,
	{3337, vendor3GPP}: true, {3344, vendor3GPP}: true, {3345, vendor3GPP}: true,
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
				// TS 29.229 §7.2.1 (applied by §4.9): set in a request, never in an answer.
				checked[k] = true
				if !top || a.Mandatory() != request {
					t.Errorf("%s: Supported-Features: M-bit = %v in a request=%v (top level %v)", name, a.Mandatory(), request, top)
				}
			case a.Vendor == vendor3GPP:
				if want, ok := mS6c[a.Code]; ok {
					checked[k] = true
					if a.Mandatory() != want {
						t.Errorf("%s: AVP %d: M-bit = %v, want %v", name, a.Code, a.Mandatory(), want)
					}
				}
			case k == [2]uint32{301, 0}:
				// DRMP: table 5.3.3.1/2 "Must not set".
				checked[k] = true
				if a.Mandatory() {
					t.Errorf("%s: DRMP has the M-bit", name)
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
	failure := SmDeliveryFailureCause{SmEnumeratedDeliveryFailureCause: SmEnumeratedDeliveryFailureCauseScCongestion,
		SmDiagnosticInfo: diam.Some([]byte{1})}
	walk("SM-Delivery-Failure-Cause", []diam.RawAVP{avp3(AVPSmDeliveryFailureCause, true, failure.AppendAVPs(nil))}, false, false, false)
	if !checked[[2]uint32{301, 0}] {
		t.Error("DRMP is not in the samples")
	}
	for c := range mS6c {
		if !checked[[2]uint32{c, vendor3GPP}] {
			t.Errorf("3GPP AVP %d is not in the samples", c)
		}
	}
	if !checked[[2]uint32{628, vendor3GPP}] {
		t.Error("Supported-Features is not in the samples")
	}
}

// SM-Delivery-Failure-Cause is listed in table 5.3.3.1/2 but carried by no
// S6c command; its group still round-trips and checks { SM-Enumerated-
// Delivery-Failure-Cause }.
func TestSmDeliveryFailureCause(t *testing.T) {
	in := SmDeliveryFailureCause{SmEnumeratedDeliveryFailureCause: SmEnumeratedDeliveryFailureCauseUserNotScUser,
		SmDiagnosticInfo: diam.Some([]byte{0x01, 0x02})}
	var out SmDeliveryFailureCause
	if err := out.DecodeAVPs(in.AppendAVPs(nil)); err != nil {
		t.Fatal(err)
	}
	if err := out.Validate(); err != nil {
		t.Fatal(err)
	}
	if out.SmEnumeratedDeliveryFailureCause != SmEnumeratedDeliveryFailureCauseUserNotScUser || !bytes.Equal(out.SmDiagnosticInfo.V, []byte{1, 2}) {
		t.Fatalf("SM-Delivery-Failure-Cause = %+v", out)
	}
	var empty SmDeliveryFailureCause
	if err := empty.DecodeAVPs(diam.AppendRaw(nil, avp3(AVPSmDiagnosticInfo, true, []byte{1}))); err != nil {
		t.Fatal(err)
	}
	ae := wantResult(t, empty.Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPSmEnumeratedDeliveryFailureCause {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
}

func TestMissingRequired(t *testing.T) {
	for _, c := range []struct {
		sample       int
		code, vendor uint32
	}{
		{0, base.AVPDestinationRealm, 0},
		{1, base.AVPAuthSessionState, 0},
		{2, AVPScAddress, vendor3GPP},
		{2, AVPUserIdentifier, vendor3GPP},
		{3, base.AVPOriginHost, 0},
		{4, AVPUserIdentifier, vendor3GPP},
		{4, AVPScAddress, vendor3GPP},
		{4, AVPSmDeliveryOutcome, vendor3GPP},
		{5, base.AVPOriginRealm, 0},
	} {
		b := dropAVP(t, encode(t, samples()[c.sample]), c.code, c.vendor)
		ae := wantResult(t, decode(t, b).Validate(), diam.ResultMissingAVP)
		if ae.AVP.Code != c.code || ae.AVP.Vendor != c.vendor {
			t.Fatalf("sample %d: AVP = %+v, want code %d vendor %d", c.sample, ae.AVP, c.code, c.vendor)
		}
	}
}

func TestTooMany(t *testing.T) {
	// { SC-Address } twice in an ALR.
	ae := wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[2]), AVPScAddress)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPScAddress {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// [ Serving-Node ] at most once in an SRA.
	ae = wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[1]), AVPServingNode)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPServingNode {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// [ Failed-AVP ] twice in an ALA.
	ae = wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[3]), base.AVPFailedAvp)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != base.AVPFailedAvp {
		t.Fatalf("AVP = %+v, want Failed-AVP", ae.AVP)
	}
	// [ SM-Delivery-Cause ] twice in MME-SM-Delivery-Outcome, inside SM-Delivery-Outcome.
	cause := avp3(AVPSmDeliveryCause, true, []byte{0, 0, 0, 1})
	outcome := diam.AppendRaw(nil, avp3(AVPMmeSmDeliveryOutcome, true, join([]diam.RawAVP{cause, cause})))
	rdr := encode(t, samples()[4])
	avps := avpsOf(t, rdr)
	for i := range avps {
		if avps[i].Code == AVPSmDeliveryOutcome {
			avps[i].Data = outcome
		}
	}
	ae = wantResult(t, decode(t, withAVPs(t, rdr, avps)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPSmDeliveryCause || !slices.Equal(ae.Path, []uint32{AVPSmDeliveryOutcome, AVPMmeSmDeliveryOutcome}) {
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
	// SRR-Flags is Unsigned32.
	ae := wantResult(t, decode(t, replace(encode(t, samples()[0]), AVPSrrFlags, []byte{0, 1})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPSrrFlags {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// Maximum-UE-Availability-Time is Time.
	ae = wantResult(t, decode(t, replace(encode(t, samples()[2]), AVPMaximumUeAvailabilityTime, []byte{1})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPMaximumUeAvailabilityTime {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// IP-SM-GW-SBI-Support-Indicator is Enumerated, inside Serving-Node.
	sn := diam.AppendRaw(nil, avp3(AVPIpSmGwSbiSupportIndicator, false, []byte{1}))
	sra := &SendRoutingInfoForSmAnswer{SessionId: "s", AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
		Unknown: []diam.RawAVP{avp3(AVPServingNode, true, sn)}}
	ae = wantResult(t, decode(t, encode(t, sra)).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPIpSmGwSbiSupportIndicator || !slices.Equal(ae.Path, []uint32{AVPServingNode}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

func TestMisplacedFixed(t *testing.T) {
	srr := encode(t, samples()[0])
	avps := avpsOf(t, srr)
	avps[0], avps[1] = avps[1], avps[0]
	ae := wantResult(t, decode(t, withAVPs(t, srr, avps)).Validate(), diam.ResultInvalidAVPValue)
	if ae.AVP.Code != base.AVPSessionId {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
}

func TestUnknownAVPs(t *testing.T) {
	srr := encode(t, samples()[0])
	quiet := diam.RawAVP{Code: 9999, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte("later release")}
	m := decode(t, withAVPs(t, srr, append(avpsOf(t, srr), quiet)))
	if err := m.Validate(); err != nil {
		t.Fatalf("unknown AVP without M-bit: %v", err)
	}
	loud := quiet
	loud.Flags |= diam.AVPFlagMandatory
	ae := wantResult(t, decode(t, withAVPs(t, srr, append(avpsOf(t, srr), loud))).Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != 9999 {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// Inside MME-SM-Delivery-Outcome, a group without *[ AVP ]: the same rule.
	rdr := samples()[4].(*ReportSmDeliveryStatusRequest)
	rdr.SmDeliveryOutcome.MmeSmDeliveryOutcome.Unknown = []diam.RawAVP{loud}
	ae = wantResult(t, decode(t, encode(t, rdr)).Validate(), diam.ResultAVPUnsupported)
	if !slices.Equal(ae.Path, []uint32{AVPSmDeliveryOutcome, AVPMmeSmDeliveryOutcome}) {
		t.Fatalf("Path = %v", ae.Path)
	}
}

// Release 11 peers send only what TS 29.338 V11.0.0 knew.
func TestRelease11Messages(t *testing.T) {
	for _, m := range []diam.Message{
		&SendRoutingInfoForSmRequest{
			SessionId: "gmsc;1", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: gmsc, OriginRealm: realm,
			DestinationRealm: realm, Msisdn: diam.Some(msisdn), ScAddress: diam.Some(scAddress), SrrFlags: diam.Some(uint32(1)),
		},
		&SendRoutingInfoForSmAnswer{
			SessionId: "gmsc;1", ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			UserName: diam.Some(imsi), ServingNode: &ServingNode{MscNumber: diam.Some(mscNumber), IpSmGwNumber: diam.Some(ipsmNumber)},
			Lmsi: diam.Some(lmsi), MwdStatus: diam.Some(uint32(2)),
		},
		&AlertServiceCentreRequest{
			SessionId: "hss;1", AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, DestinationRealm: realm,
			ScAddress: scAddress, UserIdentifier: UserIdentifier{Msisdn: diam.Some(msisdn)},
		},
		&ReportSmDeliveryStatusRequest{
			SessionId: "gmsc;2", AuthSessionState: nsm, OriginHost: gmsc, OriginRealm: realm, DestinationRealm: realm,
			UserIdentifier: UserIdentifier{Msisdn: diam.Some(msisdn)}, ScAddress: scAddress,
			SmDeliveryOutcome: SmDeliveryOutcome{MscSmDeliveryOutcome: &DeliveryOutcome{SmDeliveryCause: diam.Some(SmDeliveryCauseAbsentUser)}},
		},
	} {
		if err := decode(t, encode(t, m)).Validate(); err != nil {
			t.Fatalf("Rel-11 %T: %v", m, err)
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
		case *SendRoutingInfoForSmRequest:
			ans = NewSendRoutingInfoForSmAnswerFor(r)
		case *AlertServiceCentreRequest:
			ans = NewAlertServiceCentreAnswerFor(r)
		case *ReportSmDeliveryStatusRequest:
			ans = NewReportSmDeliveryStatusAnswerFor(r)
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
		"SM-RP-MTI SM_STATUS_REPORT (§5.3.3.2)":                            {int32(SmRpMtiSmStatusReport), 1},
		"SM-Delivery-Not-Intended ONLY_MCC_MNC_REQUESTED (§5.3.3.5)":       {int32(SmDeliveryNotIntendedOnlyMccMncRequested), 1},
		"SM-Delivery-Cause SUCCESSFUL_TRANSFER (§5.3.3.19)":                {int32(SmDeliveryCauseSuccessfulTransfer), 2},
		"SM-Enumerated-Delivery-Failure-Cause USER_NOT_SC-USER (§6.3.3.6)": {int32(SmEnumeratedDeliveryFailureCauseUserNotScUser), 6},
		"SM-Enumerated-Delivery-Failure-Cause SC-CONGESTION":               {int32(SmEnumeratedDeliveryFailureCauseScCongestion), 4},
		"SMSF-3GPP-SBI-Support-Indicator SUPPORT_SBI (§5.3.3.37)":          {int32(Smsf3GppSbiSupportIndicatorSupportSbi), 1},
		"SMSF-Non-3GPP-SBI-Support-Indicator SUPPORT_SBI (§5.3.3.38)":      {int32(SmsfNon3GppSbiSupportIndicatorSupportSbi), 1},
		"IP-SM-GW-SBI-Support-Indicator NOT_SUPPORT_SBI (§5.3.3.39)":       {int32(IpSmGwSbiSupportIndicatorNotSupportSbi), 0},
		"DRMP PRIORITY_15 (RFC 7944 §9.1)":                                 {int32(DrmpPriority15), 15},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}

// Codes restated from TS 29.338 tables 5.3.3.1/1 and 6.3.3.1/1 and the
// specifications of the re-used AVPs. The dictionary cross-check in
// internal/interop covers the AVPs go-diameter knows; this covers the ones
// it does not.
func TestAVPCodes(t *testing.T) {
	for name, c := range map[string][2]uint32{
		"SC-Address":                              {AVPScAddress, 3300},
		"SM-Delivery-Failure-Cause":               {AVPSmDeliveryFailureCause, 3303},
		"SM-Enumerated-Delivery-Failure-Cause":    {AVPSmEnumeratedDeliveryFailureCause, 3304},
		"SM-Diagnostic-Info":                      {AVPSmDiagnosticInfo, 3305},
		"SM-RP-MTI":                               {AVPSmRpMti, 3308},
		"SM-RP-SMEA":                              {AVPSmRpSmea, 3309},
		"SRR-Flags":                               {AVPSrrFlags, 3310},
		"SM-Delivery-Not-Intended":                {AVPSmDeliveryNotIntended, 3311},
		"MWD-Status":                              {AVPMwdStatus, 3312},
		"MME-Absent-User-Diagnostic-SM":           {AVPMmeAbsentUserDiagnosticSm, 3313},
		"MSC-Absent-User-Diagnostic-SM":           {AVPMscAbsentUserDiagnosticSm, 3314},
		"SGSN-Absent-User-Diagnostic-SM":          {AVPSgsnAbsentUserDiagnosticSm, 3315},
		"SM-Delivery-Outcome":                     {AVPSmDeliveryOutcome, 3316},
		"MME-SM-Delivery-Outcome":                 {AVPMmeSmDeliveryOutcome, 3317},
		"MSC-SM-Delivery-Outcome":                 {AVPMscSmDeliveryOutcome, 3318},
		"SGSN-SM-Delivery-Outcome":                {AVPSgsnSmDeliveryOutcome, 3319},
		"IP-SM-GW-SM-Delivery-Outcome":            {AVPIpSmGwSmDeliveryOutcome, 3320},
		"SM-Delivery-Cause":                       {AVPSmDeliveryCause, 3321},
		"Absent-User-Diagnostic-SM":               {AVPAbsentUserDiagnosticSm, 3322},
		"RDR-Flags":                               {AVPRdrFlags, 3323},
		"SMSMI-Correlation-ID":                    {AVPSmsmiCorrelationId, 3324},
		"HSS-ID":                                  {AVPHssId, 3325},
		"Originating-SIP-URI":                     {AVPOriginatingSipUri, 3326},
		"Destination-SIP-URI":                     {AVPDestinationSipUri, 3327},
		"Maximum-UE-Availability-Time":            {AVPMaximumUeAvailabilityTime, 3329},
		"SMS-GMSC-Alert-Event":                    {AVPSmsGmscAlertEvent, 3333},
		"SMSF-3GPP-Absent-User-Diagnostic-SM":     {AVPSmsf3gppAbsentUserDiagnosticSm, 3334},
		"SMSF-Non-3GPP-Absent-User-Diagnostic-SM": {AVPSmsfNon3gppAbsentUserDiagnosticSm, 3335},
		"SMSF-3GPP-SM-Delivery-Outcome":           {AVPSmsf3gppSmDeliveryOutcome, 3336},
		"SMSF-Non-3GPP-SM-Delivery-Outcome":       {AVPSmsfNon3gppSmDeliveryOutcome, 3337},
		"SMSF-3GPP-Number":                        {AVPSmsf3gppNumber, 3338},
		"SMSF-Non-3GPP-Number":                    {AVPSmsfNon3gppNumber, 3339},
		"SMSF-3GPP-Name":                          {AVPSmsf3gppName, 3340},
		"SMSF-Non-3GPP-Name":                      {AVPSmsfNon3gppName, 3341},
		"SMSF-3GPP-Realm":                         {AVPSmsf3gppRealm, 3342},
		"SMSF-Non-3GPP-Realm":                     {AVPSmsfNon3gppRealm, 3343},
		"SMSF-3GPP-Address":                       {AVPSmsf3gppAddress, 3344},
		"SMSF-Non-3GPP-Address":                   {AVPSmsfNon3gppAddress, 3345},
		"SMSF-3GPP-SBI-Support-Indicator":         {AVPSmsf3gppSbiSupportIndicator, 3346},
		"SMSF-Non-3GPP-SBI-Support-Indicator":     {AVPSmsfNon3gppSbiSupportIndicator, 3347},
		"IP-SM-GW-SBI-Support-Indicator":          {AVPIpSmGwSbiSupportIndicator, 3348},
		"LMSI":                                    {AVPLmsi, 2400},
		"Serving-Node":                            {AVPServingNode, 2401},
		"Additional-Serving-Node":                 {AVPAdditionalServingNode, 2406},
		"User-Identifier":                         {AVPUserIdentifier, 3102},
		"IP-SM-GW-Realm":                          {AVPIpSmGwRealm, 3112},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}
