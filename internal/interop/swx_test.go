package interop

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/cx"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6a"
	"github.com/hsdfat/go-diam/swx"
)

// swxApp is the SWx application id (TS 29.273 §8.2.1).
const swxApp = 16777265

// go-diameter v4.6.0 has an SWx application (16777265, its tgpp_swx.xml,
// written from TS 29.273 V9.2.0) with MAR/MAA, SAR/SAA and RTR/RTA, but no
// command 305 (PPR/PPA): it still reads a 305 message of the application,
// since it decodes AVPs by application, not by command. So SWx is tested
// like S6a: every AVP is built from go-diameter's SWx dictionary (with the
// inherited base protocol), and go-diameter decodes go-diam's messages with
// it. The SWx AVPs it has only under other applications are in
// fiorixSwxGaps, and those it has nowhere in swxRestated.
//
// Its SWx grammars are the Release 9 ones (no DRMP in MAR, SAR and their
// answers, SIP-Auth-Data-Item optional in MAR, Destination-Host before
// Destination-Realm in MAR, Destination-Host optional in RTR); go-diameter
// does not enforce a grammar on read, and the samples follow TS 29.273
// V18.6.0, so none of this changes the bytes.

// fiorixSwxGaps are AVPs of the SWx samples that go-diameter defines, but
// not in its SWx application, so it decodes them as Unknown inside an SWx
// message. The samples build them from go-diameter's own definition in the
// application that has them. TestFiorixSwxGaps keeps the list exact.
var fiorixSwxGaps = map[[2]uint32]string{
	{629, tgpp}:  "Feature-List-ID",
	{630, tgpp}:  "Feature-List",
	{1402, tgpp}: "IMEI",
	{1403, tgpp}: "Software-Version",
	{1503, tgpp}: "AN-Trusted",
	{1524, tgpp}: "SSID",
	{1612, tgpp}: "Active-APN",
	{1616, tgpp}: "MPS-Priority",
	{2716, tgpp}: "BSSID",
	{334, 0}:     "MIP-Home-Agent-Address",
	{649, 0}:     "SourceID",
	{650, 0}:     "Load",
	{651, 0}:     "Load-Type",
}

