package interop

import (
	"testing"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6m"
)

// s6mApp is the S6m/S6n application id (TS 29.336 §6.1.8).
const s6mApp = 16777310

// go-diameter v4.6.0 ships no S6m application: no 16777310 dictionary and
// no SIR/SIA. Of the S6m AVPs it defines Serving-Node, MME-Name, MME-Realm,
// Priority-Indication and Application-Port-Identifier (Ro/Rf), SGSN-Number
// and MME-Number-for-MT-SMS (S6a), MSISDN, Supported-Features and its
// members, and the re-used IETF AVPs (DRMP, OC-*, Load). The S6m interop
// below is built on those, as the Cx, Sh and S6t ones are (see cx_test.go).
//
// The AVPs go-diameter has nowhere are listed in s6mRestated with code,
// vendor and M-bit restated from TS 29.336 table 6.4.1/1 and, for the
// re-used ones, the defining specification (table 6.4.1/2 gives no M-bit).
// Their value encoding is still go-diameter's. TestFiorixS6mGaps keeps the
// list exact.
var s6mRestated = map[string]restated{
	"LMSI":                    {2400, tgpp, true, "TS 29.173 table 6.4.1/1"},
	"MSC-Number":              {2403, tgpp, true, "TS 29.173 table 6.4.1/1"},
	"Additional-Serving-Node": {2406, tgpp, true, "TS 29.173 table 6.4.1/1"},
	"SGSN-Name":               {2409, tgpp, false, "TS 29.173 table 6.4.1/1"},
	"SGSN-Realm":              {2410, tgpp, false, "TS 29.173 table 6.4.1/1"},
	"IP-SM-GW-Number":         {3100, tgpp, true, "table 6.4.1/1"},
	"IP-SM-GW-Name":           {3101, tgpp, true, "table 6.4.1/1"},
	"User-Identifier":         {3102, tgpp, true, "table 6.4.1/1"},
	"Service-ID":              {3103, tgpp, true, "table 6.4.1/1"},
	"SCS-Identity":            {3104, tgpp, true, "table 6.4.1/1"},
	"Service-Parameters":      {3105, tgpp, true, "table 6.4.1/1"},
	"T4-Parameters":           {3106, tgpp, true, "table 6.4.1/1"},
	"Service-Data":            {3107, tgpp, true, "table 6.4.1/1"},
	"T4-Data":                 {3108, tgpp, true, "table 6.4.1/1"},
	"HSS-Cause":               {3109, tgpp, true, "table 6.4.1/1"},
	"SIR-Flags":               {3110, tgpp, true, "table 6.4.1/1"},
	"External-Identifier":     {3111, tgpp, true, "table 6.4.1/1"},
	"IP-SM-GW-Realm":          {3112, tgpp, true, "table 6.4.1/1"},
	"SM-RP-SMEA":              {3309, tgpp, true, "TS 29.338 table 5.3.3.1/1"},
}

// s6mRequestM: Supported-Features carries the M-bit in requests and not in
// answers (TS 29.336 §6.1.9, which applies TS 29.229 §7.2.1).
var s6mRequestM = map[[2]uint32]bool{{avp.SupportedFeatures, tgpp}: true}

// fs6m builds a go-diameter message of the S6m application.
func fs6m(cmd uint32, flags uint8, avps ...*fd.AVP) *fd.Message {
	m := fd.NewMessage(cmd, flags, s6mApp, hopByHop, endToEnd, dict.Default)
	for _, a := range avps {
		m.AddAVP(a)
	}
	return m
}

