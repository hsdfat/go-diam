package sh

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
	// TS 29.329 table 6.1.1.
	wantCodes := []uint32{306, 306, 307, 307, 308, 308, 309, 309}
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
		// §6: application id 16777217.
		if h.CommandCode != wantCodes[i] || h.AppID != 16777217 || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
			t.Fatalf("%s: header = %+v", name, h)
		}
		if isRequest := i%2 == 0; (h.Flags&diam.FlagRequest != 0) != isRequest {
			t.Fatalf("%s: R-bit = %v", name, !isRequest)
		}
		// Every Sh command header carries PXY (§6.1.1-6.1.8).
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
	var udr UserDataRequest
	if err := udr.Unmarshal(encode(t, samples()[0])); err != nil {
		t.Fatal(err)
	}
	if udr.UserIdentity.PublicIdentity.V != impu || udr.WildcardedImpu.V != wildIMPU ||
		!slices.Equal(udr.DataReference, []DataReference{DataReferenceRepositoryData, DataReferenceLocationInformation, DataReferenceCsrn}) ||
		udr.IdentitySet[1] != IdentitySetAliasIdentities || udr.RequestedNodes.V != 0x09 || udr.UdrFlags.V != 0x03 ||
		string(udr.CallReferenceInfo.AsNumber) != "\x91\x51\x55" || string(udr.ServiceIndication[1]) != "svc2" {
		t.Fatalf("UDR = %+v", udr)
	}

	var pur ProfileUpdateRequest
	if err := pur.Unmarshal(encode(t, samples()[2])); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pur.UserIdentity.Msisdn.V, msisdn) || pur.UserIdentity.PublicIdentity.Set || !bytes.Equal(pur.UserData, userData) ||
		len(pur.DataReference) != 2 {
		t.Fatalf("PUR = %+v", pur)
	}

	var pua ProfileUpdateAnswer
	if err := pua.Unmarshal(encode(t, samples()[3])); err != nil {
		t.Fatal(err)
	}
	if pua.RepositoryDataId == nil || pua.RepositoryDataId.SequenceNumber != 3 || pua.ExperimentalResult.ExperimentalResultCode != 5105 ||
		pua.DataReference.V != DataReferenceRepositoryData || pua.Load[0].LoadValue.V != 1000 || pua.OcOlr.OcSequenceNumber != 9 {
		t.Fatalf("PUA = %+v", pua)
	}

	var snr SubscribeNotificationsRequest
	if err := snr.Unmarshal(encode(t, samples()[4])); err != nil {
		t.Fatal(err)
	}
	if snr.UserIdentity.ExternalIdentifier.V != "device-1@iot.example.org" || !snr.ExpiryTime.V.Equal(expiry) ||
		snr.SubsReqType != SubsReqTypeSubscribe || snr.OneTimeNotification.V != OneTimeNotificationOneTimeNotificationRequested ||
		snr.WildcardedPublicIdentity.V != wildPSI || snr.WildcardedImpu.Set {
		t.Fatalf("SNR = %+v", snr)
	}
}

// mSh is the M-bit table 6.3.1 gives each Sh AVP of vendor 3GPP, and table
// 6.3/2 gives External-Identifier: "Must" for User-Identity to
// Current-Location (700-707), DSAI-Tag (711), Server-Name (602),
// Public-Identity (601) and External-Identifier (3111), "Must not" for the
// others. Supported-Features (628) is "May"; see TestFlagsFollowSpec. It is
// restated here from the tables, not read from the schema.
var mSh = map[uint32]bool{
	601: true, 602: true, 629: false, 630: false, 631: false, 634: false, 636: false, 650: false,
	700: true, 701: true, 702: true, 703: true, 704: true, 705: true, 706: true, 707: true,
	708: false, 709: false, 710: false, 711: true, 712: false, 713: false, 714: false, 715: false,
	716: false, 717: false, 718: false, 719: false, 720: false, 721: false, 722: false,
	3111: true,
}