// swxRestated are the AVPs of the SWx samples that go-diameter defines in
// no application. Their code, vendor and M-bit are restated here from the
// specification, not read from go-diam's schema; their value encoding is
// still go-diameter's. TestFiorixSwxGaps keeps the list exact.
var swxRestated = map[string]restated{
	"PPR-Flags":                   {1508, tgpp, false, "TS 29.273 table 8.2.3.0/1"},
	"WLAN-Identifier":             {1509, tgpp, false, "TS 29.273 table 8.2.3.0/1"},
	"TWAN-Access-Info":            {1510, tgpp, false, "TS 29.273 table 8.2.3.0/1"},
	"Access-Authorization-Flags":  {1511, tgpp, false, "TS 29.273 table 8.2.3.0/1"},
	"TWAN-Default-APN-Context-Id": {1512, tgpp, false, "TS 29.273 table 8.2.3.0/1"},
	"AAA-Failure-Indication":      {1518, tgpp, false, "TS 29.273 table 8.2.3.0/1"},
	"HESSID":                      {1525, tgpp, false, "TS 29.273 table 5.2.3.1/1"},
	"Access-Network-Info":         {1526, tgpp, false, "TS 29.273 table 5.2.3.1/1"},
	"Emergency-Services":          {1538, tgpp, false, "TS 29.273 table 5.2.3.1/1"},
	"ERP-Authorization":           {1541, tgpp, false, "TS 29.273 table 8.2.3.0/1"},
	"Time-Zone":                   {1642, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Local-Time-Zone":             {1649, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Daylight-Saving-Time":        {1650, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"UE-Usage-Type":               {1680, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Emergency-Info":              {1687, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Core-Network-Restrictions":   {1704, tgpp, false, "TS 29.272 table 7.3.1/1"},
	"Operator-Name":               {126, 0, false, "RFC 5580 §6"},
	"Location-Information":        {127, 0, false, "RFC 5580 §6"},
	"Location-Data":               {128, 0, false, "RFC 5580 §6"},
	"Logical-Access-ID":           {302, 13019, false, "ETSI ES 283 034 table 10"},
}

// swxM is the SWx M-bit of the AVPs whose flag rule in go-diameter's
// dictionary is not the SWx one. Everything else about these AVPs (code,
// vendor, type, encoding) still comes from go-diameter.
var swxM = map[[2]uint32]bool{
	// TS 29.273 table 8.2.3.0/2 leaves RAT-Type to TS 29.212 (table
	// 5.3.0.1: M should not). go-diameter's SWx dictionary has M,V, the
	// STa rule of table 5.2.3.1/1; its other definitions have V only, which
	// is what dictcheck compares.
	{avp.RATType, tgpp}: false,
	// Table 8.2.3.0/1: AN-Trusted is V, M must not; go-diameter has M,V
	// (dictcheck-allow.txt).
	{1503, tgpp}: false,
	// TS 29.272 table 7.3.1/1: Active-APN is V, M must not; go-diameter has
	// M,V (dictcheck-allow.txt).
	{avp.ActiveAPN, tgpp}: false,
}

// swxRequestM: Supported-Features carries the M-bit in requests and not in
// answers (TS 29.229 §7.2.1, applied to SWx through table 8.2.3.0/2).
var swxRequestM = map[[2]uint32]bool{{avp.SupportedFeatures, tgpp}: true}

// swxBuilder builds the go-diameter AVPs of one SWx message.
type swxBuilder struct{ request bool }

// a builds an AVP that go-diameter defines: code from its constants, flags
// from its SWx dictionary or, for fiorixSwxGaps, from the application that
// has it; swxM and swxRequestM override the M-bit where noted.
func (b swxBuilder) a(code, vendor uint32, data datatype.Type) *fd.AVP {
	k := [2]uint32{code, vendor}
	def, err := dict.Default.FindAVPByCode(swxApp, code, vendor)
	if err != nil {
		if _, gap := fiorixSwxGaps[k]; !gap {
			panic(err)
		}
		if def = fiorixAVP(code, vendor); def == nil {
			panic(fmt.Sprintf("AVP %d vendor %d is in no go-diameter application", code, vendor))
		}
	}
	var flags uint8
	if strings.Contains(def.Must, "M") {
		flags = avp.Mbit
	}
	if m, ok := swxM[k]; ok {
		flags = 0
		if m {
			flags = avp.Mbit
		}
	}
	if m, ok := swxRequestM[k]; ok && b.request {
		flags = 0
		if m {
			flags = avp.Mbit
		}
	}
	return fd.NewAVP(code, flags, vendor, data)
}

func (b swxBuilder) g(code, vendor uint32, avps ...*fd.AVP) *fd.AVP {
	return b.a(code, vendor, &fd.GroupedAVP{AVP: avps})
}

// r builds an AVP that go-diameter does not define, from swxRestated.
func (b swxBuilder) r(name string, data datatype.Type) *fd.AVP {
	return perAVP{restated: swxRestated}.r(name, data)
}

func (b swxBuilder) rg(name string, avps ...*fd.AVP) *fd.AVP {
	return b.r(name, &fd.GroupedAVP{AVP: avps})
}

// fswx builds a go-diameter message of the SWx application.
func fswx(cmd uint32, flags uint8, avps ...*fd.AVP) *fd.Message {
	m := fd.NewMessage(cmd, flags, swxApp, hopByHop, endToEnd, dict.Default)
	for _, a := range avps {
		m.AddAVP(a)
	}
	return m
}

// swxPairs is one message per SWx command, built with go-diam and with
// go-diameter, carrying every AVP of the SWx grammars and grouped AVPs.
// Left out: Load-Value (go-diameter types it Unsigned32, RFC 8583 §7.3 says
// Unsigned64), and Visited-Network-Identifier inside the S6a grouped AVPs,
// whose M-bit follows S6a (see swx.proto).
func swxPairs() []pair {
	const (
		aaa, hss, realm = "aaa.epc.example.org", "hss.epc.example.org", "epc.example.org"
		nai             = "0001010000000001@wlan.mnc001.mcc001.3gppnetwork.org"
		imei            = "353490069873319"
		apn             = "internet"
		vni             = "mnc002.mcc001.3gppnetwork.org"
	)
	nsm := base.AuthSessionStateNoStateMaintained
	q, s := swxBuilder{true}, swxBuilder{false}

	session := func(v string) *fd.AVP { return fa(avp.SessionID, datatype.UTF8String(v)) }
	authState := func() *fd.AVP { return enum(avp.AuthSessionState, 1) }
	drmp := func() *fd.AVP { return s.a(avp.DRMP, 0, datatype.Enumerated(5)) }
	gVSA := base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(swxApp))}
	fVSA := func() *fd.AVP {
		return group(avp.VendorSpecificApplicationID, u32(avp.VendorID, tgpp), u32(avp.AuthApplicationID, swxApp))
	}
	// DIAMETER_ERROR_USER_UNKNOWN (TS 29.229 §6.2.2.1).
	gExp := &base.ExperimentalResult{VendorId: tgpp, ExperimentalResultCode: 5001}
	fExp := func() *fd.AVP {
		return group(avp.ExperimentalResult, u32(avp.VendorID, tgpp), u32(avp.ExperimentalResultCode, 5001))
	}

	gFeatures := []cx.SupportedFeatures{{VendorId: tgpp, FeatureListId: 1, FeatureList: 0x7f}}
	fFeatures := func(b swxBuilder) *fd.AVP {
		return b.g(avp.SupportedFeatures, tgpp, b.a(avp.VendorID, 0, datatype.Unsigned32(tgpp)),
			b.a(avp.FeatureListID, tgpp, datatype.Unsigned32(1)), b.a(avp.FeatureList, tgpp, datatype.Unsigned32(0x7f)))
	}
	gOcSF := &cx.OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))}
	fOcSF := func() *fd.AVP {
		return s.g(avp.OCSupportedFeatures, 0, s.a(avp.OCFeatureVector, 0, datatype.Unsigned64(1)))
	}
	gOlr := &cx.OcOlr{OcSequenceNumber: 9, OcReportType: cx.OcReportTypeRealmReport,
		OcReductionPercentage: diam.Some(uint32(50)), OcValidityDuration: diam.Some(uint32(30))}
	fOlr := func() *fd.AVP {
		return s.g(avp.OCOLR, 0, s.a(avp.OCSequenceNumber, 0, datatype.Unsigned64(9)), s.a(avp.OCReportType, 0, datatype.Enumerated(1)),
			s.a(avp.OCReductionPercentage, 0, datatype.Unsigned32(50)), s.a(avp.OCValidityDuration, 0, datatype.Unsigned32(30)))
	}
	gLoad := []cx.Load{{LoadType: diam.Some(cx.LoadTypeHost), Sourceid: diam.Some(hss)}}
	fLoad := func() *fd.AVP {
		return s.g(avp.Load, 0, s.a(avp.LoadType, 0, datatype.Enumerated(0)), s.a(avp.SourceID, 0, datatype.DiameterIdentity(hss)))
	}
	gTerminal := &s6a.TerminalInformation{Imei: diam.Some(imei), SoftwareVersion: diam.Some("03")}
	fTerminal := func() *fd.AVP {
		return s.g(avp.TerminalInformation, tgpp, s.a(avp.IMEI, tgpp, datatype.UTF8String(imei)), s.a(avp.SoftwareVersion, tgpp, datatype.UTF8String("03")))
	}
	gPgw := func() *s6a.Mip6AgentInfo {
		return &s6a.Mip6AgentInfo{MipHomeAgentAddress: []netip.Addr{netip.MustParseAddr("192.0.2.10")}}
	}
	fPgw := func() *fd.AVP { return s.g(avp.MIP6AgentInfo, 0, s.a(avp.MIPHomeAgentAddress, 0, ip("192.0.2.10"))) }

	ck, ik, autn, xres := make([]byte, 16), make([]byte, 16), make([]byte, 32), make([]byte, 16)
	for i := range ck {
		ck[i], ik[i], xres[i] = byte(i), byte(0x10+i), byte(0x20+i)
	}
	for i := range autn {
		autn[i] = byte(0x40 + i)
	}
	gVector := swx.SipAuthDataItem{
		SipItemNumber: diam.Some(uint32(1)), SipAuthenticationScheme: diam.Some("EAP-AKA'"), SipAuthenticate: diam.Some(autn),
		SipAuthorization: diam.Some(xres), ConfidentialityKey: diam.Some(ck), IntegrityKey: diam.Some(ik),
	}
	fVector := func() *fd.AVP {
		return s.g(avp.SIPAuthDataItem, tgpp, s.a(avp.SIPItemNumber, tgpp, datatype.Unsigned32(1)),
			s.a(avp.SIPAuthenticationScheme, tgpp, datatype.UTF8String("EAP-AKA'")), s.a(avp.SIPAuthenticate, tgpp, datatype.OctetString(autn)),
			s.a(avp.SIPAuthorization, tgpp, datatype.OctetString(xres)), s.a(avp.ConfidentialityKey, tgpp, datatype.OctetString(ck)),
			s.a(avp.IntegrityKey, tgpp, datatype.OctetString(ik)))
	}

	traceRef := []byte{0x00, 0xf1, 0x10, 0x00, 0x00, 0x01}
	gTrace := &s6a.TraceData{TraceReference: traceRef, TraceDepth: 1, TraceNeTypeList: []byte{0x08}, TraceEventList: []byte{0x01},
		TraceCollectionEntity: netip.MustParseAddr("192.0.2.20")}
	fTrace := func() *fd.AVP {
		return s.g(avp.TraceData, tgpp, s.a(avp.TraceReference, tgpp, datatype.OctetString(traceRef)), s.a(avp.TraceDepth, tgpp, datatype.Enumerated(1)),
			s.a(avp.TraceNETypeList, tgpp, datatype.OctetString([]byte{0x08})), s.a(avp.TraceEventList, tgpp, datatype.OctetString([]byte{0x01})),
			s.a(avp.TraceCollectionEntity, tgpp, ip("192.0.2.20")))
	}
	gApn := s6a.ApnConfiguration{
		ContextIdentifier: 1, PdnType: s6a.PdnTypeIpv4v6, ServiceSelection: apn,
		EpsSubscribedQosProfile: &s6a.EpsSubscribedQosProfile{QosClassIdentifier: s6a.QosClassIdentifierQci9,
			AllocationRetentionPriority: s6a.AllocationRetentionPriority{PriorityLevel: 8}},
		Ambr: &s6a.Ambr{MaxRequestedBandwidthUl: 50_000_000, MaxRequestedBandwidthDl: 100_000_000},
	}
	fApn := func() *fd.AVP {
		return s.g(avp.APNConfiguration, tgpp, s.a(avp.ContextIdentifier, tgpp, datatype.Unsigned32(1)), s.a(avp.PDNType, tgpp, datatype.Enumerated(2)),
			s.a(avp.ServiceSelection, 0, datatype.UTF8String(apn)),
			s.g(avp.EPSSubscribedQoSProfile, tgpp, s.a(avp.QoSClassIdentifier, tgpp, datatype.Enumerated(9)),
				s.g(avp.AllocationRetentionPriority, tgpp, s.a(avp.PriorityLevel, tgpp, datatype.Unsigned32(8)))),
			s.g(avp.AMBR, tgpp, s.a(avp.MaxRequestedBandwidthUL, tgpp, datatype.Unsigned32(50_000_000)),
				s.a(avp.MaxRequestedBandwidthDL, tgpp, datatype.Unsigned32(100_000_000))))
	}

	gProfile := &swx.Non3GppUserData{
		SubscriptionId:              &swx.SubscriptionId{SubscriptionIdType: diam.Some(swx.SubscriptionIdTypeEndUserE164), SubscriptionIdData: diam.Some("15551234567")},
		Non3GppIpAccess:             diam.Some(swx.Non3GppIpAccessNon3gppSubscriptionAllowed),
		Non3GppIpAccessApn:          diam.Some(swx.Non3GppIpAccessApnNon3gppApnsEnable),
		RatType:                     []s6a.RatType{s6a.RatTypeVirtual, s6a.RatTypeEutran},
		SessionTimeout:              diam.Some(uint32(3600)),
		Mip6FeatureVector:           diam.Some(uint64(0x0000400000000000)),
		Ambr:                        &s6a.Ambr{MaxRequestedBandwidthUl: 100_000_000, MaxRequestedBandwidthDl: 200_000_000},
		TgppChargingCharacteristics: diam.Some("0800"),
		ContextIdentifier:           diam.Some(uint32(1)),
		ApnOiReplacement:            diam.Some("mnc001.mcc001.gprs"),
		ApnConfiguration:            []s6a.ApnConfiguration{gApn},
		TraceInfo:                   &swx.TraceInfo{TraceData: gTrace},
		TwanDefaultApnContextId:     diam.Some(uint32(1)),
		TwanAccessInfo: []swx.TwanAccessInfo{{AccessAuthorizationFlags: diam.Some(uint32(3)),
			WlanIdentifier: &swx.WlanIdentifier{Ssid: diam.Some("operator-wifi"), Hessid: diam.Some("00-10-A4-23-19-C0")}}},
		UeUsageType:             diam.Some(uint32(7)),
		EmergencyInfo:           &s6a.EmergencyInfo{Mip6AgentInfo: gPgw()},
		ErpAuthorization:        diam.Some(uint32(1)),
		CoreNetworkRestrictions: diam.Some(uint32(0)),
		MpsPriority:             diam.Some(uint32(1)),
	}
	fProfile := func() *fd.AVP {
		return s.g(avp.Non3GPPUserData, tgpp,
			s.g(avp.SubscriptionID, 0, s.a(avp.SubscriptionIDType, 0, datatype.Enumerated(0)), s.a(avp.SubscriptionIDData, 0, datatype.UTF8String("15551234567"))),
			s.a(avp.Non3GPPIPAccess, tgpp, datatype.Enumerated(0)), s.a(avp.Non3GPPIPAccessAPN, tgpp, datatype.Enumerated(0)),
			s.a(avp.RATType, tgpp, datatype.Enumerated(1)), s.a(avp.RATType, tgpp, datatype.Enumerated(1004)),
			s.a(avp.SessionTimeout, 0, datatype.Unsigned32(3600)), s.a(avp.MIP6FeatureVector, 0, datatype.Unsigned64(0x0000400000000000)),
			s.g(avp.AMBR, tgpp, s.a(avp.MaxRequestedBandwidthUL, tgpp, datatype.Unsigned32(100_000_000)),
				s.a(avp.MaxRequestedBandwidthDL, tgpp, datatype.Unsigned32(200_000_000))),
			s.a(avp.TGPPChargingCharacteristics, tgpp, datatype.UTF8String("0800")), s.a(avp.ContextIdentifier, tgpp, datatype.Unsigned32(1)),
			s.a(avp.APNOIReplacement, tgpp, datatype.UTF8String("mnc001.mcc001.gprs")), fApn(),
			s.g(avp.TraceInfo, tgpp, fTrace()),
			s.r("TWAN-Default-APN-Context-Id", datatype.Unsigned32(1)),
			s.rg("TWAN-Access-Info", s.r("Access-Authorization-Flags", datatype.Unsigned32(3)),
				s.rg("WLAN-Identifier", s.a(avp.SSID, tgpp, datatype.UTF8String("operator-wifi")), s.r("HESSID", datatype.UTF8String("00-10-A4-23-19-C0")))),
			s.r("UE-Usage-Type", datatype.Unsigned32(7)), s.rg("Emergency-Info", fPgw()), s.r("ERP-Authorization", datatype.Unsigned32(1)),
			s.r("Core-Network-Restrictions", datatype.Unsigned32(0)), s.a(avp.MPSPriority, tgpp, datatype.Unsigned32(1)))
	}

	const req, pxy = fd.RequestFlag, fd.ProxiableFlag
	return []pair{
		{"MAR",
			&swx.MultimediaAuthRequest{
				Header: hdr, SessionId: "aaa;1;1", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: gVSA, AuthSessionState: nsm,
				OriginHost: aaa, OriginRealm: realm, DestinationRealm: realm, DestinationHost: diam.Some(hss), UserName: nai,
				RatType: diam.Some(s6a.RatTypeWlan), AnTrusted: diam.Some(swx.AnTrustedUntrusted), Anid: diam.Some("WLAN"),
				VisitedNetworkIdentifier: diam.Some([]byte(vni)), TerminalInformation: gTerminal,
				SipAuthDataItem: swx.SipAuthDataItem{SipAuthenticationScheme: diam.Some("EAP-AKA'")}, SipNumberAuthItems: 1,
				AaaFailureIndication: diam.Some(uint32(1)), OcSupportedFeatures: gOcSF, SupportedFeatures: gFeatures,
			},
			fswx(303, req|pxy, session("aaa;1;1"), drmp(), fVSA(), authState(), ident(avp.OriginHost, aaa), ident(avp.OriginRealm, realm),
				ident(avp.DestinationRealm, realm), ident(avp.DestinationHost, hss), utf8(avp.UserName, nai),
				q.a(avp.RATType, tgpp, datatype.Enumerated(0)), q.a(1503, tgpp, datatype.Enumerated(1)), q.a(avp.ANID, tgpp, datatype.UTF8String("WLAN")),
				q.a(avp.VisitedNetworkIdentifier, tgpp, datatype.OctetString(vni)), fTerminal(),
				q.g(avp.SIPAuthDataItem, tgpp, q.a(avp.SIPAuthenticationScheme, tgpp, datatype.UTF8String("EAP-AKA'"))),
				q.a(avp.SIPNumberAuthItems, tgpp, datatype.Unsigned32(1)), q.r("AAA-Failure-Indication", datatype.Unsigned32(1)),
				fOcSF(), fFeatures(q)),
		},
		{"MAA",
			&swx.MultimediaAuthAnswer{
				Header: hdr, SessionId: "aaa;1;1", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, UserName: nai,
				SipNumberAuthItems: diam.Some(uint32(1)), SipAuthDataItem: []swx.SipAuthDataItem{gVector}, TgppAaaServerName: diam.Some(aaa),
				OcSupportedFeatures: gOcSF, OcOlr: gOlr, Load: gLoad, SupportedFeatures: gFeatures,
			},
			fswx(303, pxy, session("aaa;1;1"), drmp(), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), utf8(avp.UserName, nai), s.a(avp.SIPNumberAuthItems, tgpp, datatype.Unsigned32(1)), fVector(),
				s.a(avp.TGPPAAAServerName, tgpp, datatype.DiameterIdentity(aaa)), fOcSF(), fOlr(), fLoad(), fFeatures(s)),
		},
		{"PPR",
			&swx.PushProfileRequest{
				Header: hdr, SessionId: "hss;1;2", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: gVSA, AuthSessionState: nsm,
				OriginHost: hss, OriginRealm: realm, DestinationHost: aaa, DestinationRealm: realm, UserName: nai,
				Non3GppUserData: &swx.Non3GppUserData{TraceInfo: &swx.TraceInfo{TraceReference: diam.Some(traceRef)}},
				PprFlags:        diam.Some(uint32(0xf)), SupportedFeatures: gFeatures,
			},
			fswx(305, req|pxy, session("hss;1;2"), drmp(), fVSA(), authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, aaa), ident(avp.DestinationRealm, realm), utf8(avp.UserName, nai),
				q.g(avp.Non3GPPUserData, tgpp, q.g(avp.TraceInfo, tgpp, q.a(avp.TraceReference, tgpp, datatype.OctetString(traceRef)))),
				q.r("PPR-Flags", datatype.Unsigned32(0xf)), fFeatures(q)),
		},
		{"PPA",
			&swx.PushProfileAnswer{
				Header: hdr, SessionId: "hss;1;2", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: aaa, OriginRealm: realm,
				AccessNetworkInfo: &swx.AccessNetworkInfo{
					Ssid: diam.Some("operator-wifi"), Bssid: diam.Some("00-10-A4-23-19-C1"), LocationInformation: diam.Some([]byte{0, 0, 0, 1}),
					LocationData: diam.Some([]byte("US")), OperatorName: diam.Some([]byte("1example.org")), LogicalAccessId: diam.Some([]byte("port-7")),
				},
				LocalTimeZone:     &s6a.LocalTimeZone{TimeZone: "+04", DaylightSavingTime: s6a.DaylightSavingTimePlusOneHourAdjustment},
				SupportedFeatures: gFeatures,
			},
			fswx(305, pxy, session("hss;1;2"), drmp(), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, aaa),
				ident(avp.OriginRealm, realm),
				s.rg("Access-Network-Info", s.a(avp.SSID, tgpp, datatype.UTF8String("operator-wifi")), s.a(avp.BSSID, tgpp, datatype.UTF8String("00-10-A4-23-19-C1")),
					s.r("Location-Information", datatype.OctetString([]byte{0, 0, 0, 1})), s.r("Location-Data", datatype.OctetString("US")),
					s.r("Operator-Name", datatype.OctetString("1example.org")), s.r("Logical-Access-ID", datatype.OctetString("port-7"))),
				s.rg("Local-Time-Zone", s.r("Time-Zone", datatype.UTF8String("+04")), s.r("Daylight-Saving-Time", datatype.Enumerated(1))),
				fFeatures(s)),
		},
		{"SAR",
			&swx.ServerAssignmentRequest{
				Header: hdr, SessionId: "aaa;1;3", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: gVSA, AuthSessionState: nsm,
				OriginHost: aaa, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
				ServiceSelection: diam.Some(apn), ContextIdentifier: diam.Some(uint32(1)), Mip6AgentInfo: gPgw(),
				VisitedNetworkIdentifier: diam.Some([]byte(vni)), UserName: nai, ServerAssignmentType: cx.ServerAssignmentTypePgwUpdate,
				ActiveApn:           []s6a.ActiveApn{{ContextIdentifier: 1, ServiceSelection: diam.Some(apn), Mip6AgentInfo: gPgw()}},
				OcSupportedFeatures: gOcSF, SupportedFeatures: gFeatures, TerminalInformation: gTerminal, EmergencyServices: diam.Some(uint32(1)),
			},
			fswx(301, req|pxy, session("aaa;1;3"), drmp(), fVSA(), authState(), ident(avp.OriginHost, aaa), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, hss), ident(avp.DestinationRealm, realm), q.a(avp.ServiceSelection, 0, datatype.UTF8String(apn)),
				q.a(avp.ContextIdentifier, tgpp, datatype.Unsigned32(1)), fPgw(), q.a(avp.VisitedNetworkIdentifier, tgpp, datatype.OctetString(vni)),
				utf8(avp.UserName, nai), q.a(avp.ServerAssignmentType, tgpp, datatype.Enumerated(13)),
				q.g(avp.ActiveAPN, tgpp, q.a(avp.ContextIdentifier, tgpp, datatype.Unsigned32(1)), q.a(avp.ServiceSelection, 0, datatype.UTF8String(apn)), fPgw()),
				fOcSF(), fFeatures(q), fTerminal(), q.r("Emergency-Services", datatype.Unsigned32(1))),
		},
		{"SAA",
			&swx.ServerAssignmentAnswer{
				Header: hdr, SessionId: "aaa;1;3", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, UserName: nai,
				Non3GppUserData: gProfile, TgppAaaServerName: diam.Some(aaa), OcSupportedFeatures: gOcSF, OcOlr: gOlr, Load: gLoad,
				SupportedFeatures: gFeatures,
			},
			fswx(301, pxy, session("aaa;1;3"), drmp(), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), utf8(avp.UserName, nai), fProfile(), s.a(avp.TGPPAAAServerName, tgpp, datatype.DiameterIdentity(aaa)),
				fOcSF(), fOlr(), fLoad(), fFeatures(s)),
		},
		// An unknown user: Experimental-Result, no profile (TS 29.273 §8.1.2.2.2.2).
		{"SAA-unknown",
			&swx.ServerAssignmentAnswer{
				Header: hdr, SessionId: "aaa;1;5", VendorSpecificApplicationId: gVSA, ExperimentalResult: gExp, AuthSessionState: nsm,
				OriginHost: hss, OriginRealm: realm, UserName: nai,
			},
			fswx(301, pxy, session("aaa;1;5"), fVSA(), fExp(), authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				utf8(avp.UserName, nai)),
		},
		{"RTR",
			&swx.RegistrationTerminationRequest{
				Header: hdr, SessionId: "hss;1;4", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: gVSA, AuthSessionState: nsm,
				OriginHost: hss, OriginRealm: realm, DestinationHost: aaa, DestinationRealm: realm, UserName: nai,
				DeregistrationReason: cx.DeregistrationReason{ReasonCode: cx.ReasonCodePermanentTermination, ReasonInfo: diam.Some("withdrawn")},
				SupportedFeatures:    gFeatures,
			},
			fswx(304, req|pxy, session("hss;1;4"), drmp(), fVSA(), authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, aaa), ident(avp.DestinationRealm, realm), utf8(avp.UserName, nai),
				q.g(avp.DeregistrationReason, tgpp, q.a(avp.ReasonCode, tgpp, datatype.Enumerated(0)), q.a(avp.ReasonInfo, tgpp, datatype.UTF8String("withdrawn"))),
				fFeatures(q)),
		},
		{"RTA",
			&swx.RegistrationTerminationAnswer{
				Header: hdr, SessionId: "hss;1;4", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: aaa, OriginRealm: realm, SupportedFeatures: gFeatures,
			},
			fswx(304, pxy, session("hss;1;4"), drmp(), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, aaa),
				ident(avp.OriginRealm, realm), fFeatures(s)),
		},
	}
}

