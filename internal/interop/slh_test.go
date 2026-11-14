package interop

import (
	"net/netip"
	"testing"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6a"
	"github.com/hsdfat/go-diam/slh"
)

// slhApp is the SLh application id (TS 29.173 §6.1.8).
const slhApp = 16777291

// go-diameter v4.6.0 ships no SLh application: no 16777291 dictionary and no
// command 8388622. Of the SLh AVPs it defines Serving-Node, MME-Name,
// MME-Realm, GMLC-Address, MSISDN and SGSN-Number (Ro/Rf), GMLC-Number (S6a),
// 3GPP-AAA-Server-Name (SWx) and Supported-Features with Feature-List-ID and
// Feature-List (Ro/Rf, Rx, SWx). The SLh interop below is built on those, as
// the Cx and Sh ones are (see cx_test.go).
//
// The AVPs go-diameter has nowhere are listed in slhRestated with code,
// vendor and M-bit restated from TS 29.173 table 6.4.1/1 here, in the test,
// not read from go-diam's schema. Their value encoding is still
// go-diameter's. TestFiorixSLhGaps keeps the list exact.
var slhRestated = map[string]restated{
	"LMSI":                    {2400, tgpp, true, "table 6.4.1/1"},
	"MSC-Number":              {2403, tgpp, true, "table 6.4.1/1"},
	"LCS-Capabilities-Sets":   {2404, tgpp, true, "table 6.4.1/1"},
	"Additional-Serving-Node": {2406, tgpp, true, "table 6.4.1/1"},
	"PPR-Address":             {2407, tgpp, true, "table 6.4.1/1"},
	"SGSN-Name":               {2409, tgpp, false, "table 6.4.1/1"},
	"SGSN-Realm":              {2410, tgpp, false, "table 6.4.1/1"},
	"RIA-Flags":               {2411, tgpp, false, "table 6.4.1/1"},
}

// fslh builds a go-diameter message of the SLh application.
func fslh(cmd uint32, flags uint8, avps ...*fd.AVP) *fd.Message {
	m := fd.NewMessage(cmd, flags, slhApp, hopByHop, endToEnd, dict.Default)
	for _, a := range avps {
		m.AddAVP(a)
	}
	return m
}