// mIETF is the M-bit of the IETF AVPs TS 29.329 re-uses: table 6.3.1 for
// DRMP, OC-Supported-Features, OC-OLR and Load ("Must not"); RFC 7683 and
// RFC 8583 for the AVPs inside OC-OLR and Load.
var mIETF = map[uint32]bool{
	301: false, 621: false, 622: false, 623: false, 624: false, 625: false, 626: false, 627: false,
	649: false, 650: false, 651: false, 652: false,
}

// groupCodes are the grouped AVPs, so the flag check can walk into them
// without consulting the schema.
var groupCodes = map[[2]uint32]bool{
	{284, 0}: true, {297, 0}: true, {279, 0}: true, {260, 0}: true, {621, 0}: true, {623, 0}: true, {650, 0}: true,
	{628, vendor3GPP}: true, {631, vendor3GPP}: true, {700, vendor3GPP}: true, {715, vendor3GPP}: true, {720, vendor3GPP}: true,
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
				// TS 29.229 §7.2.1 (applied by §7.1): set in a request, never in an answer.
				checked[k] = true
				if !top || a.Mandatory() != request {
					t.Errorf("%s: Supported-Features: M-bit = %v in a request=%v (top level %v)", name, a.Mandatory(), request, top)
				}
			case a.Vendor == vendor3GPP:
				if want, ok := mSh[a.Code]; ok {
					checked[k] = true
					if a.Mandatory() != want {
						t.Errorf("%s: AVP %d: M-bit = %v, table 6.3.1 says %v", name, a.Code, a.Mandatory(), want)
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
	for c := range mIETF {
		if !checked[[2]uint32{c, 0}] {
			t.Errorf("IETF AVP %d is not in the samples", c)
		}
	}
	// Every 3GPP AVP of tables 6.3.1 and 6.3/2 is in the samples, except
	// Supported-Applications (631), which no command carries.
	for c := range mSh {
		if c != 631 && !checked[[2]uint32{c, vendor3GPP}] {
			t.Errorf("3GPP AVP %d is not in the samples", c)
		}
	}
	if !checked[[2]uint32{628, vendor3GPP}] {
		t.Error("Supported-Features is not in the samples")
	}
}

// Supported-Applications is carried by no Sh command; its group still
// round-trips.
func TestSupportedApplications(t *testing.T) {
	in := SupportedApplications{AuthApplicationId: []uint32{16777217}, AcctApplicationId: []uint32{3},
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
	for _, c := range []struct {
		sample       int
		code, vendor uint32
	}{
		{0, AVPUserIdentity, vendor3GPP},
		// *{ Data-Reference } needs one (RFC 6733 §3.2).
		{0, AVPDataReference, vendor3GPP},
		// §6.1 NOTE: still { } in every command.
		{0, base.AVPVendorSpecificApplicationId, 0},
		{0, base.AVPDestinationRealm, 0},
		{2, AVPUserData, vendor3GPP},
		{2, AVPDataReference, vendor3GPP},
		{2, AVPUserIdentity, vendor3GPP},
		{4, AVPSubsReqType, vendor3GPP},
		{4, AVPDataReference, vendor3GPP},
		{6, base.AVPDestinationHost, 0},
		{6, AVPUserData, vendor3GPP},
		{6, AVPUserIdentity, vendor3GPP},
		{7, base.AVPAuthSessionState, 0},
	} {
		b := dropAVP(t, encode(t, samples()[c.sample]), c.code, c.vendor)
		ae := wantResult(t, decode(t, b).Validate(), diam.ResultMissingAVP)
		if ae.AVP.Code != c.code || ae.AVP.Vendor != c.vendor {
			t.Fatalf("sample %d: AVP = %+v, want code %d vendor %d", c.sample, ae.AVP, c.code, c.vendor)
		}
	}
}

// Required members of the Sh grouped AVPs: 5005 with the path.
func TestMissingInNestedGroup(t *testing.T) {
	// { Sequence-Number } in Repository-Data-ID (§6.3.24), inside a PUA.
	rdi := diam.AppendRaw(nil, diam.RawAVP{Code: AVPServiceIndication, Vendor: vendor3GPP,
		Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte("svc")})
	pua := &ProfileUpdateAnswer{SessionId: "s", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
		Unknown: []diam.RawAVP{grp(AVPRepositoryDataId, false, rdi)}}
	ae := wantResult(t, decode(t, encode(t, pua)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPSequenceNumber || !slices.Equal(ae.Path, []uint32{AVPRepositoryDataId}) {
		t.Fatalf("AVPError = %+v", ae)
	}

	// { AS-Number } in Call-Reference-Info (§6.3.29), inside a UDR.
	udr := samples()[0].(*UserDataRequest)
	cri := diam.AppendRaw(nil, diam.RawAVP{Code: AVPCallReferenceNumber, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte{1}})
	udr.CallReferenceInfo = nil
	udr.Unknown = []diam.RawAVP{grp(AVPCallReferenceInfo, false, cri)}
	ae = wantResult(t, decode(t, encode(t, udr)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPAsNumber || !slices.Equal(ae.Path, []uint32{AVPCallReferenceInfo}) {
		t.Fatalf("AVPError = %+v", ae)
	}

	// { Feature-List } in Supported-Features, inside a PNA.
	sf := join(slices.DeleteFunc(split(t, features[0].AppendAVPs(nil)), func(a diam.RawAVP) bool { return a.Code == AVPFeatureList }))
	pna := &PushNotificationAnswer{SessionId: "s", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: as, OriginRealm: realm,
		Unknown: []diam.RawAVP{grp(AVPSupportedFeatures, false, sf)}}
	ae = wantResult(t, decode(t, encode(t, pna)).Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPFeatureList || !slices.Equal(ae.Path, []uint32{AVPSupportedFeatures}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

func TestTooMany(t *testing.T) {
	// { User-Identity } twice in a UDR.
	ae := wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[0]), AVPUserIdentity)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPUserIdentity {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// [ Data-Reference ] is at most one in a PUA.
	ae = wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[3]), AVPDataReference)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPDataReference {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// [ Wildcarded-IMPU ] twice in a PNR.
	ae = wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[6]), AVPWildcardedImpu)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPWildcardedImpu {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// [ Failed-AVP ] (current release) twice in a PNA.
	ae = wantResult(t, decode(t, doubleAVP(t, encode(t, samples()[7]), base.AVPFailedAvp)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != base.AVPFailedAvp {
		t.Fatalf("AVP = %+v, want Failed-AVP", ae.AVP)
	}
	// [ MSISDN ] twice inside User-Identity.
	pur := samples()[2].(*ProfileUpdateRequest)
	pur.UserIdentity.Unknown = []diam.RawAVP{{Code: AVPMsisdn, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: msisdn}}
	ae = wantResult(t, decode(t, encode(t, pur)).Validate(), diam.ResultAVPOccursTooMany)
	if ae.AVP.Code != AVPMsisdn || !slices.Equal(ae.Path, []uint32{AVPUserIdentity}) {
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
	// UDR-Flags is Unsigned32: two octets is a bad length.
	ae := wantResult(t, decode(t, replace(encode(t, samples()[0]), AVPUdrFlags, []byte{0, 1})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPUdrFlags {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// Expiry-Time is Time (four octets).
	ae = wantResult(t, decode(t, replace(encode(t, samples()[4]), AVPExpiryTime, []byte{1, 2, 3})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPExpiryTime {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// Data-Reference is Enumerated (four octets).
	ae = wantResult(t, decode(t, replace(encode(t, samples()[2]), AVPDataReference, []byte{0})).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPDataReference {
		t.Fatalf("AVP = %+v", ae.AVP)
	}
	// Sequence-Number is Unsigned32, inside Repository-Data-ID.
	rdi := join([]diam.RawAVP{
		{Code: AVPServiceIndication, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte("svc")},
		{Code: AVPSequenceNumber, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte{0, 0, 0, 0, 3}},
	})
	pua := &ProfileUpdateAnswer{SessionId: "s", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
		Unknown: []diam.RawAVP{grp(AVPRepositoryDataId, false, rdi)}}
	ae = wantResult(t, decode(t, encode(t, pua)).Validate(), diam.ResultInvalidAVPLength)
	if ae.AVP.Code != AVPSequenceNumber || !slices.Equal(ae.Path, []uint32{AVPRepositoryDataId}) {
		t.Fatalf("AVPError = %+v", ae)
	}
}

func TestMisplacedFixed(t *testing.T) {
	// Session-Id is < > in every command.
	udr := encode(t, samples()[0])
	avps := avpsOf(t, udr)
	avps[0], avps[1] = avps[1], avps[0]
	ae := wantResult(t, decode(t, withAVPs(t, udr, avps)).Validate(), diam.ResultInvalidAVPValue)
	if ae.AVP.Code != base.AVPSessionId {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// OC-OLR ::= < OC-Sequence-Number > < OC-Report-Type > ... (RFC 7683 §7.3).
	olr := split(t, ocOlr.AppendAVPs(nil))
	olr[0], olr[1] = olr[1], olr[0]
	sna := &SubscribeNotificationsAnswer{SessionId: "s", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: hss,
		OriginRealm: realm, Unknown: []diam.RawAVP{{Code: AVPOcOlr, Data: join(olr)}}}
	ae = wantResult(t, decode(t, encode(t, sna)).Validate(), diam.ResultInvalidAVPValue)
	if !slices.Equal(ae.Path, []uint32{AVPOcOlr}) {
		t.Fatalf("Path = %v", ae.Path)
	}
}

func TestUnknownAVPs(t *testing.T) {
	udr := encode(t, samples()[0])
	quiet := diam.RawAVP{Code: 9999, Vendor: vendor3GPP, Flags: diam.AVPFlagVendor, Data: []byte("later release")}
	m := decode(t, withAVPs(t, udr, append(avpsOf(t, udr), quiet)))
	if err := m.Validate(); err != nil {
		t.Fatalf("unknown AVP without M-bit: %v", err)
	}
	if got := m.(*UserDataRequest).Unknown; len(got) != 1 || got[0].Code != 9999 {
		t.Fatalf("Unknown = %+v", got)
	}

	loud := quiet
	loud.Flags |= diam.AVPFlagMandatory
	ae := wantResult(t, decode(t, withAVPs(t, udr, append(avpsOf(t, udr), loud))).Validate(), diam.ResultAVPUnsupported)
	if ae.AVP.Code != 9999 {
		t.Fatalf("AVP = %+v", ae.AVP)
	}

	// The same inside User-Identity, a group with *[ AVP ].
	pnr := samples()[6].(*PushNotificationRequest)
	pnr.UserIdentity.Unknown = []diam.RawAVP{loud}
	ae = wantResult(t, decode(t, encode(t, pnr)).Validate(), diam.ResultAVPUnsupported)
	if !slices.Equal(ae.Path, []uint32{AVPUserIdentity}) {
		t.Fatalf("Path = %v", ae.Path)
	}
}

// Wildcarded-IMPU (§6.3.20) and Wildcarded-Public-Identity (§6.3.19) are
// both decoded into their fields in every command that lists them, never
// left in Unknown. TS 29.328 forbids a request carrying both; that is a
// procedure rule, so such a request still passes Validate and the
// application decides.
func TestWildcardedIdentities(t *testing.T) {
	for i, m := range samples() {
		got := decode(t, encode(t, m))
		v := reflect.ValueOf(got).Elem()
		if u := v.FieldByName("Unknown"); u.Len() != 0 {
			t.Fatalf("%T: Unknown = %v", got, u.Interface())
		}
		if i == 7 { // PNA lists neither
			continue
		}
		if !v.FieldByName("WildcardedImpu").IsValid() || !v.FieldByName("WildcardedPublicIdentity").IsValid() {
			t.Fatalf("%T lacks a wildcarded identity field", got)
		}
	}

	both := samples()[0].(*UserDataRequest)
	both.WildcardedPublicIdentity = diam.Some(wildPSI)
	got := decode(t, encode(t, both)).(*UserDataRequest)
	if err := got.Validate(); err != nil {
		t.Fatalf("UDR with both wildcarded identities: %v", err)
	}
	if got.WildcardedImpu.V != wildIMPU || got.WildcardedPublicIdentity.V != wildPSI {
		t.Fatalf("wildcarded = %+v / %+v", got.WildcardedImpu, got.WildcardedPublicIdentity)
	}
	// Written in grammar order: Wildcarded-Public-Identity, then Wildcarded-IMPU.
	var order []uint32
	for _, a := range avpsOf(t, encode(t, both)) {
		if a.Vendor == vendor3GPP && (a.Code == AVPWildcardedPublicIdentity || a.Code == AVPWildcardedImpu) {
			order = append(order, a.Code)
		}
	}
	if want := []uint32{AVPWildcardedPublicIdentity, AVPWildcardedImpu}; !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

// Release 8 peers send only what TS 29.329 V8.9.0 knew; their messages
// must validate.
func TestRelease8Messages(t *testing.T) {
	for _, m := range []diam.Message{
		&UserDataRequest{
			SessionId: "as;1", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: as, OriginRealm: realm,
			DestinationRealm: realm, UserIdentity: UserIdentity{PublicIdentity: diam.Some(impu)},
			DataReference: []DataReference{DataReferenceImsUserState},
		},
		&UserDataAnswer{
			SessionId: "as;1", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm,
			OriginHost: hss, OriginRealm: realm, UserData: diam.Some(userData),
		},
		// V8.9.0 §6.1.3: exactly one { Data-Reference }.
		&ProfileUpdateRequest{
			SessionId: "as;2", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: as, OriginRealm: realm,
			DestinationRealm: realm, UserIdentity: UserIdentity{Msisdn: diam.Some(msisdn)},
			DataReference: []DataReference{DataReferenceRepositoryData}, UserData: userData,
		},
		&SubscribeNotificationsRequest{
			SessionId: "as;3", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: as, OriginRealm: realm,
			DestinationRealm: realm, UserIdentity: UserIdentity{PublicIdentity: diam.Some(impu)}, SubsReqType: SubsReqTypeUnsubscribe,
			DataReference: []DataReference{DataReferenceInitialFilterCriteria},
		},
		&PushNotificationRequest{
			SessionId: "hss;1", VendorSpecificApplicationId: vsai, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			DestinationHost: as, DestinationRealm: realm, UserIdentity: UserIdentity{PublicIdentity: diam.Some(impu)}, UserData: userData,
		},
	} {
		if err := decode(t, encode(t, m)).Validate(); err != nil {
			t.Fatalf("Rel-8 %T: %v", m, err)
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
		case *UserDataRequest:
			ans = NewUserDataAnswerFor(r)
		case *ProfileUpdateRequest:
			ans = NewProfileUpdateAnswerFor(r)
		case *SubscribeNotificationsRequest:
			ans = NewSubscribeNotificationsAnswerFor(r)
		case *PushNotificationRequest:
			ans = NewPushNotificationAnswerFor(r)
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
		"Data-Reference RepositoryData (§6.3.4)":                                  {int32(DataReferenceRepositoryData), 0},
		"Data-Reference IMSPublicIdentity":                                        {int32(DataReferenceImsPublicIdentity), 10},
		"Data-Reference S-CSCFName":                                               {int32(DataReferenceSCscfName), 12},
		"Data-Reference DSAI":                                                     {int32(DataReferenceDsai), 19},
		"Data-Reference ServiceLevelTraceInfo (20 is reserved)":                   {int32(DataReferenceServiceLevelTraceInfo), 21},
		"Data-Reference UEReachabilityForIP":                                      {int32(DataReferenceUeReachabilityForIp), 25},
		"Data-Reference STN-SR":                                                   {int32(DataReferenceStnSr), 27},
		"Data-Reference CSRN":                                                     {int32(DataReferenceCsrn), 30},
		"Data-Reference IMEISV":                                                   {int32(DataReferenceImeisv), 34},
		"Data-Reference UE-5G-SRVCC-Capability":                                   {int32(DataReferenceUe5gSrvccCapability), 35},
		"Subs-Req-Type Unsubscribe (§6.3.6)":                                      {int32(SubsReqTypeUnsubscribe), 1},
		"Requested-Domain PS-Domain (§6.3.7)":                                     {int32(RequestedDomainPsDomain), 1},
		"Current-Location InitiateActiveLocationRetrieval (§6.3.8)":               {int32(CurrentLocationInitiateActiveLocationRetrieval), 1},
		"Identity-Set ALIAS_IDENTITIES (§6.3.10)":                                 {int32(IdentitySetAliasIdentities), 3},
		"Send-Data-Indication USER_DATA_REQUESTED (§6.3.17)":                      {int32(SendDataIndicationUserDataRequested), 1},
		"Session-Priority PRIORITY-4 (§6.3.21)":                                   {int32(SessionPriorityPriority4), 4},
		"One-Time-Notification ONE_TIME_NOTIFICATION_REQUESTED (§6.3.22)":         {int32(OneTimeNotificationOneTimeNotificationRequested), 0},
		"Serving-Node-Indication ONLY_SERVING_NODES_REQUIRED (§6.3.23)":           {int32(ServingNodeIndicationOnlyServingNodesRequired), 0},
		"Pre-paging-Supported PREPAGING_SUPPORTED (§6.3.26)":                      {int32(PrePagingSupportedPrepagingSupported), 1},
		"Local-Time-Zone-Indication LOCAL_TIME_ZONE_WITH_LOCATION_INFO (§6.3.27)": {int32(LocalTimeZoneIndicationLocalTimeZoneWithLocationInfoRequested), 1},
		"DRMP PRIORITY_15 (RFC 7944 §9.1)":                                        {int32(DrmpPriority15), 15},
		"OC-Report-Type REALM_REPORT (RFC 7683 §7.6)":                             {int32(OcReportTypeRealmReport), 1},
		"Load-Type PEER (RFC 8583 §7.2)":                                          {int32(LoadTypePeer), 1},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
	if got := DataReferenceSCscfName.String(); got != "S_CSCF_NAME" {
		t.Errorf("DataReferenceSCscfName.String() = %q", got)
	}
	if got := DataReference(20).String(); got != "20" {
		t.Errorf("reserved Data-Reference String() = %q", got)
	}
}

// Codes the generated constants must carry, restated from TS 29.329 table
// 6.3.1, TS 29.336 §6.4.11 and the RFCs of the re-used AVPs. The dictionary
// cross-check in internal/interop covers the AVPs go-diameter knows; this
// covers the ones it does not.
func TestAVPCodes(t *testing.T) {
	for name, c := range map[string][2]uint32{
		"Public-Identity":            {AVPPublicIdentity, 601},
		"Supported-Applications":     {AVPSupportedApplications, 631},
		"Wildcarded-Public-Identity": {AVPWildcardedPublicIdentity, 634},
		"Wildcarded-IMPU":            {AVPWildcardedImpu, 636},
		"User-Identity":              {AVPUserIdentity, 700},
		"User-Data":                  {AVPUserData, 702},
		"Data-Reference":             {AVPDataReference, 703},
		"Service-Indication":         {AVPServiceIndication, 704},
		"Subs-Req-Type":              {AVPSubsReqType, 705},
		"Requested-Domain":           {AVPRequestedDomain, 706},
		"Current-Location":           {AVPCurrentLocation, 707},
		"Identity-Set":               {AVPIdentitySet, 708},
		"Expiry-Time":                {AVPExpiryTime, 709},
		"Send-Data-Indication":       {AVPSendDataIndication, 710},
		"DSAI-Tag":                   {AVPDsaiTag, 711},
		"One-Time-Notification":      {AVPOneTimeNotification, 712},
		"Requested-Nodes":            {AVPRequestedNodes, 713},
		"Serving-Node-Indication":    {AVPServingNodeIndication, 714},
		"Repository-Data-ID":         {AVPRepositoryDataId, 715},
		"Sequence-Number":            {AVPSequenceNumber, 716},
		"Pre-paging-Supported":       {AVPPrePagingSupported, 717},
		"Local-Time-Zone-Indication": {AVPLocalTimeZoneIndication, 718},
		"UDR-Flags":                  {AVPUdrFlags, 719},
		"Call-Reference-Info":        {AVPCallReferenceInfo, 720},
		"Call-Reference-Number":      {AVPCallReferenceNumber, 721},
		"AS-Number":                  {AVPAsNumber, 722},
		"External-Identifier":        {AVPExternalIdentifier, 3111},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
}
