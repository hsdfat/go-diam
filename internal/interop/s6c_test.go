package interop

import (
	"strings"
	"testing"
	"time"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6c"
)

// s6cApp is the S6c application id (TS 29.338 §4.8).
const s6cApp = 16777312

// go-diameter v4.6.0 ships no S6c application: no 16777312 dictionary and
// no SRR/SRA, ALR/ALA or RDR/RDA. Of the S6c AVPs it defines Serving-Node,
// MME-Name and MME-Realm (Ro/Rf), SGSN-Number and MME-Number-for-MT-SMS
// (S6a), MSISDN, Supported-Features and its members, and DRMP. The S6c
// interop below is built on those, as the Cx, Sh, S6t and S6m ones are (see
// cx_test.go).
//
// The AVPs go-diameter has nowhere are listed in s6cRestated with code,
// vendor and M-bit restated from TS 29.338 tables 5.3.3.1/1, 5.3.3.1/2 and
// 6.3.3.1/1 and the defining specifications here, in the test, not read
// from go-diam's schema. Their value encoding is still go-diameter's.
// TestFiorixS6cGaps keeps the list exact.
var s6cRestated = map[string]restated{
	"LMSI":                                    {2400, tgpp, true, "TS 29.173 table 6.4.1/1"},
	"MSC-Number":                              {2403, tgpp, true, "TS 29.173 table 6.4.1/1"},
	"Additional-Serving-Node":                 {2406, tgpp, true, "TS 29.173 table 6.4.1/1"},
	"SGSN-Name":                               {2409, tgpp, false, "TS 29.173 table 6.4.1/1"},
	"SGSN-Realm":                              {2410, tgpp, false, "TS 29.173 table 6.4.1/1"},
	"IP-SM-GW-Number":                         {3100, tgpp, true, "TS 29.336 table 6.4.1/1"},
	"IP-SM-GW-Name":                           {3101, tgpp, true, "TS 29.336 table 6.4.1/1"},
	"User-Identifier":                         {3102, tgpp, true, "TS 29.336 table 6.4.1/1"},
	"External-Identifier":                     {3111, tgpp, true, "TS 29.336 table 6.4.1/1"},
	"IP-SM-GW-Realm":                          {3112, tgpp, true, "TS 29.336 table 6.4.1/1"},
	"SC-Address":                              {3300, tgpp, true, "table 6.3.3.1/1"},
	"SM-RP-MTI":                               {3308, tgpp, true, "table 5.3.3.1/1"},
	"SM-RP-SMEA":                              {3309, tgpp, true, "table 5.3.3.1/1"},
	"SRR-Flags":                               {3310, tgpp, true, "table 5.3.3.1/1"},
	"SM-Delivery-Not-Intended":                {3311, tgpp, true, "table 5.3.3.1/1"},
	"MWD-Status":                              {3312, tgpp, true, "table 5.3.3.1/1"},
	"MME-Absent-User-Diagnostic-SM":           {3313, tgpp, true, "table 5.3.3.1/1"},
	"MSC-Absent-User-Diagnostic-SM":           {3314, tgpp, true, "table 5.3.3.1/1"},
	"SGSN-Absent-User-Diagnostic-SM":          {3315, tgpp, true, "table 5.3.3.1/1"},
	"SM-Delivery-Outcome":                     {3316, tgpp, true, "table 5.3.3.1/1"},
	"MME-SM-Delivery-Outcome":                 {3317, tgpp, true, "table 5.3.3.1/1"},
	"MSC-SM-Delivery-Outcome":                 {3318, tgpp, true, "table 5.3.3.1/1"},
	"SGSN-SM-Delivery-Outcome":                {3319, tgpp, true, "table 5.3.3.1/1"},
	"IP-SM-GW-SM-Delivery-Outcome":            {3320, tgpp, true, "table 5.3.3.1/1"},
	"SM-Delivery-Cause":                       {3321, tgpp, true, "table 5.3.3.1/1"},
	"Absent-User-Diagnostic-SM":               {3322, tgpp, true, "table 5.3.3.1/1"},
	"RDR-Flags":                               {3323, tgpp, false, "table 5.3.3.1/1"},
	"SMSMI-Correlation-ID":                    {3324, tgpp, false, "table 6.3.3.1/1 (code; §6.3.3.13 writes 3308)"},
	"HSS-ID":                                  {3325, tgpp, false, "table 6.3.3.1/1"},
	"Originating-SIP-URI":                     {3326, tgpp, false, "table 6.3.3.1/1"},
	"Destination-SIP-URI":                     {3327, tgpp, false, "table 6.3.3.1/1"},
	"Maximum-UE-Availability-Time":            {3329, tgpp, false, "table 5.3.3.1/1"},
	"SMS-GMSC-Alert-Event":                    {3333, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-3GPP-Absent-User-Diagnostic-SM":     {3334, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-Non-3GPP-Absent-User-Diagnostic-SM": {3335, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-3GPP-SM-Delivery-Outcome":           {3336, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-Non-3GPP-SM-Delivery-Outcome":       {3337, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-3GPP-Number":                        {3338, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-Non-3GPP-Number":                    {3339, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-3GPP-Name":                          {3340, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-Non-3GPP-Name":                      {3341, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-3GPP-Realm":                         {3342, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-Non-3GPP-Realm":                     {3343, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-3GPP-Address":                       {3344, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-Non-3GPP-Address":                   {3345, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-3GPP-SBI-Support-Indicator":         {3346, tgpp, false, "table 5.3.3.1/1"},
	"SMSF-Non-3GPP-SBI-Support-Indicator":     {3347, tgpp, false, "table 5.3.3.1/1"},
	"IP-SM-GW-SBI-Support-Indicator":          {3348, tgpp, false, "table 5.3.3.1/1"},
}

// s6cRequestM: Supported-Features carries the M-bit in requests and not in
// answers (TS 29.338 §4.9, which applies TS 29.229 §7.2.1).
var s6cRequestM = map[[2]uint32]bool{{avp.SupportedFeatures, tgpp}: true}

// s6cM is the S6c M-bit of AVPs go-diameter defines with another one: table
// 5.3.3.1/2 says "Must" for MME-Realm and MME-Number-for-MT-SMS, which
// go-diameter has with the TS 29.173 and TS 29.272 rule (M must not).
// Everything else about these AVPs is go-diameter's. TestFiorixS6cGaps
// keeps the list exact.
var s6cM = map[[2]uint32]bool{
	{avp.MMERealm, tgpp}:          true,
	{avp.MMENumberforMTSMS, tgpp}: true,
}

// s6cAVP is perAVP with the S6c M-bit of s6cM.
type s6cAVP struct{ perAVP }

func (c s6cAVP) a(code, vendor uint32, data datatype.Type) *fd.AVP {
	a := c.perAVP.a(code, vendor, data)
	if m, ok := s6cM[[2]uint32{code, vendor}]; ok {
		a.Flags &^= avp.Mbit
		if m {
			a.Flags |= avp.Mbit
		}
	}
	return a
}

func (c s6cAVP) g(code, vendor uint32, avps ...*fd.AVP) *fd.AVP {
	return c.a(code, vendor, &fd.GroupedAVP{AVP: avps})
}

// fs6c builds a go-diameter message of the S6c application.
func fs6c(cmd uint32, flags uint8, avps ...*fd.AVP) *fd.Message {
	m := fd.NewMessage(cmd, flags, s6cApp, hopByHop, endToEnd, dict.Default)
	for _, a := range avps {
		m.AddAVP(a)
	}
	return m
}

// s6cPairs is one message per S6c command, built with go-diam and with
// go-diameter, covering every grouped AVP a command can carry.
func s6cPairs() []pair {
	const (
		gmsc, hss, realm = "sms-gmsc.example.org", "hss.example.org", "example.org"
		mme              = "mme.example.org"
		imsi             = "001010000000001"
		extID            = "device-1@iot.example.org"
		dra              = "dra.example.org"
	)
	available := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	msisdn := []byte{0x51, 0x55, 0x21, 0x43, 0x65, 0xf7}
	sc := []byte{0x51, 0x55, 0x00, 0x00, 0x10, 0xf0}
	msc := []byte{0x51, 0x55, 0x00, 0x00, 0x20, 0xf0}
	sgsn := []byte{0x51, 0x55, 0x00, 0x00, 0x30, 0xf0}
	mmeNum := []byte{0x51, 0x55, 0x00, 0x00, 0x40, 0xf0}
	ipsm := []byte{0x51, 0x55, 0x00, 0x00, 0x50, 0xf0}
	smsf := []byte{0x51, 0x55, 0x00, 0x00, 0x60, 0xf0}
	lmsi := []byte{0, 0, 0, 0x2a}
	smea := []byte{0x0b, 0x91, 0x51, 0x55}
	nsm := base.AuthSessionStateNoStateMaintained
	q := s6cAVP{perAVP{true, s6cRestated, s6cRequestM}}
	s := s6cAVP{perAVP{false, s6cRestated, s6cRequestM}}

	session := func(v string) *fd.AVP { return fa(avp.SessionID, datatype.UTF8String(v)) }
	authState := func() *fd.AVP { return enum(avp.AuthSessionState, 1) }
	gVSA := &base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(s6cApp))}
	fVSA := func() *fd.AVP {
		return group(avp.VendorSpecificApplicationID, u32(avp.VendorID, tgpp), u32(avp.AuthApplicationID, s6cApp))
	}
	gProxy := []base.ProxyInfo{{ProxyHost: dra, ProxyState: []byte{1, 2, 3}}}
	fProxy := func() *fd.AVP {
		return group(avp.ProxyInfo, ident(avp.ProxyHost, dra), octets(avp.ProxyState, 1, 2, 3))
	}
	fRoute := func() *fd.AVP { return ident(avp.RouteRecord, dra) }
	gFailed := &base.FailedAvp{Unknown: []diam.RawAVP{{Code: base.AVPOriginStateId, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 9}}}}
	fFailed := func() *fd.AVP { return group(avp.FailedAVP, u32(avp.OriginStateID, 9)) }
	// DIAMETER_ERROR_ABSENT_USER (§7.3.3).
	gExp := &base.ExperimentalResult{VendorId: tgpp, ExperimentalResultCode: 5550}
	fExp := func() *fd.AVP {
		return group(avp.ExperimentalResult, u32(avp.VendorID, tgpp), u32(avp.ExperimentalResultCode, 5550))
	}
	gFeatures := []s6c.SupportedFeatures{{VendorId: tgpp, FeatureListId: 1, FeatureList: 1}}
	fFeatures := func(b s6cAVP) *fd.AVP {
		return b.g(avp.SupportedFeatures, tgpp, b.a(avp.VendorID, 0, datatype.Unsigned32(tgpp)),
			b.a(avp.FeatureListID, tgpp, datatype.Unsigned32(1)), b.a(avp.FeatureList, tgpp, datatype.Unsigned32(1)))
	}
	oct := func(b s6cAVP, name string, v []byte) *fd.AVP { return b.r(name, datatype.OctetString(v)) }
	id := func(b s6cAVP, name string, v string) *fd.AVP { return b.r(name, datatype.DiameterIdentity(v)) }
	u := func(b s6cAVP, name string, v uint32) *fd.AVP { return b.r(name, datatype.Unsigned32(v)) }
	en := func(b s6cAVP, name string, v int32) *fd.AVP { return b.r(name, datatype.Enumerated(v)) }

	gCorr := &s6c.SmsmiCorrelationId{HssId: diam.Some("hss-1"), OriginatingSipUri: diam.Some("sip:alice@ims.example.org"),
		DestinationSipUri: diam.Some("sip:bob@ims.example.org")}
	fCorr := func(b s6cAVP) *fd.AVP {
		return b.rg("SMSMI-Correlation-ID", b.r("HSS-ID", datatype.UTF8String("hss-1")),
			b.r("Originating-SIP-URI", datatype.UTF8String("sip:alice@ims.example.org")),
			b.r("Destination-SIP-URI", datatype.UTF8String("sip:bob@ims.example.org")))
	}

	const req, pxy = fd.RequestFlag, fd.ProxiableFlag
	return []pair{
		{"SRR",
			&s6c.SendRoutingInfoForSmRequest{
				Header: hdr, SessionId: "gmsc;1;1", Drmp: diam.Some(s6c.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				AuthSessionState: nsm, OriginHost: gmsc, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
				Msisdn: diam.Some(msisdn), UserName: diam.Some(imsi), SmsmiCorrelationId: gCorr, SupportedFeatures: gFeatures,
				ScAddress: diam.Some(sc), SmRpMti: diam.Some(s6c.SmRpMtiSmDeliver), SmRpSmea: diam.Some(smea),
				SrrFlags: diam.Some(uint32(7)), SmDeliveryNotIntended: diam.Some(s6c.SmDeliveryNotIntendedOnlyImsiRequested),
				ProxyInfo: gProxy, RouteRecord: []string{dra},
			},
			fs6c(8388647, req|pxy, session("gmsc;1;1"), q.a(avp.DRMP, 0, datatype.Enumerated(5)), fVSA(), authState(),
				ident(avp.OriginHost, gmsc), ident(avp.OriginRealm, realm), ident(avp.DestinationHost, hss), ident(avp.DestinationRealm, realm),
				q.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn)), utf8(avp.UserName, imsi), fCorr(q), fFeatures(q),
				oct(q, "SC-Address", sc), en(q, "SM-RP-MTI", 0), oct(q, "SM-RP-SMEA", smea), u(q, "SRR-Flags", 7),
				en(q, "SM-Delivery-Not-Intended", 0), fProxy(), fRoute()),
		},
		{"SRA",
			&s6c.SendRoutingInfoForSmAnswer{
				Header: hdr, SessionId: "gmsc;1;1", Drmp: diam.Some(s6c.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
				UserName: diam.Some(imsi), SupportedFeatures: gFeatures,
				ServingNode: &s6c.ServingNode{
					SgsnName: diam.Some("sgsn.example.org"), SgsnRealm: diam.Some(realm), SgsnNumber: diam.Some(sgsn),
					MmeName: diam.Some(mme), MmeRealm: diam.Some(realm), MmeNumberForMtSms: diam.Some(mmeNum),
					MscNumber: diam.Some(msc), IpSmGwNumber: diam.Some(ipsm), IpSmGwName: diam.Some("ipsmgw.example.org"),
					IpSmGwRealm: diam.Some(realm), IpSmGwSbiSupportIndicator: diam.Some(s6c.IpSmGwSbiSupportIndicatorSupportSbi),
				},
				AdditionalServingNode: &s6c.AdditionalServingNode{
					SgsnName: diam.Some("sgsn2.example.org"), SgsnRealm: diam.Some(realm), SgsnNumber: diam.Some(sgsn),
					MmeName: diam.Some("mme2.example.org"), MmeRealm: diam.Some(realm), MmeNumberForMtSms: diam.Some(mmeNum),
					MscNumber: diam.Some(msc),
				},
				Smsf3GppAddress: &s6c.Smsf3GppAddress{Smsf3GppNumber: diam.Some(smsf), Smsf3GppName: diam.Some("smsf.example.org"),
					Smsf3GppRealm: diam.Some(realm), Smsf3GppSbiSupportIndicator: diam.Some(s6c.Smsf3GppSbiSupportIndicatorSupportSbi)},
				SmsfNon3GppAddress: &s6c.SmsfNon3GppAddress{SmsfNon3GppNumber: diam.Some(smsf), SmsfNon3GppName: diam.Some("smsf-n3.example.org"),
					SmsfNon3GppRealm: diam.Some(realm), SmsfNon3GppSbiSupportIndicator: diam.Some(s6c.SmsfNon3GppSbiSupportIndicatorNotSupportSbi)},
				Lmsi: diam.Some(lmsi),
				UserIdentifier: &s6c.UserIdentifier{UserName: diam.Some(imsi), Msisdn: diam.Some(msisdn),
					ExternalIdentifier: diam.Some(extID), Lmsi: diam.Some(lmsi)},
				MwdStatus: diam.Some(uint32(6)), MmeAbsentUserDiagnosticSm: diam.Some(uint32(1)),
				MscAbsentUserDiagnosticSm: diam.Some(uint32(2)), SgsnAbsentUserDiagnosticSm: diam.Some(uint32(3)),
				Smsf3GppAbsentUserDiagnosticSm: diam.Some(uint32(4)), SmsfNon3GppAbsentUserDiagnosticSm: diam.Some(uint32(5)),
				FailedAvp: gFailed, ProxyInfo: gProxy, RouteRecord: []string{dra},
			},
			fs6c(8388647, pxy, session("gmsc;1;1"), s.a(avp.DRMP, 0, datatype.Enumerated(5)), fVSA(), u32(avp.ResultCode, 2001),
				authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm), utf8(avp.UserName, imsi), fFeatures(s),
				s.g(avp.ServingNode, tgpp, id(s, "SGSN-Name", "sgsn.example.org"), id(s, "SGSN-Realm", realm),
					s.a(avp.SGSNNumber, tgpp, datatype.OctetString(sgsn)), s.a(avp.MMEName, tgpp, datatype.DiameterIdentity(mme)),
					s.a(avp.MMERealm, tgpp, datatype.DiameterIdentity(realm)), s.a(avp.MMENumberforMTSMS, tgpp, datatype.OctetString(mmeNum)),
					oct(s, "MSC-Number", msc), oct(s, "IP-SM-GW-Number", ipsm), id(s, "IP-SM-GW-Name", "ipsmgw.example.org"),
					id(s, "IP-SM-GW-Realm", realm), en(s, "IP-SM-GW-SBI-Support-Indicator", 1)),
				s.rg("Additional-Serving-Node", id(s, "SGSN-Name", "sgsn2.example.org"), id(s, "SGSN-Realm", realm),
					s.a(avp.SGSNNumber, tgpp, datatype.OctetString(sgsn)), s.a(avp.MMEName, tgpp, datatype.DiameterIdentity("mme2.example.org")),
					s.a(avp.MMERealm, tgpp, datatype.DiameterIdentity(realm)), s.a(avp.MMENumberforMTSMS, tgpp, datatype.OctetString(mmeNum)),
					oct(s, "MSC-Number", msc)),
				s.rg("SMSF-3GPP-Address", oct(s, "SMSF-3GPP-Number", smsf), id(s, "SMSF-3GPP-Name", "smsf.example.org"),
					id(s, "SMSF-3GPP-Realm", realm), en(s, "SMSF-3GPP-SBI-Support-Indicator", 1)),
				s.rg("SMSF-Non-3GPP-Address", oct(s, "SMSF-Non-3GPP-Number", smsf), id(s, "SMSF-Non-3GPP-Name", "smsf-n3.example.org"),
					id(s, "SMSF-Non-3GPP-Realm", realm), en(s, "SMSF-Non-3GPP-SBI-Support-Indicator", 0)),
				oct(s, "LMSI", lmsi),
				s.rg("User-Identifier", utf8(avp.UserName, imsi), s.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn)),
					s.r("External-Identifier", datatype.UTF8String(extID)), oct(s, "LMSI", lmsi)),
				u(s, "MWD-Status", 6), u(s, "MME-Absent-User-Diagnostic-SM", 1), u(s, "MSC-Absent-User-Diagnostic-SM", 2),
				u(s, "SGSN-Absent-User-Diagnostic-SM", 3), u(s, "SMSF-3GPP-Absent-User-Diagnostic-SM", 4),
				u(s, "SMSF-Non-3GPP-Absent-User-Diagnostic-SM", 5), fFailed(), fProxy(), fRoute()),
		},
		{"ALR",
			&s6c.AlertServiceCentreRequest{
				Header: hdr, SessionId: "hss;1;2", Drmp: diam.Some(s6c.DrmpPriority3), VendorSpecificApplicationId: gVSA,
				AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, DestinationHost: diam.Some(gmsc), DestinationRealm: realm,
				ScAddress: sc, UserIdentifier: s6c.UserIdentifier{Msisdn: diam.Some(msisdn)}, SmsmiCorrelationId: gCorr,
				MaximumUeAvailabilityTime: diam.Some(available), SmsGmscAlertEvent: diam.Some(uint32(3)),
				ServingNode:       &s6c.ServingNode{MmeName: diam.Some(mme), MmeRealm: diam.Some(realm), MmeNumberForMtSms: diam.Some(mmeNum)},
				SupportedFeatures: gFeatures, ProxyInfo: gProxy, RouteRecord: []string{dra},
			},
			fs6c(8388648, req|pxy, session("hss;1;2"), q.a(avp.DRMP, 0, datatype.Enumerated(3)), fVSA(), authState(),
				ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm), ident(avp.DestinationHost, gmsc), ident(avp.DestinationRealm, realm),
				oct(q, "SC-Address", sc), q.rg("User-Identifier", q.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn))), fCorr(q),
				q.r("Maximum-UE-Availability-Time", datatype.Time(available)), u(q, "SMS-GMSC-Alert-Event", 3),
				q.g(avp.ServingNode, tgpp, q.a(avp.MMEName, tgpp, datatype.DiameterIdentity(mme)),
					q.a(avp.MMERealm, tgpp, datatype.DiameterIdentity(realm)), q.a(avp.MMENumberforMTSMS, tgpp, datatype.OctetString(mmeNum))),
				fFeatures(q), fProxy(), fRoute()),
		},
		{"ALA",
			&s6c.AlertServiceCentreAnswer{
				Header: hdr, SessionId: "hss;1;2", VendorSpecificApplicationId: gVSA, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: gmsc, OriginRealm: realm, SupportedFeatures: gFeatures,
				FailedAvp: gFailed, ProxyInfo: gProxy, RouteRecord: []string{dra},
			},
			fs6c(8388648, pxy, session("hss;1;2"), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, gmsc),
				ident(avp.OriginRealm, realm), fFeatures(s), fFailed(), fProxy(), fRoute()),
		},
		{"RDR",
			&s6c.ReportSmDeliveryStatusRequest{
				Header: hdr, SessionId: "gmsc;1;3", Drmp: diam.Some(s6c.DrmpPriority2), VendorSpecificApplicationId: gVSA,
				AuthSessionState: nsm, OriginHost: gmsc, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
				SupportedFeatures: gFeatures, UserIdentifier: s6c.UserIdentifier{UserName: diam.Some(imsi)}, SmsmiCorrelationId: gCorr,
				ScAddress: sc,
				SmDeliveryOutcome: s6c.SmDeliveryOutcome{
					MmeSmDeliveryOutcome:         &s6c.DeliveryOutcome{SmDeliveryCause: diam.Some(s6c.SmDeliveryCauseAbsentUser), AbsentUserDiagnosticSm: diam.Some(uint32(1))},
					MscSmDeliveryOutcome:         &s6c.DeliveryOutcome{SmDeliveryCause: diam.Some(s6c.SmDeliveryCauseUeMemoryCapacityExceeded)},
					SgsnSmDeliveryOutcome:        &s6c.DeliveryOutcome{SmDeliveryCause: diam.Some(s6c.SmDeliveryCauseSuccessfulTransfer)},
					IpSmGwSmDeliveryOutcome:      &s6c.DeliveryOutcome{AbsentUserDiagnosticSm: diam.Some(uint32(7))},
					Smsf3GppSmDeliveryOutcome:    &s6c.DeliveryOutcome{SmDeliveryCause: diam.Some(s6c.SmDeliveryCauseAbsentUser)},
					SmsfNon3GppSmDeliveryOutcome: &s6c.DeliveryOutcome{SmDeliveryCause: diam.Some(s6c.SmDeliveryCauseAbsentUser), AbsentUserDiagnosticSm: diam.Some(uint32(2))},
				},
				RdrFlags: diam.Some(uint32(1)), ProxyInfo: gProxy, RouteRecord: []string{dra},
			},
			fs6c(8388649, req|pxy, session("gmsc;1;3"), q.a(avp.DRMP, 0, datatype.Enumerated(2)), fVSA(), authState(),
				ident(avp.OriginHost, gmsc), ident(avp.OriginRealm, realm), ident(avp.DestinationHost, hss), ident(avp.DestinationRealm, realm),
				fFeatures(q), q.rg("User-Identifier", utf8(avp.UserName, imsi)), fCorr(q), oct(q, "SC-Address", sc),
				q.rg("SM-Delivery-Outcome",
					q.rg("MME-SM-Delivery-Outcome", en(q, "SM-Delivery-Cause", 1), u(q, "Absent-User-Diagnostic-SM", 1)),
					q.rg("MSC-SM-Delivery-Outcome", en(q, "SM-Delivery-Cause", 0)),
					q.rg("SGSN-SM-Delivery-Outcome", en(q, "SM-Delivery-Cause", 2)),
					q.rg("IP-SM-GW-SM-Delivery-Outcome", u(q, "Absent-User-Diagnostic-SM", 7)),
					q.rg("SMSF-3GPP-SM-Delivery-Outcome", en(q, "SM-Delivery-Cause", 1)),
					q.rg("SMSF-Non-3GPP-SM-Delivery-Outcome", en(q, "SM-Delivery-Cause", 1), u(q, "Absent-User-Diagnostic-SM", 2))),
				u(q, "RDR-Flags", 1), fProxy(), fRoute()),
		},
		{"RDA",
			&s6c.ReportSmDeliveryStatusAnswer{
				Header: hdr, SessionId: "gmsc;1;3", Drmp: diam.Some(s6c.DrmpPriority2), VendorSpecificApplicationId: gVSA,
				ExperimentalResult: gExp, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, SupportedFeatures: gFeatures,
				UserIdentifier: &s6c.UserIdentifier{Msisdn: diam.Some(msisdn)},
				FailedAvp:      gFailed, ProxyInfo: gProxy, RouteRecord: []string{dra},
			},
			fs6c(8388649, pxy, session("gmsc;1;3"), s.a(avp.DRMP, 0, datatype.Enumerated(2)), fVSA(), fExp(), authState(),
				ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm), fFeatures(s),
				s.rg("User-Identifier", s.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn))), fFailed(), fProxy(), fRoute()),
		},
	}
}

