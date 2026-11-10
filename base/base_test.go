package base

import (
	"bytes"
	"errors"
	"net/netip"
	"reflect"
	"testing"

	"github.com/hsdfat/go-diam/diam"
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

func avps(t *testing.T, msg []byte) []diam.RawAVP {
	t.Helper()
	_, data, err := diam.ParseHeader(msg)
	if err != nil {
		t.Fatal(err)
	}
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

// commandCode reads the command code of an encoded message. The Header of a
// hand-built message only has it after encoding.
func commandCode(t *testing.T, msg []byte) uint32 {
	t.Helper()
	h, _, err := diam.ParseHeader(msg)
	if err != nil {
		t.Fatal(err)
	}
	return h.CommandCode
}

var (
	hdr       = diam.Header{HopByHop: 0x11, EndToEnd: 0x22}
	proxyInfo = []ProxyInfo{{ProxyHost: "dra.example.org", ProxyState: []byte{1, 2, 3}}}
	failedAvp = &FailedAvp{Unknown: []diam.RawAVP{
		{Code: AVPOriginStateId, Flags: diam.AVPFlagMandatory, Data: []byte{1, 2}}, // wrong length: the kind of AVP a Failed-AVP carries
	}}
)

// samples returns one realistic message per base command, in command order.
func samples() []diam.Message {
	addrs := []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("2001:db8::1")}
	vsa := []VendorSpecificApplicationId{{VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777251))}}
	return []diam.Message{
		&CapabilitiesExchangeRequest{
			Header: hdr, OriginHost: "mme.example.org", OriginRealm: "example.org", HostIpAddress: addrs,
			VendorId: 10415, ProductName: "go-diam", OriginStateId: diam.Some(uint32(7)),
			SupportedVendorId: []uint32{10415, 13019}, AuthApplicationId: []uint32{16777251},
			InbandSecurityId: []uint32{0}, AcctApplicationId: []uint32{3},
			VendorSpecificApplicationId: vsa, FirmwareRevision: diam.Some(uint32(1)),
		},
		&CapabilitiesExchangeAnswer{
			Header: hdr, ResultCode: 2001, OriginHost: "hss.example.org", OriginRealm: "example.org", HostIpAddress: addrs[:1],
			VendorId: 10415, ProductName: "go-diam", OriginStateId: diam.Some(uint32(8)),
			ErrorMessage: diam.Some("ok"), FailedAvp: failedAvp, SupportedVendorId: []uint32{10415},
			AuthApplicationId: []uint32{16777251}, InbandSecurityId: []uint32{0}, AcctApplicationId: []uint32{3},
			VendorSpecificApplicationId: vsa, FirmwareRevision: diam.Some(uint32(2)),
		},
		&DeviceWatchdogRequest{Header: hdr, OriginHost: "mme.example.org", OriginRealm: "example.org", OriginStateId: diam.Some(uint32(7))},
		&DeviceWatchdogAnswer{
			Header: hdr, ResultCode: 2001, OriginHost: "hss.example.org", OriginRealm: "example.org",
			ErrorMessage: diam.Some("ok"), FailedAvp: failedAvp, OriginStateId: diam.Some(uint32(8)),
		},
		&DisconnectPeerRequest{Header: hdr, OriginHost: "mme.example.org", OriginRealm: "example.org", DisconnectCause: DisconnectCauseBusy},
		&DisconnectPeerAnswer{
			Header: hdr, ResultCode: 2001, OriginHost: "hss.example.org", OriginRealm: "example.org",
			ErrorMessage: diam.Some("bye"), FailedAvp: failedAvp,
		},
		&ReAuthRequest{
			Header: hdr, SessionId: "hss;1;2", OriginHost: "hss.example.org", OriginRealm: "example.org",
			DestinationRealm: "example.org", DestinationHost: "mme.example.org", AuthApplicationId: 16777251,
			ReAuthRequestType: ReAuthRequestTypeAuthorizeAuthenticate, UserName: diam.Some("001010000000001"),
			OriginStateId: diam.Some(uint32(8)), ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&ReAuthAnswer{
			Header: hdr, SessionId: "hss;1;2", ResultCode: 2001, OriginHost: "mme.example.org", OriginRealm: "example.org",
			UserName: diam.Some("001010000000001"), OriginStateId: diam.Some(uint32(7)), ErrorMessage: diam.Some("ok"),
			ErrorReportingHost: diam.Some("mme.example.org"), FailedAvp: failedAvp,
			RedirectHost: []string{"aaa://alt.example.org:3868"}, RedirectHostUsage: diam.Some(RedirectHostUsageAllRealm),
			RedirectMaxCacheTime: diam.Some(uint32(60)), ProxyInfo: proxyInfo,
		},
		&SessionTerminationRequest{
			Header: hdr, SessionId: "mme;1;2", OriginHost: "mme.example.org", OriginRealm: "example.org",
			DestinationRealm: "example.org", AuthApplicationId: 16777251, TerminationCause: TerminationCauseLogout,
			UserName: diam.Some("001010000000001"), DestinationHost: diam.Some("hss.example.org"),
			Class: [][]byte{{1}, {2, 3}}, OriginStateId: diam.Some(uint32(7)), ProxyInfo: proxyInfo,
			RouteRecord: []string{"dra.example.org"},
		},
		&SessionTerminationAnswer{
			Header: hdr, SessionId: "mme;1;2", ResultCode: 2001, OriginHost: "hss.example.org", OriginRealm: "example.org",
			UserName: diam.Some("001010000000001"), Class: [][]byte{{1}}, ErrorMessage: diam.Some("ok"),
			ErrorReportingHost: diam.Some("hss.example.org"), FailedAvp: failedAvp, OriginStateId: diam.Some(uint32(8)),
			RedirectHost: []string{"aaa://alt.example.org:3868"}, RedirectHostUsage: diam.Some(RedirectHostUsageDontCache),
			RedirectMaxCacheTime: diam.Some(uint32(60)), ProxyInfo: proxyInfo,
		},
		&AbortSessionRequest{
			Header: hdr, SessionId: "hss;1;2", OriginHost: "hss.example.org", OriginRealm: "example.org",
			DestinationRealm: "example.org", DestinationHost: "mme.example.org", AuthApplicationId: 16777251,
			UserName: diam.Some("001010000000001"), OriginStateId: diam.Some(uint32(8)), ProxyInfo: proxyInfo,
			RouteRecord: []string{"dra.example.org"},
		},
		&AbortSessionAnswer{
			Header: hdr, SessionId: "hss;1;2", ResultCode: 2001, OriginHost: "mme.example.org", OriginRealm: "example.org",
			UserName: diam.Some("001010000000001"), OriginStateId: diam.Some(uint32(7)), ErrorMessage: diam.Some("ok"),
			ErrorReportingHost: diam.Some("mme.example.org"), FailedAvp: failedAvp,
			RedirectHost: []string{"aaa://alt.example.org:3868"}, RedirectHostUsage: diam.Some(RedirectHostUsageAllUser),
			RedirectMaxCacheTime: diam.Some(uint32(60)), ProxyInfo: proxyInfo,
		},
	}
}