// slhPairs is RIR and RIA, built with go-diam and with go-diameter, covering
// every AVP the two grammars and both grouped AVPs allow, and an
// ABSENT_USER answer (§5.2.1.2 step 3).
func slhPairs() []pair {
	const (
		gmlc, hss, realm = "gmlc.epc.example.org", "hss.epc.example.org", "epc.example.org"
		imsi             = "001010000000001"
		mme, sgsn, aaa   = "mme.epc.example.org", "sgsn.epc.example.org", "aaa.epc.example.org"
	)
	msisdn := []byte{0x51, 0x55, 0x21, 0x43, 0x65, 0xf7}
	gmlcNumber := []byte{0x51, 0x55, 0x00, 0x00, 0x00, 0xf1}
	sgsnNumber := []byte{0x51, 0x55, 0x00, 0x00, 0x00, 0xf2}
	mscNumber := []byte{0x51, 0x55, 0x00, 0x00, 0x00, 0xf3}
	lmsi := []byte{1, 2, 3, 4}
	nsm := base.AuthSessionStateNoStateMaintained
	// SLh has no per-message M-bit rule: Supported-Features keeps
	// go-diameter's "may M", which it sends clear, in both directions.
	q, s := perAVP{true, slhRestated, nil}, perAVP{false, slhRestated, nil}

	session := func(v string) *fd.AVP { return fa(avp.SessionID, datatype.UTF8String(v)) }
	authState := func() *fd.AVP { return enum(avp.AuthSessionState, 1) }
	gVSA := &base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(slhApp))}
	fVSA := func() *fd.AVP {
		return group(avp.VendorSpecificApplicationID, u32(avp.VendorID, tgpp), u32(avp.AuthApplicationID, slhApp))
	}
	gProxy := []base.ProxyInfo{{ProxyHost: "dra.epc.example.org", ProxyState: []byte{1, 2, 3}}}
	fProxy := func() *fd.AVP {
		return group(avp.ProxyInfo, ident(avp.ProxyHost, "dra.epc.example.org"), octets(avp.ProxyState, 1, 2, 3))
	}
	fRoute := func() *fd.AVP { return ident(avp.RouteRecord, "dra.epc.example.org") }
	gFailed := &base.FailedAvp{Unknown: []diam.RawAVP{{Code: base.AVPOriginStateId, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 9}}}}
	fFailed := func() *fd.AVP { return group(avp.FailedAVP, u32(avp.OriginStateID, 9)) }

	gFeatures := []s6a.SupportedFeatures{{VendorId: tgpp, FeatureListId: 1, FeatureList: 0}}
	fFeatures := func(b perAVP) *fd.AVP {
		return b.g(avp.SupportedFeatures, tgpp, b.a(avp.VendorID, 0, datatype.Unsigned32(tgpp)),
			b.a(avp.FeatureListID, tgpp, datatype.Unsigned32(1)), b.a(avp.FeatureList, tgpp, datatype.Unsigned32(0)))
	}

	const req, pxy = fd.RequestFlag, fd.ProxiableFlag
	return []pair{
		{"RIR",
			&slh.LcsRoutingInfoRequest{
				Header: hdr, SessionId: "gmlc;1;1", VendorSpecificApplicationId: gVSA, AuthSessionState: nsm,
				OriginHost: gmlc, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
				UserName: diam.Some(imsi), Msisdn: diam.Some(msisdn), GmlcNumber: diam.Some(gmlcNumber),
				SupportedFeatures: gFeatures, ProxyInfo: gProxy, RouteRecord: []string{"dra.epc.example.org"},
			},
			fslh(8388622, req|pxy, session("gmlc;1;1"), fVSA(), authState(), ident(avp.OriginHost, gmlc),
				ident(avp.OriginRealm, realm), ident(avp.DestinationHost, hss), ident(avp.DestinationRealm, realm),
				utf8(avp.UserName, imsi), q.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn)),
				q.a(avp.GMLCNumber, tgpp, datatype.OctetString(gmlcNumber)), fFeatures(q), fProxy(), fRoute()),
		},
		{"RIA",
			&slh.LcsRoutingInfoAnswer{
				Header: hdr, SessionId: "gmlc;1;1", VendorSpecificApplicationId: gVSA, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, SupportedFeatures: gFeatures,
				UserName: diam.Some(imsi), Msisdn: diam.Some(msisdn), Lmsi: diam.Some(lmsi),
				ServingNode: &slh.ServingNode{
					SgsnNumber: diam.Some(sgsnNumber), SgsnName: diam.Some(sgsn), SgsnRealm: diam.Some(realm),
					MmeName: diam.Some(mme), MmeRealm: diam.Some(realm), MscNumber: diam.Some(mscNumber),
					TgppAaaServerName: diam.Some(aaa), LcsCapabilitiesSets: diam.Some(uint32(0x10)),
					GmlcAddress: diam.Some(netip.MustParseAddr("2001:db8::10")),
				},
				AdditionalServingNode: []slh.AdditionalServingNode{{
					SgsnNumber: diam.Some(sgsnNumber), MmeName: diam.Some(mme), SgsnName: diam.Some(sgsn),
					SgsnRealm: diam.Some(realm), MmeRealm: diam.Some(realm), MscNumber: diam.Some(mscNumber),
					TgppAaaServerName: diam.Some(aaa), LcsCapabilitiesSets: diam.Some(uint32(0x1f)),
					GmlcAddress: diam.Some(netip.MustParseAddr("192.0.2.11")),
				}},
				GmlcAddress: diam.Some(netip.MustParseAddr("192.0.2.10")), PprAddress: diam.Some(netip.MustParseAddr("192.0.2.20")),
				RiaFlags: diam.Some(uint32(1)), FailedAvp: gFailed, ProxyInfo: gProxy, RouteRecord: []string{"dra.epc.example.org"},
			},
			fslh(8388622, pxy, session("gmlc;1;1"), fVSA(), u32(avp.ResultCode, 2001), authState(),
				ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm), fFeatures(s), utf8(avp.UserName, imsi),
				s.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn)), s.r("LMSI", datatype.OctetString(lmsi)),
				s.g(avp.ServingNode, tgpp, s.a(avp.SGSNNumber, tgpp, datatype.OctetString(sgsnNumber)),
					s.r("SGSN-Name", datatype.DiameterIdentity(sgsn)), s.r("SGSN-Realm", datatype.DiameterIdentity(realm)),
					s.a(avp.MMEName, tgpp, datatype.DiameterIdentity(mme)), s.a(avp.MMERealm, tgpp, datatype.DiameterIdentity(realm)),
					s.r("MSC-Number", datatype.OctetString(mscNumber)), s.a(avp.TGPPAAAServerName, tgpp, datatype.DiameterIdentity(aaa)),
					s.r("LCS-Capabilities-Sets", datatype.Unsigned32(0x10)), s.a(avp.GMLCAddress, tgpp, ip("2001:db8::10"))),
				s.rg("Additional-Serving-Node", s.a(avp.SGSNNumber, tgpp, datatype.OctetString(sgsnNumber)),
					s.a(avp.MMEName, tgpp, datatype.DiameterIdentity(mme)), s.r("SGSN-Name", datatype.DiameterIdentity(sgsn)),
					s.r("SGSN-Realm", datatype.DiameterIdentity(realm)), s.a(avp.MMERealm, tgpp, datatype.DiameterIdentity(realm)),
					s.r("MSC-Number", datatype.OctetString(mscNumber)), s.a(avp.TGPPAAAServerName, tgpp, datatype.DiameterIdentity(aaa)),
					s.r("LCS-Capabilities-Sets", datatype.Unsigned32(0x1f)), s.a(avp.GMLCAddress, tgpp, ip("192.0.2.11"))),
				s.a(avp.GMLCAddress, tgpp, ip("192.0.2.10")), s.r("PPR-Address", ip("192.0.2.20")),
				s.r("RIA-Flags", datatype.Unsigned32(1)), fFailed(), fProxy(), fRoute()),
		},
		// DIAMETER_ERROR_ABSENT_USER (4201, §6.3.4.1): Experimental-Result,
		// no Result-Code, nothing after Origin-Realm.
		{"RIA-absent",
			&slh.LcsRoutingInfoAnswer{
				Header: hdr, SessionId: "gmlc;1;2",
				ExperimentalResult: &base.ExperimentalResult{VendorId: tgpp, ExperimentalResultCode: 4201},
				AuthSessionState:   nsm, OriginHost: hss, OriginRealm: realm,
			},
			fslh(8388622, pxy, session("gmlc;1;2"),
				group(avp.ExperimentalResult, u32(avp.VendorID, tgpp), u32(avp.ExperimentalResultCode, 4201)),
				authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm)),
		},
	}
}