// TestFiorixS6cGaps states what go-diameter v4.6.0 lacks for S6c and keeps
// s6cRestated and s6cM exact.
func TestFiorixS6cGaps(t *testing.T) {
	if _, err := dict.Default.App(s6cApp); err == nil {
		t.Errorf("go-diameter now has an S6c (%d) dictionary: build the S6c interop on it and drop the per-AVP lookup", s6cApp)
	}
	for name, d := range s6cRestated {
		if _, def := fiorixHome(d.code, d.vendor); def != nil {
			t.Errorf("go-diameter now defines %s (%d vendor %d) in application %s: remove it from s6cRestated", name, d.code, d.vendor, def.App.Name)
		}
	}
	for k, m := range s6cM {
		_, def := fiorixHome(k[0], k[1])
		if def == nil {
			t.Errorf("AVP %d vendor %d: go-diameter no longer defines it: move it to s6cRestated", k[0], k[1])
			continue
		}
		if strings.Contains(def.Must, "M") == m {
			t.Errorf("AVP %d vendor %d: go-diameter now agrees on the M-bit: remove it from s6cM", k[0], k[1])
		}
	}
}

// go-diameter encodes; go-diam decodes, validates and re-encodes the same
// bytes, and its own message of the same content is the same bytes.
func TestS6cFiorixToGoDiam(t *testing.T) {
	for _, p := range s6cPairs() {
		t.Run(p.name, func(t *testing.T) { checkFiorixToGoDiam(t, p) })
	}
}

// go-diam encodes; go-diameter reads the message (it has no S6c dictionary)
// and decodes every AVP in the application that defines it
// (checkGoDiamToFiorixByAVP in cx_test.go).
func TestS6cGoDiamToFiorix(t *testing.T) {
	for _, p := range s6cPairs() {
		t.Run(p.name, func(t *testing.T) { checkGoDiamToFiorixByAVP(t, p, s6cApp, s6cRestated) })
	}
}
