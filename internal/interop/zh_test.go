package interop

import (
	"bytes"
	"testing"
	"time"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/cx"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/zh"
)

// zhApp is the Zh application id (TS 29.109 §6.0).
const zhApp = 16777221

// go-diameter v4.6.0 ships no Zh application: no 16777221 dictionary, and
// none of the GAA AVPs of TS 29.109 table 6.1. Of the Cx AVPs that Zh uses
// "as they are" (§6.3) it defines SIP-Auth-Data-Item and its AKA members
// (SWx). The Zh interop below is built on those, as the Cx one is (see
// cx_test.go).
//
// The AVPs go-diameter has nowhere are listed in zhRestated with code,
// vendor and M-bit restated from TS 29.109 table 6.1 and, for the Cx AVPs,
// TS 29.229 table 6.3.0.1, here, in the test, not read from go-diam's
// schema. Their value encoding is still go-diameter's. TestFiorixZhGaps
// keeps the list exact.
var zhRestated = map[string]restated{
	"GBA-UserSecSettings":     {400, tgpp, true, "TS 29.109 table 6.1"},
	"GUSS-Timestamp":          {409, tgpp, false, "TS 29.109 table 6.1"},
	"SHA-256-Indication":      {421, tgpp, true, "TS 29.109 table 6.1"},
	"Public-Identity":         {601, tgpp, true, "TS 29.229 table 6.3.0.1"},
	"SIP-Digest-Authenticate": {635, tgpp, false, "TS 29.229 table 6.3.0.1"},
	"Digest-Realm":            {104, 0, true, "TS 29.229 table 6.3.0.1"},
	"Digest-QoP":              {110, 0, true, "TS 29.229 table 6.3.0.1"},
	"Digest-Algorithm":        {111, 0, true, "TS 29.229 table 6.3.0.1"},
	"Digest-HA1":              {121, 0, true, "TS 29.229 table 6.3.0.1"},
}

// fzh builds a go-diameter message of the Zh application.
func fzh(cmd uint32, flags uint8, avps ...*fd.AVP) *fd.Message {
	m := fd.NewMessage(cmd, flags, zhApp, hopByHop, endToEnd, dict.Default)
	for _, a := range avps {
		m.AddAVP(a)
	}
	return m
}

