// Package interop cross-checks go-diam against fiorix/go-diameter, an
// independent implementation with its own dictionary and encoder.
package interop

import (
	"bytes"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"testing"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
)

const (
	hopByHop = 0x11
	endToEnd = 0x22
)

var hdr = diam.Header{HopByHop: hopByHop, EndToEnd: endToEnd}

// fa builds a go-diameter AVP. The code comes from go-diameter's own
// constants and the M-bit from its own dictionary, so nothing on this side of
// the comparison is derived from go-diam's schema.
func fa(code uint32, data datatype.Type) *fd.AVP {
	def, err := dict.Default.FindAVPByCode(0, code, 0)
	if err != nil {
		panic(err)
	}
	var flags uint8
	if strings.Contains(def.Must, "M") {
		flags = avp.Mbit
	}
	return fd.NewAVP(code, flags, 0, data)
}

func u32(code, v uint32) *fd.AVP          { return fa(code, datatype.Unsigned32(v)) }
func ident(code uint32, s string) *fd.AVP { return fa(code, datatype.DiameterIdentity(s)) }
func utf8(code uint32, s string) *fd.AVP  { return fa(code, datatype.UTF8String(s)) }
func enum(code uint32, v int32) *fd.AVP   { return fa(code, datatype.Enumerated(v)) }
func octets(code uint32, b ...byte) *fd.AVP {
	return fa(code, datatype.OctetString(b))
}
func uri(code uint32, s string) *fd.AVP { return fa(code, datatype.DiameterURI(s)) }
func addr(s string) *fd.AVP {
	ip := net.ParseIP(s)
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return fa(avp.HostIPAddress, datatype.Address(ip))
}
func group(code uint32, avps ...*fd.AVP) *fd.AVP {
	return fa(code, &fd.GroupedAVP{AVP: avps})
}

// fmsg builds a go-diameter message of the base application.
func fmsg(cmd uint32, flags uint8, avps ...*fd.AVP) *fd.Message {
	m := fd.NewMessage(cmd, flags, 0, hopByHop, endToEnd, dict.Default)
	for _, a := range avps {
		m.AddAVP(a)
	}
	return m
}

const (
	req = fd.RequestFlag
	pxy = fd.ProxiableFlag
)

// pair is the same message built twice: with go-diam and with go-diameter.
type pair struct {
	name   string
	godiam diam.Message
	fiorix *fd.Message
}