// TestFiorixSwxApp states what the SWx interop relies on: go-diameter's SWx
// application with commands 301, 303 and 304 (not 305), and the lists of
// AVPs it lacks kept exact.
func TestFiorixSwxApp(t *testing.T) {
	if _, err := dict.Default.App(swxApp); err != nil {
		t.Fatalf("go-diameter has no SWx (%d) application: %v", swxApp, err)
	}
	for _, c := range []uint32{301, 303, 304} {
		if _, err := dict.Default.FindCommand(swxApp, c); err != nil {
			t.Errorf("go-diameter's SWx application has no command %d: %v", c, err)
		}
	}
	if _, err := dict.Default.FindCommand(swxApp, 305); err == nil {
		t.Error("go-diameter's SWx application now has command 305 (PPR/PPA): update the comment at the top of this file")
	}
	for k, name := range fiorixSwxGaps {
		if _, err := dict.Default.FindAVPByCode(swxApp, k[0], k[1]); err == nil {
			t.Errorf("go-diameter's SWx dictionary now has %s (%d vendor %d): remove it from fiorixSwxGaps", name, k[0], k[1])
		}
		if fiorixAVP(k[0], k[1]) == nil {
			t.Errorf("%s (%d vendor %d) is in no go-diameter application", name, k[0], k[1])
		}
	}
	for name, d := range swxRestated {
		if _, def := fiorixHome(d.code, d.vendor); def != nil {
			t.Errorf("go-diameter now defines %s (%d vendor %d) in application %s: remove it from swxRestated", name, d.code, d.vendor, def.App.Name)
		}
	}
}

// fiorixSwxUnknown is what go-diameter may decode as Unknown in an SWx
// message: the AVPs it has only elsewhere and those it has nowhere.
var fiorixSwxUnknown = func() map[[2]uint32]string {
	out := map[[2]uint32]string{}
	for k, name := range fiorixSwxGaps {
		out[k] = name
	}
	for name, d := range swxRestated {
		out[[2]uint32{d.code, d.vendor}] = name
	}
	return out
}()