// TestFiorixSLhGaps states what go-diameter v4.6.0 lacks for SLh and keeps
// slhRestated exact: an entry go-diameter gains must move to the
// independent side.
func TestFiorixSLhGaps(t *testing.T) {
	if _, err := dict.Default.App(slhApp); err == nil {
		t.Errorf("go-diameter now has an SLh (%d) dictionary: build the SLh interop on it and drop the per-AVP lookup", slhApp)
	}
	for name, d := range slhRestated {
		if _, def := fiorixHome(d.code, d.vendor); def != nil {
			t.Errorf("go-diameter now defines %s (%d vendor %d) in application %s: remove it from slhRestated", name, d.code, d.vendor, def.App.Name)
		}
	}
}

// go-diameter encodes; go-diam decodes, validates and re-encodes the same
// bytes, and its own message of the same content is the same bytes.
func TestSLhFiorixToGoDiam(t *testing.T) {
	for _, p := range slhPairs() {
		t.Run(p.name, func(t *testing.T) { checkFiorixToGoDiam(t, p) })
	}
}

// go-diam encodes; go-diameter reads the message (it has no SLh dictionary)
// and decodes every AVP in the application that defines it
// (checkGoDiamToFiorixByAVP in cx_test.go).
func TestSLhGoDiamToFiorix(t *testing.T) {
	for _, p := range slhPairs() {
		t.Run(p.name, func(t *testing.T) { checkGoDiamToFiorixByAVP(t, p, slhApp, slhRestated) })
	}
}