func pairs() []pair {
	const (
		mme, hss, realm = "mme.example.org", "hss.example.org", "example.org"
		imsi            = "001010000000001"
		s6a             = 16777251
	)
	gProxy := []base.ProxyInfo{{ProxyHost: "dra.example.org", ProxyState: []byte{1, 2, 3}}}
	fProxy := func() *fd.AVP {
		return group(avp.ProxyInfo, ident(avp.ProxyHost, "dra.example.org"), octets(avp.ProxyState, 1, 2, 3))
	}
	// The Failed-AVP content is a well-formed AVP, so go-diameter can decode it.
	gFailed := &base.FailedAvp{Unknown: []diam.RawAVP{{Code: base.AVPOriginStateId, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 9}}}}
	fFailed := func() *fd.AVP { return group(avp.FailedAVP, u32(avp.OriginStateID, 9)) }
	gVSA := []base.VendorSpecificApplicationId{{VendorId: 10415, AuthApplicationId: diam.Some(uint32(s6a))}}
	fVSA := func() *fd.AVP {
		return group(avp.VendorSpecificApplicationID, u32(avp.VendorID, 10415), u32(avp.AuthApplicationID, s6a))
	}
	addrs := []netip.Addr{netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("2001:db8::1")}

	return []pair{
		{"CER",
			&base.CapabilitiesExchangeRequest{
				Header: hdr, OriginHost: mme, OriginRealm: realm, HostIpAddress: addrs, VendorId: 10415, ProductName: "go-diam",
				OriginStateId: diam.Some(uint32(7)), SupportedVendorId: []uint32{10415, 13019}, AuthApplicationId: []uint32{s6a},
				InbandSecurityId: []uint32{0}, AcctApplicationId: []uint32{3}, VendorSpecificApplicationId: gVSA,
				FirmwareRevision: diam.Some(uint32(1)),
			},
			fmsg(257, req,
				ident(avp.OriginHost, mme), ident(avp.OriginRealm, realm), addr("192.0.2.1"), addr("2001:db8::1"),
				u32(avp.VendorID, 10415), utf8(avp.ProductName, "go-diam"), u32(avp.OriginStateID, 7),
				u32(avp.SupportedVendorID, 10415), u32(avp.SupportedVendorID, 13019), u32(avp.AuthApplicationID, s6a),
				u32(avp.InbandSecurityID, 0), u32(avp.AcctApplicationID, 3), fVSA(), u32(avp.FirmwareRevision, 1)),
		},
		{"CEA",
			&base.CapabilitiesExchangeAnswer{
				Header: hdr, ResultCode: 2001, OriginHost: hss, OriginRealm: realm, HostIpAddress: addrs[:1], VendorId: 10415,
				ProductName: "go-diam", OriginStateId: diam.Some(uint32(8)), ErrorMessage: diam.Some("ok"), FailedAvp: gFailed,
				SupportedVendorId: []uint32{10415}, AuthApplicationId: []uint32{s6a}, InbandSecurityId: []uint32{0},
				AcctApplicationId: []uint32{3}, VendorSpecificApplicationId: gVSA, FirmwareRevision: diam.Some(uint32(2)),
			},
			fmsg(257, 0,
				u32(avp.ResultCode, 2001), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm), addr("192.0.2.1"),
				u32(avp.VendorID, 10415), utf8(avp.ProductName, "go-diam"), u32(avp.OriginStateID, 8),
				utf8(avp.ErrorMessage, "ok"), fFailed(), u32(avp.SupportedVendorID, 10415), u32(avp.AuthApplicationID, s6a),
				u32(avp.InbandSecurityID, 0), u32(avp.AcctApplicationID, 3), fVSA(), u32(avp.FirmwareRevision, 2)),
		},
		{"DWR",
			&base.DeviceWatchdogRequest{Header: hdr, OriginHost: mme, OriginRealm: realm, OriginStateId: diam.Some(uint32(7))},
			fmsg(280, req, ident(avp.OriginHost, mme), ident(avp.OriginRealm, realm), u32(avp.OriginStateID, 7)),
		},
		{"DWA",
			&base.DeviceWatchdogAnswer{
				Header: hdr, ResultCode: 2001, OriginHost: hss, OriginRealm: realm, ErrorMessage: diam.Some("ok"),
				FailedAvp: gFailed, OriginStateId: diam.Some(uint32(8)),
			},
			fmsg(280, 0, u32(avp.ResultCode, 2001), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				utf8(avp.ErrorMessage, "ok"), fFailed(), u32(avp.OriginStateID, 8)),
		},
		{"DPR",
			&base.DisconnectPeerRequest{Header: hdr, OriginHost: mme, OriginRealm: realm, DisconnectCause: base.DisconnectCauseBusy},
			fmsg(282, req, ident(avp.OriginHost, mme), ident(avp.OriginRealm, realm), enum(avp.DisconnectCause, 1)),
		},
		{"DPA",
			&base.DisconnectPeerAnswer{
				Header: hdr, ResultCode: 2001, OriginHost: hss, OriginRealm: realm, ErrorMessage: diam.Some("bye"), FailedAvp: gFailed,
			},
			fmsg(282, 0, u32(avp.ResultCode, 2001), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				utf8(avp.ErrorMessage, "bye"), fFailed()),
		},
		{"RAR",
			&base.ReAuthRequest{
				Header: hdr, SessionId: "hss;1;2", OriginHost: hss, OriginRealm: realm, DestinationRealm: realm, DestinationHost: mme,
				AuthApplicationId: s6a, ReAuthRequestType: base.ReAuthRequestTypeAuthorizeAuthenticate, UserName: diam.Some(imsi),
				OriginStateId: diam.Some(uint32(8)), ProxyInfo: gProxy, RouteRecord: []string{"dra.example.org"},
			},
			fmsg(258, req|pxy, utf8(avp.SessionID, "hss;1;2"), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				ident(avp.DestinationRealm, realm), ident(avp.DestinationHost, mme), u32(avp.AuthApplicationID, s6a),
				enum(avp.ReAuthRequestType, 1), utf8(avp.UserName, imsi), u32(avp.OriginStateID, 8), fProxy(),
				ident(avp.RouteRecord, "dra.example.org")),
		},
		{"RAA",
			&base.ReAuthAnswer{
				Header: hdr, SessionId: "hss;1;2", ResultCode: 2001, OriginHost: mme, OriginRealm: realm, UserName: diam.Some(imsi),
				OriginStateId: diam.Some(uint32(7)), ErrorMessage: diam.Some("ok"), ErrorReportingHost: diam.Some(mme),
				FailedAvp: gFailed, RedirectHost: []string{"aaa://alt.example.org:3868"},
				RedirectHostUsage: diam.Some(base.RedirectHostUsageAllRealm), RedirectMaxCacheTime: diam.Some(uint32(60)), ProxyInfo: gProxy,
			},
			fmsg(258, pxy, utf8(avp.SessionID, "hss;1;2"), u32(avp.ResultCode, 2001), ident(avp.OriginHost, mme),
				ident(avp.OriginRealm, realm), utf8(avp.UserName, imsi), u32(avp.OriginStateID, 7),
				utf8(avp.ErrorMessage, "ok"), ident(avp.ErrorReportingHost, mme), fFailed(),
				uri(avp.RedirectHost, "aaa://alt.example.org:3868"), enum(avp.RedirectHostUsage, 2),
				u32(avp.RedirectMaxCacheTime, 60), fProxy()),
		},
		{"STR",
			&base.SessionTerminationRequest{
				Header: hdr, SessionId: "mme;1;2", OriginHost: mme, OriginRealm: realm, DestinationRealm: realm, AuthApplicationId: s6a,
				TerminationCause: base.TerminationCauseLogout, UserName: diam.Some(imsi), DestinationHost: diam.Some(hss),
				Class: [][]byte{{1}, {2, 3}}, OriginStateId: diam.Some(uint32(7)), ProxyInfo: gProxy, RouteRecord: []string{"dra.example.org"},
			},
			fmsg(275, req|pxy, utf8(avp.SessionID, "mme;1;2"), ident(avp.OriginHost, mme), ident(avp.OriginRealm, realm),
				ident(avp.DestinationRealm, realm), u32(avp.AuthApplicationID, s6a), enum(avp.TerminationCause, 1),
				utf8(avp.UserName, imsi), ident(avp.DestinationHost, hss), octets(avp.Class, 1), octets(avp.Class, 2, 3),
				u32(avp.OriginStateID, 7), fProxy(), ident(avp.RouteRecord, "dra.example.org")),
		},
		{"STA",
			&base.SessionTerminationAnswer{
				Header: hdr, SessionId: "mme;1;2", ResultCode: 2001, OriginHost: hss, OriginRealm: realm, UserName: diam.Some(imsi),
				Class: [][]byte{{1}}, ErrorMessage: diam.Some("ok"), ErrorReportingHost: diam.Some(hss), FailedAvp: gFailed,
				OriginStateId: diam.Some(uint32(8)), RedirectHost: []string{"aaa://alt.example.org:3868"},
				RedirectHostUsage: diam.Some(base.RedirectHostUsageDontCache), RedirectMaxCacheTime: diam.Some(uint32(60)), ProxyInfo: gProxy,
			},
			fmsg(275, pxy, utf8(avp.SessionID, "mme;1;2"), u32(avp.ResultCode, 2001), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), utf8(avp.UserName, imsi), octets(avp.Class, 1),
				utf8(avp.ErrorMessage, "ok"), ident(avp.ErrorReportingHost, hss), fFailed(), u32(avp.OriginStateID, 8),
				uri(avp.RedirectHost, "aaa://alt.example.org:3868"), enum(avp.RedirectHostUsage, 0),
				u32(avp.RedirectMaxCacheTime, 60), fProxy()),
		},
		{"ASR",
			&base.AbortSessionRequest{
				Header: hdr, SessionId: "hss;1;2", OriginHost: hss, OriginRealm: realm, DestinationRealm: realm, DestinationHost: mme,
				AuthApplicationId: s6a, UserName: diam.Some(imsi), OriginStateId: diam.Some(uint32(8)), ProxyInfo: gProxy,
				RouteRecord: []string{"dra.example.org"},
			},
			fmsg(274, req|pxy, utf8(avp.SessionID, "hss;1;2"), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				ident(avp.DestinationRealm, realm), ident(avp.DestinationHost, mme), u32(avp.AuthApplicationID, s6a),
				utf8(avp.UserName, imsi), u32(avp.OriginStateID, 8), fProxy(), ident(avp.RouteRecord, "dra.example.org")),
		},
		{"ASA",
			&base.AbortSessionAnswer{
				Header: hdr, SessionId: "hss;1;2", ResultCode: 2001, OriginHost: mme, OriginRealm: realm, UserName: diam.Some(imsi),
				OriginStateId: diam.Some(uint32(7)), ErrorMessage: diam.Some("ok"), ErrorReportingHost: diam.Some(mme),
				FailedAvp: gFailed, RedirectHost: []string{"aaa://alt.example.org:3868"},
				RedirectHostUsage: diam.Some(base.RedirectHostUsageAllUser), RedirectMaxCacheTime: diam.Some(uint32(60)), ProxyInfo: gProxy,
			},
			fmsg(274, pxy, utf8(avp.SessionID, "hss;1;2"), u32(avp.ResultCode, 2001), ident(avp.OriginHost, mme),
				ident(avp.OriginRealm, realm), utf8(avp.UserName, imsi), u32(avp.OriginStateID, 7),
				utf8(avp.ErrorMessage, "ok"), ident(avp.ErrorReportingHost, mme), fFailed(),
				uri(avp.RedirectHost, "aaa://alt.example.org:3868"), enum(avp.RedirectHostUsage, 6),
				u32(avp.RedirectMaxCacheTime, 60), fProxy()),
		},
	}
}