// s6mPairs is SIR and SIA, built with go-diam and with go-diameter, covering
// every grouped AVP and every member of Serving-Node.
func s6mPairs() []pair {
	const (
		iwf, hss, realm = "mtc-iwf.example.org", "hss.example.org", "example.org"
		imsi            = "001010000000001"
		extID           = "device-1@iot.example.org"
		dra             = "dra.example.org"
	)
	msisdn := []byte{0x51, 0x55, 0x21, 0x43, 0x65, 0xf7}
	scs := []byte{0x51, 0x55, 0x00, 0x00, 0x10, 0xf0}
	msc := []byte{0x51, 0x55, 0x00, 0x00, 0x20, 0xf0}
	sgsn := []byte{0x51, 0x55, 0x00, 0x00, 0x30, 0xf0}
	mmeNum := []byte{0x51, 0x55, 0x00, 0x00, 0x40, 0xf0}
	ipsm := []byte{0x51, 0x55, 0x00, 0x00, 0x50, 0xf0}
	lmsi := []byte{0, 0, 0, 0x2a}
	smea := []byte{0x0b, 0x91, 0x51, 0x55}
	nsm := base.AuthSessionStateNoStateMaintained
	q, s := perAVP{true, s6mRestated, s6mRequestM}, perAVP{false, s6mRestated, s6mRequestM}

	session := func(v string) *fd.AVP { return fa(avp.SessionID, datatype.UTF8String(v)) }
	authState := func() *fd.AVP { return enum(avp.AuthSessionState, 1) }
	gVSA := &base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(s6mApp))}
	fVSA := func() *fd.AVP {
		return group(avp.VendorSpecificApplicationID, u32(avp.VendorID, tgpp), u32(avp.AuthApplicationID, s6mApp))
	}
	gProxy := []base.ProxyInfo{{ProxyHost: dra, ProxyState: []byte{1, 2, 3}}}
	fProxy := func() *fd.AVP {
		return group(avp.ProxyInfo, ident(avp.ProxyHost, dra), octets(avp.ProxyState, 1, 2, 3))
	}
	fRoute := func() *fd.AVP { return ident(avp.RouteRecord, dra) }
	gFailed := &base.FailedAvp{Unknown: []diam.RawAVP{{Code: base.AVPOriginStateId, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 9}}}}
	fFailed := func() *fd.AVP { return group(avp.FailedAVP, u32(avp.OriginStateID, 9)) }
	gFeatures := []s6m.SupportedFeatures{{VendorId: tgpp, FeatureListId: 1, FeatureList: 0}}
	fFeatures := func(b perAVP) *fd.AVP {
		return b.g(avp.SupportedFeatures, tgpp, b.a(avp.VendorID, 0, datatype.Unsigned32(tgpp)),
			b.a(avp.FeatureListID, tgpp, datatype.Unsigned32(1)), b.a(avp.FeatureList, tgpp, datatype.Unsigned32(0)))
	}
	gOcSF := &s6m.OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))}
	fOcSF := func() *fd.AVP {
		return s.g(avp.OCSupportedFeatures, 0, s.a(avp.OCFeatureVector, 0, datatype.Unsigned64(1)))
	}
	gOlr := &s6m.OcOlr{OcSequenceNumber: 9, OcReportType: s6m.OcReportTypeRealmReport,
		OcReductionPercentage: diam.Some(uint32(50)), OcValidityDuration: diam.Some(uint32(30))}
	fOlr := func() *fd.AVP {
		return s.g(avp.OCOLR, 0, s.a(avp.OCSequenceNumber, 0, datatype.Unsigned64(9)), s.a(avp.OCReportType, 0, datatype.Enumerated(1)),
			s.a(avp.OCReductionPercentage, 0, datatype.Unsigned32(50)), s.a(avp.OCValidityDuration, 0, datatype.Unsigned32(30)))
	}
	// Load-Value is left out: go-diameter types it Unsigned32, RFC 8583
	// §7.3 says Unsigned64 (see dictcheck-allow.txt).
	gLoad := []s6m.Load{{LoadType: diam.Some(s6m.LoadTypeHost), Sourceid: diam.Some(hss)}}
	fLoad := func() *fd.AVP {
		return s.g(avp.Load, 0, s.a(avp.LoadType, 0, datatype.Enumerated(0)), s.a(avp.SourceID, 0, datatype.DiameterIdentity(hss)))
	}
	oct := func(b perAVP, name string, v []byte) *fd.AVP { return b.r(name, datatype.OctetString(v)) }
	id := func(b perAVP, name string, v string) *fd.AVP { return b.r(name, datatype.DiameterIdentity(v)) }

	const req, pxy = fd.RequestFlag, fd.ProxiableFlag
	return []pair{
		{"SIR",
			&s6m.SubscriberInformationRequest{
				Header: hdr, SessionId: "iwf;1;1", Drmp: diam.Some(s6m.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				AuthSessionState: nsm, OriginHost: iwf, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
				UserIdentifier: s6m.UserIdentifier{ExternalIdentifier: diam.Some(extID)},
				ServiceId:      diam.Some(s6m.ServiceIdDeviceTrigger), ScsIdentity: diam.Some(scs),
				ServiceParameters: &s6m.ServiceParameters{
					T4Parameters:              &s6m.T4Parameters{PriorityIndication: diam.Some(s6m.PriorityIndicationPriority), SmRpSmea: diam.Some(smea)},
					ApplicationPortIdentifier: diam.Some(uint32(9200)),
				},
				SirFlags: 1, OcSupportedFeatures: gOcSF, SupportedFeatures: gFeatures, ProxyInfo: gProxy, RouteRecord: []string{dra},
			},
			fs6m(8388641, req|pxy, session("iwf;1;1"), q.a(avp.DRMP, 0, datatype.Enumerated(5)), fVSA(), authState(),
				ident(avp.OriginHost, iwf), ident(avp.OriginRealm, realm), ident(avp.DestinationHost, hss), ident(avp.DestinationRealm, realm),
				q.rg("User-Identifier", q.r("External-Identifier", datatype.UTF8String(extID))),
				q.r("Service-ID", datatype.Enumerated(0)), oct(q, "SCS-Identity", scs),
				q.rg("Service-Parameters",
					q.rg("T4-Parameters", q.a(avp.PriorityIndication, tgpp, datatype.Enumerated(1)), oct(q, "SM-RP-SMEA", smea)),
					q.a(avp.ApplicationPortIdentifer, tgpp, datatype.Unsigned32(9200))),
				q.r("SIR-Flags", datatype.Unsigned32(1)), fOcSF(), fFeatures(q), fProxy(), fRoute()),
		},
		{"SIA",
			&s6m.SubscriberInformationAnswer{
				Header: hdr, SessionId: "iwf;1;1", Drmp: diam.Some(s6m.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
				OcSupportedFeatures: gOcSF, OcOlr: gOlr, Load: gLoad, SupportedFeatures: gFeatures,
				UserIdentifier: []s6m.UserIdentifier{
					{UserName: diam.Some(imsi), Msisdn: diam.Some(msisdn), ExternalIdentifier: diam.Some(extID), Lmsi: diam.Some(lmsi)},
				},
				ServiceData: &s6m.ServiceData{T4Data: &s6m.T4Data{
					HssCause: diam.Some(uint32(0)),
					ServingNode: &s6m.ServingNode{
						SgsnName: diam.Some("sgsn.example.org"), SgsnRealm: diam.Some(realm), SgsnNumber: diam.Some(sgsn),
						MmeName: diam.Some("mme.example.org"), MmeRealm: diam.Some(realm), MmeNumberForMtSms: diam.Some(mmeNum),
						MscNumber: diam.Some(msc), IpSmGwNumber: diam.Some(ipsm), IpSmGwName: diam.Some("ipsmgw.example.org"),
						IpSmGwRealm: diam.Some(realm),
					},
					AdditionalServingNode: []s6m.AdditionalServingNode{{
						SgsnName: diam.Some("sgsn2.example.org"), SgsnRealm: diam.Some(realm), SgsnNumber: diam.Some(sgsn),
						MmeName: diam.Some("mme2.example.org"), MmeRealm: diam.Some(realm), MmeNumberForMtSms: diam.Some(mmeNum),
						MscNumber: diam.Some(msc),
					}},
				}},
				FailedAvp: gFailed, ProxyInfo: gProxy, RouteRecord: []string{dra},
			},
			fs6m(8388641, pxy, session("iwf;1;1"), s.a(avp.DRMP, 0, datatype.Enumerated(5)), fVSA(), u32(avp.ResultCode, 2001),
				authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm), fOcSF(), fOlr(), fLoad(), fFeatures(s),
				s.rg("User-Identifier", utf8(avp.UserName, imsi), s.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn)),
					s.r("External-Identifier", datatype.UTF8String(extID)), oct(s, "LMSI", lmsi)),
				s.rg("Service-Data", s.rg("T4-Data", s.r("HSS-Cause", datatype.Unsigned32(0)),
					s.g(avp.ServingNode, tgpp, id(s, "SGSN-Name", "sgsn.example.org"), id(s, "SGSN-Realm", realm),
						s.a(avp.SGSNNumber, tgpp, datatype.OctetString(sgsn)), s.a(avp.MMEName, tgpp, datatype.DiameterIdentity("mme.example.org")),
						s.a(avp.MMERealm, tgpp, datatype.DiameterIdentity(realm)), s.a(avp.MMENumberforMTSMS, tgpp, datatype.OctetString(mmeNum)),
						oct(s, "MSC-Number", msc), oct(s, "IP-SM-GW-Number", ipsm), id(s, "IP-SM-GW-Name", "ipsmgw.example.org"),
						id(s, "IP-SM-GW-Realm", realm)),
					s.rg("Additional-Serving-Node", id(s, "SGSN-Name", "sgsn2.example.org"), id(s, "SGSN-Realm", realm),
						s.a(avp.SGSNNumber, tgpp, datatype.OctetString(sgsn)), s.a(avp.MMEName, tgpp, datatype.DiameterIdentity("mme2.example.org")),
						s.a(avp.MMERealm, tgpp, datatype.DiameterIdentity(realm)), s.a(avp.MMENumberforMTSMS, tgpp, datatype.OctetString(mmeNum)),
						oct(s, "MSC-Number", msc)))),
				fFailed(), fProxy(), fRoute()),
		},
	}
}