// zhPairs is MAR and MAA, built with go-diam and with go-diameter: a MAR for
// a synchronization failure carrying every AVP of the grammar, an AKA MAA
// with every AVP of the grammar, a SIP Digest MAA, and an IDENTITY_UNKNOWN
// (5401) answer.
func zhPairs() []pair {
	const (
		bsf, hss, realm = "bsf.ims.example.org", "hss.ims.example.org", "ims.example.org"
		impi            = "001010000000001@ims.example.org"
		impu            = "sip:+15551234567@ims.example.org"
		aka             = "Digest-AKAv1-MD5"
	)
	stamp := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	auts := bytes.Repeat([]byte{0x5a}, 30)
	key16 := bytes.Repeat([]byte{0x3c}, 16)
	rand := bytes.Repeat([]byte{0xa5}, 32)
	xres := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	guss := []byte(`<?xml version="1.0" encoding="UTF-8"?><guss xmlns="urn:3gpp:gba:GBAGUSSSchema-R9:2010-02" id="1"><bsfInfo/><ussList/></guss>`)
	nsm := base.AuthSessionStateNoStateMaintained
	// No per-message M-bit rule: Zh carries no Supported-Features.
	q, s := perAVP{true, zhRestated, nil}, perAVP{false, zhRestated, nil}

	session := func(v string) *fd.AVP { return fa(avp.SessionID, datatype.UTF8String(v)) }
	authState := func() *fd.AVP { return enum(avp.AuthSessionState, 1) }
	gVSA := base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(zhApp))}
	fVSA := func() *fd.AVP {
		return group(avp.VendorSpecificApplicationID, u32(avp.VendorID, tgpp), u32(avp.AuthApplicationID, zhApp))
	}
	gProxy := []base.ProxyInfo{{ProxyHost: "dra.ims.example.org", ProxyState: []byte{1, 2, 3}}}
	fProxy := func() *fd.AVP {
		return group(avp.ProxyInfo, ident(avp.ProxyHost, "dra.ims.example.org"), octets(avp.ProxyState, 1, 2, 3))
	}
	fRoute := func() *fd.AVP { return ident(avp.RouteRecord, "dra.ims.example.org") }

	const req, pxy = fd.RequestFlag, fd.ProxiableFlag
	return []pair{
		{"MAR",
			&zh.MultimediaAuthRequest{
				Header: hdr, SessionId: "bsf;1;1", VendorSpecificApplicationId: gVSA, AuthSessionState: nsm,
				OriginHost: bsf, OriginRealm: realm, DestinationRealm: realm, DestinationHost: diam.Some(hss),
				UserName: diam.Some(impi), PublicIdentity: diam.Some(impu),
				SipAuthDataItem: &cx.SipAuthDataItem{SipAuthenticationScheme: diam.Some(aka), SipAuthorization: diam.Some(auts)},
				GussTimestamp:   diam.Some(stamp), ProxyInfo: gProxy, RouteRecord: []string{"dra.ims.example.org"},
			},
			fzh(303, req|pxy, session("bsf;1;1"), fVSA(), authState(), ident(avp.OriginHost, bsf), ident(avp.OriginRealm, realm),
				ident(avp.DestinationRealm, realm), ident(avp.DestinationHost, hss), utf8(avp.UserName, impi),
				q.r("Public-Identity", datatype.UTF8String(impu)),
				q.g(avp.SIPAuthDataItem, tgpp, q.a(avp.SIPAuthenticationScheme, tgpp, datatype.UTF8String(aka)),
					q.a(avp.SIPAuthorization, tgpp, datatype.OctetString(auts))),
				q.r("GUSS-Timestamp", datatype.Time(stamp)), fProxy(), fRoute()),
		},
		{"MAA",
			&zh.MultimediaAuthAnswer{
				Header: hdr, SessionId: "bsf;1;1", VendorSpecificApplicationId: gVSA, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, UserName: diam.Some(impi), PublicIdentity: diam.Some(impu),
				SipAuthDataItem: &cx.SipAuthDataItem{SipItemNumber: diam.Some(uint32(1)), SipAuthenticationScheme: diam.Some(aka),
					SipAuthenticate: diam.Some(rand), SipAuthorization: diam.Some(xres),
					ConfidentialityKey: diam.Some(key16), IntegrityKey: diam.Some(key16)},
				GbaUserSecSettings: diam.Some(guss), Sha256Indication: diam.Some(zh.Sha256IndicationUiccSupportSha256),
				ProxyInfo: gProxy, RouteRecord: []string{"dra.ims.example.org"},
			},
			fzh(303, pxy, session("bsf;1;1"), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), utf8(avp.UserName, impi), s.r("Public-Identity", datatype.UTF8String(impu)),
				s.g(avp.SIPAuthDataItem, tgpp, s.a(avp.SIPItemNumber, tgpp, datatype.Unsigned32(1)),
					s.a(avp.SIPAuthenticationScheme, tgpp, datatype.UTF8String(aka)),
					s.a(avp.SIPAuthenticate, tgpp, datatype.OctetString(rand)), s.a(avp.SIPAuthorization, tgpp, datatype.OctetString(xres)),
					s.a(avp.ConfidentialityKey, tgpp, datatype.OctetString(key16)), s.a(avp.IntegrityKey, tgpp, datatype.OctetString(key16))),
				s.r("GBA-UserSecSettings", datatype.OctetString(guss)), s.r("SHA-256-Indication", datatype.Enumerated(1)),
				fProxy(), fRoute()),
		},
		// SIP Digest (§4.2 step 2): the vector is a SIP-Digest-Authenticate.
		{"MAA-digest",
			&zh.MultimediaAuthAnswer{
				Header: hdr, SessionId: "bsf;1;2", VendorSpecificApplicationId: gVSA, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, UserName: diam.Some(impi),
				SipAuthDataItem: &cx.SipAuthDataItem{SipAuthenticationScheme: diam.Some("SIP Digest"),
					SipDigestAuthenticate: &cx.SipDigestAuthenticate{DigestRealm: realm, DigestAlgorithm: diam.Some("MD5"),
						DigestQop: "auth", DigestHa1: "939e7578"}},
			},
			fzh(303, pxy, session("bsf;1;2"), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), utf8(avp.UserName, impi),
				s.g(avp.SIPAuthDataItem, tgpp, s.a(avp.SIPAuthenticationScheme, tgpp, datatype.UTF8String("SIP Digest")),
					s.rg("SIP-Digest-Authenticate", s.r("Digest-Realm", datatype.UTF8String(realm)),
						s.r("Digest-Algorithm", datatype.UTF8String("MD5")), s.r("Digest-QoP", datatype.UTF8String("auth")),
						s.r("Digest-HA1", datatype.UTF8String("939e7578"))))),
		},
		// DIAMETER_ERROR_IDENTITY_UNKNOWN (5401, §6.2.2.1).
		{"MAA-unknown",
			&zh.MultimediaAuthAnswer{
				Header: hdr, SessionId: "bsf;1;3", VendorSpecificApplicationId: gVSA,
				ExperimentalResult: &base.ExperimentalResult{VendorId: tgpp, ExperimentalResultCode: 5401},
				AuthSessionState:   nsm, OriginHost: hss, OriginRealm: realm,
			},
			fzh(303, pxy, session("bsf;1;3"), fVSA(),
				group(avp.ExperimentalResult, u32(avp.VendorID, tgpp), u32(avp.ExperimentalResultCode, 5401)),
				authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm)),
		},
	}
}

// TestFiorixZhGaps states what go-diameter v4.6.0 lacks for Zh and keeps
// zhRestated exact: an entry go-diameter gains must move to the independent
// side.
func TestFiorixZhGaps(t *testing.T) {
	if _, err := dict.Default.App(zhApp); err == nil {
		t.Errorf("go-diameter now has a Zh (%d) dictionary: build the Zh interop on it and drop the per-AVP lookup", zhApp)
	}
	for name, d := range zhRestated {
		if _, def := fiorixHome(d.code, d.vendor); def != nil {
			t.Errorf("go-diameter now defines %s (%d vendor %d) in application %s: remove it from zhRestated", name, d.code, d.vendor, def.App.Name)
		}
	}
}

// go-diameter encodes; go-diam decodes, validates and re-encodes the same
// bytes, and its own message of the same content is the same bytes.
func TestZhFiorixToGoDiam(t *testing.T) {
	for _, p := range zhPairs() {
		t.Run(p.name, func(t *testing.T) { checkFiorixToGoDiam(t, p) })
	}
}

// go-diam encodes; go-diameter reads the message (it has no Zh dictionary)
// and decodes every AVP in the application that defines it
// (checkGoDiamToFiorixByAVP in cx_test.go).
func TestZhGoDiamToFiorix(t *testing.T) {
	for _, p := range zhPairs() {
		t.Run(p.name, func(t *testing.T) { checkGoDiamToFiorixByAVP(t, p, zhApp, zhRestated) })
	}
}