func TestBaseRoundTrip(t *testing.T) {
	wantCodes := []uint32{257, 257, 280, 280, 282, 282, 258, 258, 275, 275, 274, 274}
	for i, in := range samples() {
		name := reflect.TypeOf(in).Elem().Name()
		b1, err := in.AppendMarshal(nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		m, err := diam.Decode(b1)
		if err != nil {
			t.Fatalf("%s: Decode: %v", name, err)
		}
		if reflect.TypeOf(m) != reflect.TypeOf(in) {
			t.Fatalf("%s: Decode returned %T", name, m)
		}
		if err := m.Validate(); err != nil {
			t.Fatalf("%s: Validate: %v", name, err)
		}
		h := m.DiamHeader()
		if h.CommandCode != wantCodes[i] || h.AppID != 0 || h.HopByHop != 0x11 || h.EndToEnd != 0x22 {
			t.Fatalf("%s: header = %+v", name, h)
		}
		if isRequest := i%2 == 0; (h.Flags&diam.FlagRequest != 0) != isRequest {
			t.Fatalf("%s: R-bit = %v", name, !isRequest)
		}
		b2, err := m.AppendMarshal(nil)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b1, b2) {
			t.Fatalf("%s: re-encode differs\n%x\n%x", name, b1, b2)
		}
	}
}