// TestFiorixS6mGaps states what go-diameter v4.6.0 lacks for S6m and keeps
// s6mRestated exact: an entry go-diameter gains must move to the
// independent side.
func TestFiorixS6mGaps(t *testing.T) {
	if _, err := dict.Default.App(s6mApp); err == nil {
		t.Errorf("go-diameter now has an S6m (%d) dictionary: build the S6m interop on it and drop the per-AVP lookup", s6mApp)
	}
	for name, d := range s6mRestated {
		if _, def := fiorixHome(d.code, d.vendor); def != nil {
			t.Errorf("go-diameter now defines %s (%d vendor %d) in application %s: remove it from s6mRestated", name, d.code, d.vendor, def.App.Name)
		}
	}
}

// go-diameter encodes; go-diam decodes, validates and re-encodes the same
// bytes, and its own message of the same content is the same bytes.
func TestS6mFiorixToGoDiam(t *testing.T) {
	for _, p := range s6mPairs() {
		t.Run(p.name, func(t *testing.T) { checkFiorixToGoDiam(t, p) })
	}
}

// go-diam encodes; go-diameter reads the message (it has no S6m dictionary)
// and decodes every AVP in the application that defines it
// (checkGoDiamToFiorixByAVP in cx_test.go).
func TestS6mGoDiamToFiorix(t *testing.T) {
	for _, p := range s6mPairs() {
		t.Run(p.name, func(t *testing.T) { checkGoDiamToFiorixByAVP(t, p, s6mApp, s6mRestated) })
	}
}