// allPairs is every interop sample of an application go-diameter has a
// dictionary for: the base protocol, S6a/S6d and S13/S13'.
func allPairs() []pair { return append(append(pairs(), s6aPairs()...), s13Pairs()...) }

// fiorixGaps holds, per application, the AVPs go-diameter is allowed to
// decode as Unknown in that application's messages.
var fiorixGaps = map[uint32]map[[2]uint32]string{s6aApp: fiorixS6aGaps}

// noUnknown reports the first AVP in avps, or nested in them, that
// go-diameter decoded as datatype.Unknown, other than the known gaps of its
// dictionary.
func noUnknown(avps []*fd.AVP, path string, gaps map[[2]uint32]string) error {
	for _, a := range avps {
		where := fmt.Sprintf("%s/%d", path, a.Code)
		switch d := a.Data.(type) {
		case datatype.Unknown:
			if _, gap := gaps[[2]uint32{a.Code, a.VendorID}]; !gap {
				return fmt.Errorf("go-diameter decoded AVP %s (vendor %d) as Unknown", where, a.VendorID)
			}
		case *fd.GroupedAVP:
			if err := noUnknown(d.AVP, where, gaps); err != nil {
				return err
			}
		}
	}
	return nil
}

func serialize(t testing.TB, m *fd.Message) []byte {
	t.Helper()
	b, err := m.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// go-diameter encodes; go-diam decodes, validates and re-encodes the same
// bytes.
func TestFiorixToGoDiam(t *testing.T) {
	for _, p := range allPairs() {
		t.Run(p.name, func(t *testing.T) { checkFiorixToGoDiam(t, p) })
	}
}

func checkFiorixToGoDiam(t *testing.T, p pair) {
	t.Helper()
	wire := serialize(t, p.fiorix)
	m, err := diam.Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	out, err := m.AppendMarshal(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, wire) {
		t.Fatalf("go-diam re-encode differs from go-diameter bytes\n%x\n%x", wire, out)
	}
	// The hand-built go-diam message of the same content encodes to the
	// very same bytes: two encoders, one wire format.
	own, err := p.godiam.AppendMarshal(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(own, wire) {
		t.Fatalf("go-diam encoding differs from go-diameter's\n%x\n%x", wire, own)
	}
}

// go-diam encodes; go-diameter decodes with its own dictionary and
// re-encodes the same bytes.
func TestGoDiamToFiorix(t *testing.T) {
	for _, p := range allPairs() {
		t.Run(p.name, func(t *testing.T) {
			wire, err := p.godiam.AppendMarshal(nil)
			if err != nil {
				t.Fatal(err)
			}
			m, err := fd.ReadMessage(bytes.NewReader(wire), dict.Default)
			if err != nil {
				t.Fatalf("go-diameter ReadMessage: %v", err)
			}
			if m.Header.HopByHopID != hopByHop || m.Header.EndToEndID != endToEnd ||
				m.Header.ApplicationID != p.fiorix.Header.ApplicationID || m.Header.CommandCode != p.fiorix.Header.CommandCode ||
				m.Header.CommandFlags != p.fiorix.Header.CommandFlags {
				t.Fatalf("header = %+v, want %+v", m.Header, p.fiorix.Header)
			}
			// Every AVP, at any depth, must be one go-diameter's dictionary
			// knows for this application: an AVP it decodes as Unknown has a
			// code or vendor go-diameter does not recognise.
			if err := noUnknown(m.AVP, "", fiorixGaps[p.fiorix.Header.ApplicationID]); err != nil {
				t.Fatal(err)
			}

			want := p.fiorix
			for _, code := range []uint32{avp.OriginHost, avp.OriginRealm, avp.SessionID, avp.ResultCode, avp.UserName} {
				wa, werr := want.FindAVP(code, 0)
				ga, gerr := m.FindAVP(code, 0)
				if (werr == nil) != (gerr == nil) {
					t.Fatalf("AVP %d: present in one message only (%v / %v)", code, werr, gerr)
				}
				if werr == nil && ga.Data.String() != wa.Data.String() {
					t.Fatalf("AVP %d = %s, want %s", code, ga.Data, wa.Data)
				}
			}

			again := serialize(t, m)
			if !bytes.Equal(again, wire) {
				t.Fatalf("go-diameter re-encode differs\n%x\n%x", wire, again)
			}
		})
	}
}