func TestDecodedValues(t *testing.T) {
	b, _ := samples()[0].AppendMarshal(nil)
	var cer CapabilitiesExchangeRequest
	if err := cer.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if cer.OriginHost != "mme.example.org" || cer.VendorId != 10415 || cer.ProductName != "go-diam" ||
		len(cer.HostIpAddress) != 2 || cer.HostIpAddress[1].String() != "2001:db8::1" ||
		!reflect.DeepEqual(cer.SupportedVendorId, []uint32{10415, 13019}) ||
		len(cer.VendorSpecificApplicationId) != 1 || cer.VendorSpecificApplicationId[0].AuthApplicationId.V != 16777251 ||
		cer.VendorSpecificApplicationId[0].AcctApplicationId.Set {
		t.Fatalf("CER = %+v", cer)
	}

	b, _ = samples()[8].AppendMarshal(nil)
	var str SessionTerminationRequest
	if err := str.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if str.SessionId != "mme;1;2" || str.TerminationCause != TerminationCauseLogout || len(str.Class) != 2 ||
		!bytes.Equal(str.Class[1], []byte{2, 3}) || str.ProxyInfo[0].ProxyHost != "dra.example.org" || str.RouteRecord[0] != "dra.example.org" {
		t.Fatalf("STR = %+v", str)
	}
}

func TestCERHostIPAddressMin(t *testing.T) {
	cer := &CapabilitiesExchangeRequest{OriginHost: "a", OriginRealm: "b", VendorId: 10415, ProductName: "go-diam"}
	b, _ := cer.Marshal()
	var got CapabilitiesExchangeRequest
	if err := got.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	ae := wantResult(t, got.Validate(), diam.ResultMissingAVP)
	if ae.AVP.Code != AVPHostIpAddress {
		t.Fatalf("AVP = %+v, want Host-IP-Address", ae.AVP)
	}
}

func TestFlagsBase(t *testing.T) {
	for _, m := range samples() {
		b, _ := m.AppendMarshal(nil)
		code := commandCode(t, b)
		proxiable := b[4]&diam.FlagProxiable != 0
		// RFC 6733: CER/CEA, DWR/DWA, DPR/DPA are not proxiable; RAR/RAA,
		// STR/STA, ASR/ASA are.
		want := code == 258 || code == 275 || code == 274
		if proxiable != want {
			t.Errorf("%T: P-bit = %v, want %v", m, proxiable, want)
		}
	}
}

func TestMBitFollowsRFCTable(t *testing.T) {
	// RFC 6733 §4.5: every base AVP has the M-bit except Product-Name,
	// Firmware-Revision, Error-Message and Error-Reporting-Host.
	noM := map[uint32]bool{AVPProductName: true, AVPFirmwareRevision: true, AVPErrorMessage: true, AVPErrorReportingHost: true}
	seen := map[uint32]bool{}
	for _, m := range samples() {
		b, _ := m.AppendMarshal(nil)
		for _, a := range avps(t, b) {
			seen[a.Code] = true
			if a.Vendor != 0 || a.Flags&diam.AVPFlagVendor != 0 {
				t.Errorf("%T: AVP %d has a vendor", m, a.Code)
			}
			if got := a.Mandatory(); got == noM[a.Code] {
				t.Errorf("%T: AVP %d M-bit = %v", m, a.Code, got)
			}
		}
	}
	for code := range noM {
		if !seen[code] {
			t.Errorf("AVP %d not covered by the samples", code)
		}
	}
}

func TestSessionIdIsFirst(t *testing.T) {
	checked := 0
	for _, m := range samples() {
		b, _ := m.AppendMarshal(nil)
		if code := commandCode(t, b); code != 258 && code != 275 && code != 274 {
			continue
		}
		checked++
		if first := avps(t, b)[0]; first.Code != AVPSessionId {
			t.Errorf("%T: first AVP is %d, want Session-Id", m, first.Code)
		}
	}
	if checked != 6 {
		t.Fatalf("checked %d session commands, want 6", checked)
	}
}

func TestNewCEAFor(t *testing.T) {
	cer := samples()[0].(*CapabilitiesExchangeRequest)
	cer.Header = diam.Header{Version: 1, Flags: diam.FlagRequest, CommandCode: 257, HopByHop: 5, EndToEnd: 6}
	cea := NewCapabilitiesExchangeAnswerFor(cer)
	if want := (diam.Header{Version: 1, CommandCode: 257, HopByHop: 5, EndToEnd: 6}); cea.Header != want {
		t.Fatalf("CEA header = %+v, want %+v", cea.Header, want)
	}
	if cea.OriginHost != "" || cea.ResultCode != 0 {
		t.Fatalf("CEA has AVPs set: %+v", cea)
	}

	str := samples()[8].(*SessionTerminationRequest)
	sta := NewSessionTerminationAnswerFor(str)
	if sta.SessionId != "mme;1;2" || sta.Header.HopByHop != 0x11 {
		t.Fatalf("STA = %+v", sta)
	}
}

func TestAnswerWithFailedAvp(t *testing.T) {
	x := diam.RawAVP{Code: 9999, Vendor: 10415, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte("offending")}
	sta := &SessionTerminationAnswer{
		SessionId: "s", ResultCode: 5001, OriginHost: "h", OriginRealm: "r",
		FailedAvp: &FailedAvp{Unknown: []diam.RawAVP{x}},
	}
	b1, _ := sta.Marshal()
	m, err := diam.Decode(b1)
	if err != nil {
		t.Fatal(err)
	}
	got := m.(*SessionTerminationAnswer)
	// A Failed-AVP carries AVPs the peer could not accept, typically with the
	// M-bit: they are its content, not unknown AVPs of this message.
	if err := got.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.FailedAvp == nil || !reflect.DeepEqual(got.FailedAvp.Unknown, []diam.RawAVP{x}) {
		t.Fatalf("Failed-AVP content = %+v, want %+v", got.FailedAvp, x)
	}
	b2, _ := got.Marshal()
	if !bytes.Equal(b1, b2) {
		t.Fatal("re-encode differs")
	}
}

func TestEnumValuesFollowRFC(t *testing.T) {
	for name, c := range map[string][2]int32{
		"Auth-Session-State NO_STATE_MAINTAINED":          {int32(AuthSessionStateNoStateMaintained), 1},
		"Disconnect-Cause REBOOTING":                      {int32(DisconnectCauseRebooting), 0},
		"Disconnect-Cause DO_NOT_WANT_TO_TALK_TO_YOU":     {int32(DisconnectCauseDoNotWantToTalkToYou), 2},
		"Re-Auth-Request-Type AUTHORIZE_ONLY":             {int32(ReAuthRequestTypeAuthorizeOnly), 0},
		"Re-Auth-Request-Type AUTHORIZE_AUTHENTICATE":     {int32(ReAuthRequestTypeAuthorizeAuthenticate), 1},
		"Auth-Request-Type AUTHENTICATE_ONLY":             {int32(AuthRequestTypeAuthenticateOnly), 1},
		"Auth-Request-Type AUTHORIZE_AUTHENTICATE":        {int32(AuthRequestTypeAuthorizeAuthenticate), 3},
		"Termination-Cause LOGOUT":                        {int32(TerminationCauseLogout), 1},
		"Termination-Cause SESSION_TIMEOUT":               {int32(TerminationCauseSessionTimeout), 8},
		"Redirect-Host-Usage DONT_CACHE":                  {int32(RedirectHostUsageDontCache), 0},
		"Redirect-Host-Usage ALL_USER":                    {int32(RedirectHostUsageAllUser), 6},
		"Session-Server-Failover REFUSE_SERVICE":          {int32(SessionServerFailoverRefuseService), 0},
		"Session-Server-Failover TRY_AGAIN_ALLOW_SERVICE": {int32(SessionServerFailoverTryAgainAllowService), 3},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %d, want %d", name, c[0], c[1])
		}
	}
	if got := TerminationCauseLogout.String(); got != "LOGOUT" {
		t.Errorf("TerminationCauseLogout.String() = %q", got)
	}
}
