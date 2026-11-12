package interop

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6a"
)

// s6aApp is the S6a/S6d application id (TS 29.272 §7.1.8).
const s6aApp = 16777251

// s6aM holds the S6a M-bit of the AVPs whose flag rule in go-diameter's
// dictionary is not the TS 29.272 one. Each has a reviewed line in
// testdata/dictcheck-allow.txt citing the clause. Everything else about
// these AVPs (code, vendor, type, encoding) still comes from go-diameter.
var s6aM = map[uint32]bool{
	avp.RATType:                  true,  // table 7.3.1/2: Must set
	avp.IDRFlags:                 true,  // table 7.3.1/1: M, V
	avp.ActiveAPN:                false, // table 7.3.1/1: M must not
	avp.ExtPDPType:               false, // table 7.3.1/1: M must not
	avp.ExtPDPAddress:            false, // table 7.3.1/1: M must not
	avp.MDTConfiguration:         false, // table 7.3.1/1: M must not
	avp.GMLCAddress:              false, // table 7.3.1/2: Must not set
	avp.VisitedNetworkIdentifier: false, // table 7.3.1/2: Must not set
	avp.UserCSGInformation:       false, // table 7.3.1/2: Must not set
}

// fiorixS6aGaps are AVPs of the samples that go-diameter's dictionary has
// only under other applications, not under S6a (16777251). go-diameter
// therefore decodes them as Unknown inside an S6a message; the samples still
// build them from go-diameter's own definition (code, flags, type) found in
// the other application. TestFiorixS6aGaps keeps the list exact.
var fiorixS6aGaps = map[[2]uint32]string{
	{1471, tgpp}: "3GPP2-MEID (go-diameter: S13 only)",
	{486, 0}:     "MIP6-Agent-Info",
	{493, 0}:     "Service-Selection",
	{649, 0}:     "SourceID",
	{650, 0}:     "Load",
	{651, 0}:     "Load-Type",
}

// sa builds a go-diameter AVP for an S6a message from go-diameter's own
// dictionary: its S6a application (with the inherited base protocol), or,
// for the AVPs in fiorixS6aGaps, the application that has them. An AVP that
// go-diameter does not have at all cannot be on this independent side, and
// the samples leave it out (see testdata/dictcheck-absent.txt).
func sa(code, vendor uint32, data datatype.Type) *fd.AVP {
	def, err := dict.Default.FindAVPByCode(s6aApp, code, vendor)
	if err != nil {
		if _, gap := fiorixS6aGaps[[2]uint32{code, vendor}]; !gap {
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
	if vendor == tgpp {
		if m, ok := s6aM[code]; ok {
			flags = 0
			if m {
				flags = avp.Mbit
			}
		}
	}
	return fd.NewAVP(code, flags, vendor, data)
}

func TestFiorixS6aGaps(t *testing.T) {
	for k, name := range fiorixS6aGaps {
		if _, err := dict.Default.FindAVPByCode(s6aApp, k[0], k[1]); err == nil {
			t.Errorf("go-diameter's S6a dictionary now has %s (%d vendor %d): remove it from fiorixS6aGaps", name, k[0], k[1])
		}
		if fiorixAVP(k[0], k[1]) == nil {
			t.Errorf("%s (%d vendor %d) is in no go-diameter application", name, k[0], k[1])
		}
	}
}

const tgpp = 10415

func su32(code, v uint32) *fd.AVP        { return sa(code, tgpp, datatype.Unsigned32(v)) }
func senum(code uint32, v int32) *fd.AVP { return sa(code, tgpp, datatype.Enumerated(v)) }
func soct(code uint32, b ...byte) *fd.AVP {
	return sa(code, tgpp, datatype.OctetString(b))
}
func sutf8(code uint32, s string) *fd.AVP { return sa(code, tgpp, datatype.UTF8String(s)) }
func sgrp(code uint32, avps ...*fd.AVP) *fd.AVP {
	return sa(code, tgpp, &fd.GroupedAVP{AVP: avps})
}
func sipAddr(code, vendor uint32, s string) *fd.AVP {
	ip := net.ParseIP(s)
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return sa(code, vendor, datatype.Address(ip))
}

// fs6a builds a go-diameter message of the S6a application.
func fs6a(cmd uint32, flags uint8, avps ...*fd.AVP) *fd.Message {
	m := fd.NewMessage(cmd, flags, s6aApp, hopByHop, endToEnd, dict.Default)
	for _, a := range avps {
		m.AddAVP(a)
	}
	return m
}

// s6aPairs is one message per S6a/S6d command, built with go-diam and with
// go-diameter. They use only AVPs that go-diameter's S6a dictionary has, so
// several Release 9+ AVPs (EPS-User-State, Local-Time-Zone, monitoring,
// eDRX, V2X, ...) are covered by the round-trip tests in package s6a only.
// Load-Value is left out of the Load AVP: go-diameter types it Unsigned32,
// RFC 8583 §7.3 says Unsigned64.
func s6aPairs() []pair {
	const (
		mme, hss, realm = "mme.example.org", "hss.example.org", "example.org"
		imsi            = "001010000000001"
	)
	plmn := []byte{0x00, 0xf1, 0x10}
	when := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	nsm := base.AuthSessionStateNoStateMaintained

	// Base AVPs: go-diameter's base dictionary, as in pairs().
	session := func(s string) *fd.AVP { return fa(avp.SessionID, datatype.UTF8String(s)) }
	authState := func() *fd.AVP { return enum(avp.AuthSessionState, 1) }
	gProxy := []base.ProxyInfo{{ProxyHost: "dra.example.org", ProxyState: []byte{1, 2, 3}}}
	fProxy := func() *fd.AVP {
		return group(avp.ProxyInfo, ident(avp.ProxyHost, "dra.example.org"), octets(avp.ProxyState, 1, 2, 3))
	}
	gFailed := &base.FailedAvp{Unknown: []diam.RawAVP{{Code: base.AVPOriginStateId, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 9}}}}
	fFailed := func() *fd.AVP { return group(avp.FailedAVP, u32(avp.OriginStateID, 9)) }

	gFeatures := []s6a.SupportedFeatures{{VendorId: tgpp, FeatureListId: 1, FeatureList: 0x0bff}}
	fFeatures := func() *fd.AVP {
		return sgrp(avp.SupportedFeatures, sa(avp.VendorID, 0, datatype.Unsigned32(tgpp)), su32(avp.FeatureListID, 1), su32(avp.FeatureList, 0x0bff))
	}
	gOlr := &s6a.OcOlr{OcSequenceNumber: 9, OcReportType: s6a.OcReportTypeRealmReport, OcReductionPercentage: diam.Some(uint32(50)), OcValidityDuration: diam.Some(uint32(30))}
	fOlr := func() *fd.AVP {
		return sa(avp.OCOLR, 0, &fd.GroupedAVP{AVP: []*fd.AVP{
			sa(avp.OCSequenceNumber, 0, datatype.Unsigned64(9)), sa(avp.OCReportType, 0, datatype.Enumerated(1)),
			sa(avp.OCReductionPercentage, 0, datatype.Unsigned32(50)), sa(avp.OCValidityDuration, 0, datatype.Unsigned32(30)),
		}})
	}
	gOcSF := &s6a.OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))}
	fOcSF := func() *fd.AVP {
		return sa(avp.OCSupportedFeatures, 0, &fd.GroupedAVP{AVP: []*fd.AVP{sa(avp.OCFeatureVector, 0, datatype.Unsigned64(1))}})
	}
	gLoad := []s6a.Load{{LoadType: diam.Some(s6a.LoadTypeHost), Sourceid: diam.Some(hss)}}
	fLoad := func() *fd.AVP {
		return sa(avp.Load, 0, &fd.GroupedAVP{AVP: []*fd.AVP{
			sa(avp.LoadType, 0, datatype.Enumerated(0)), sa(avp.SourceID, 0, datatype.DiameterIdentity(hss)),
		}})
	}
	gPgw := func() *s6a.Mip6AgentInfo {
		return &s6a.Mip6AgentInfo{
			MipHomeAgentAddress: []netip.Addr{netip.MustParseAddr("192.0.2.10"), netip.MustParseAddr("2001:db8::10")},
			MipHomeAgentHost:    &s6a.MipHomeAgentHost{DestinationRealm: realm, DestinationHost: "pgw.example.org"},
		}
	}
	fPgw := func() *fd.AVP {
		return sa(avp.MIP6AgentInfo, 0, &fd.GroupedAVP{AVP: []*fd.AVP{
			sipAddr(avp.MIPHomeAgentAddress, 0, "192.0.2.10"), sipAddr(avp.MIPHomeAgentAddress, 0, "2001:db8::10"),
			sa(avp.MIPHomeAgentHost, 0, &fd.GroupedAVP{AVP: []*fd.AVP{ident(avp.DestinationRealm, realm), ident(avp.DestinationHost, "pgw.example.org")}}),
		}})
	}
	gAmbr := &s6a.Ambr{MaxRequestedBandwidthUl: 50_000_000, MaxRequestedBandwidthDl: 100_000_000,
		ExtendedMaxRequestedBwUl: diam.Some(uint32(5_000_000)), ExtendedMaxRequestedBwDl: diam.Some(uint32(10_000_000))}
	fAmbr := func() *fd.AVP {
		return sgrp(avp.AMBR, su32(avp.MaxRequestedBandwidthUL, 50_000_000), su32(avp.MaxRequestedBandwidthDL, 100_000_000),
			su32(avp.ExtendedMaxRequestedBWUL, 5_000_000), su32(avp.ExtendedMaxRequestedBWDL, 10_000_000))
	}

	gSub := &s6a.SubscriptionData{
		SubscriberStatus: diam.Some(s6a.SubscriberStatusServiceGranted), Msisdn: diam.Some([]byte{0x21, 0x43, 0x65, 0x87}),
		StnSr: diam.Some([]byte{0x91, 0x21}), IcsIndicator: diam.Some(s6a.IcsIndicatorTrue),
		NetworkAccessMode: diam.Some(s6a.NetworkAccessModeOnlyPacket), OperatorDeterminedBarring: diam.Some(uint32(0)),
		HplmnOdb: diam.Some(uint32(1)), RegionalSubscriptionZoneCode: [][]byte{{0, 1}},
		AccessRestrictionData: diam.Some(uint32(0x20)), ApnOiReplacement: diam.Some("mnc001.mcc001.gprs"),
		LcsInfo: &s6a.LcsInfo{
			GmlcNumber: [][]byte{{0x91, 0x11}},
			LcsPrivacyexception: []s6a.LcsPrivacyexception{{
				SsCode: []byte{0x11}, SsStatus: []byte{0x05}, NotificationToUeUser: diam.Some(s6a.NotificationToUeUserLocationNotAllowed),
				ExternalClient: []s6a.ExternalClient{{ClientIdentity: []byte{0x91, 0x22}, GmlcRestriction: diam.Some(s6a.GmlcRestrictionHomeCountry)}},
				PlmnClient:     []s6a.PlmnClient{s6a.PlmnClientOAndMHplmn},
				ServiceType:    []s6a.ServiceType{{Servicetypeidentity: 3}},
			}},
			MoLr: []s6a.MoLr{{SsCode: []byte{0x12}, SsStatus: []byte{0x05}}},
		},
		TeleserviceList:             &s6a.TeleserviceList{TsCode: [][]byte{{0x21}}},
		CallBarringInfo:             []s6a.CallBarringInfo{{SsCode: []byte{0x92}, SsStatus: []byte{0x01}}},
		TgppChargingCharacteristics: diam.Some("0800"),
		Ambr:                        gAmbr,
		ApnConfigurationProfile: &s6a.ApnConfigurationProfile{
			ContextIdentifier: 1, AllApnConfigurationsIncludedIndicator: s6a.AllApnConfigurationsIncludedIndicatorAllApnConfigurationsIncluded,
			ApnConfiguration: []s6a.ApnConfiguration{{
				ContextIdentifier: 1, ServedPartyIpAddress: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
				PdnType: s6a.PdnTypeIpv4v6, ServiceSelection: "internet",
				EpsSubscribedQosProfile: &s6a.EpsSubscribedQosProfile{
					QosClassIdentifier: s6a.QosClassIdentifierQci9,
					AllocationRetentionPriority: s6a.AllocationRetentionPriority{PriorityLevel: 8,
						PreEmptionCapability:    diam.Some(s6a.PreEmptionCapabilityPreEmptionCapabilityDisabled),
						PreEmptionVulnerability: diam.Some(s6a.PreEmptionVulnerabilityPreEmptionVulnerabilityEnabled)},
				},
				VplmnDynamicAddressAllowed: diam.Some(s6a.VplmnDynamicAddressAllowedAllowed), Mip6AgentInfo: gPgw(),
				VisitedNetworkIdentifier:    diam.Some([]byte("mnc001.mcc001.3gppnetwork.org")),
				PdnGwAllocationType:         diam.Some(s6a.PdnGwAllocationTypeDynamic),
				TgppChargingCharacteristics: diam.Some("0800"), Ambr: gAmbr,
				SpecificApnInfo: []s6a.SpecificApnInfo{{ServiceSelection: "corp", Mip6AgentInfo: *gPgw()}},
				SiptoPermission: diam.Some(s6a.SiptoPermissionSiptoAboveRanNotallowed),
				LipaPermission:  diam.Some(s6a.LipaPermissionLipaProhibited),
			}},
		},
		RatFrequencySelectionPriorityId: diam.Some(uint32(5)),
		TraceData: &s6a.TraceData{
			TraceReference: []byte{0x00, 0xf1, 0x10, 0x00, 0x00, 0x01}, TraceDepth: 1, TraceNeTypeList: []byte{0x01},
			TraceInterfaceList: diam.Some([]byte{0x02}), TraceEventList: []byte{0x01, 0x02}, OmcId: diam.Some([]byte{0x03}),
			TraceCollectionEntity: netip.MustParseAddr("192.0.2.50"),
		},
		GprsSubscriptionData: &s6a.GprsSubscriptionData{
			CompleteDataListIncludedIndicator: s6a.CompleteDataListIncludedIndicatorAllPdpContextsIncluded,
			PdpContext: []s6a.PdpContext{{
				ContextIdentifier: 1, PdpType: []byte{0x01, 0x21}, PdpAddress: diam.Some(netip.MustParseAddr("10.0.0.2")),
				QosSubscribed: []byte{0x0b, 0x92, 0x1f}, VplmnDynamicAddressAllowed: diam.Some(s6a.VplmnDynamicAddressAllowedNotallowed),
				ServiceSelection: "internet", TgppChargingCharacteristics: diam.Some("0800"),
				ExtPdpType: diam.Some([]byte{0x00, 0x8d}), ExtPdpAddress: diam.Some(netip.MustParseAddr("2001:db8::2")),
				Ambr: gAmbr, ApnOiReplacement: diam.Some("mnc001.mcc001.gprs"),
			}},
		},
		CsgSubscriptionData:                      []s6a.CsgSubscriptionData{{CsgId: 7, ExpirationDate: diam.Some(when), ServiceSelection: []string{"internet"}, VisitedPlmnId: diam.Some(plmn)}},
		RoamingRestrictedDueToUnsupportedFeature: diam.Some(s6a.RoamingRestrictedDueToUnsupportedFeatureRoamingRestrictedDueToUnsupportedFeature),
		SubscribedPeriodicRauTauTimer:            diam.Some(uint32(54)), MpsPriority: diam.Some(uint32(1)),
		VplmnLipaAllowed: diam.Some(s6a.VplmnLipaAllowedLipaNotallowed), RelayNodeIndicator: diam.Some(s6a.RelayNodeIndicatorNotRelayNode),
		MdtUserConsent: diam.Some(s6a.MdtUserConsentConsentGiven), SubscribedVsrvcc: diam.Some(s6a.SubscribedVsrvccVsrvccSubscribed),
	}
	fSub := func() *fd.AVP {
		return sgrp(avp.SubscriptionData,
			senum(avp.SubscriberStatus, 0), soct(avp.MSISDN, 0x21, 0x43, 0x65, 0x87), soct(avp.STNSR, 0x91, 0x21),
			senum(avp.ICSIndicator, 1), senum(avp.NetworkAccessMode, 2), su32(avp.OperatorDeterminedBarring, 0),
			su32(avp.HPLMNODB, 1), soct(avp.RegionalSubscriptionZoneCode, 0, 1), su32(avp.AccessRestrictionData, 0x20),
			sutf8(avp.APNOIReplacement, "mnc001.mcc001.gprs"),
			sgrp(avp.LCSInfo,
				soct(avp.GMLCNumber, 0x91, 0x11),
				sgrp(avp.LCSPrivacyException, soct(avp.SSCode, 0x11), soct(avp.SSStatus, 0x05), senum(avp.NotificationToUEUser, 3),
					sgrp(avp.ExternalClient, soct(avp.ClientIdentity, 0x91, 0x22), senum(avp.GMLCRestriction, 1)),
					senum(avp.PLMNClient, 1),
					sgrp(avp.TGPPServiceType, su32(avp.ServiceTypeIdentity, 3))),
				sgrp(avp.MOLR, soct(avp.SSCode, 0x12), soct(avp.SSStatus, 0x05))),
			sgrp(avp.TeleserviceList, soct(avp.TSCode, 0x21)),
			sgrp(avp.CallBarringInfo, soct(avp.SSCode, 0x92), soct(avp.SSStatus, 0x01)),
			sutf8(avp.TGPPChargingCharacteristics, "0800"),
			fAmbr(),
			sgrp(avp.APNConfigurationProfile,
				su32(avp.ContextIdentifier, 1), senum(avp.AllAPNConfigurationsIncludedIndicator, 0),
				sgrp(avp.APNConfiguration,
					su32(avp.ContextIdentifier, 1), sipAddr(avp.ServedPartyIPAddress, tgpp, "10.0.0.1"), senum(avp.PDNType, 2),
					sa(avp.ServiceSelection, 0, datatype.UTF8String("internet")),
					sgrp(avp.EPSSubscribedQoSProfile, senum(avp.QoSClassIdentifier, 9),
						sgrp(avp.AllocationRetentionPriority, su32(avp.PriorityLevel, 8), senum(avp.PreemptionCapability, 1), senum(avp.PreemptionVulnerability, 0))),
					senum(avp.VPLMNDynamicAddressAllowed, 1), fPgw(),
					soct(avp.VisitedNetworkIdentifier, []byte("mnc001.mcc001.3gppnetwork.org")...),
					senum(avp.PDNGWAllocationType, 1), sutf8(avp.TGPPChargingCharacteristics, "0800"), fAmbr(),
					sgrp(avp.SpecificAPNInfo, sa(avp.ServiceSelection, 0, datatype.UTF8String("corp")), fPgw()),
					senum(avp.SIPTOPermission, 1), senum(avp.LIPAPermission, 0))),
			su32(avp.RATFrequencySelectionPriorityID, 5),
			sgrp(avp.TraceData, soct(avp.TraceReference, 0x00, 0xf1, 0x10, 0x00, 0x00, 0x01), senum(avp.TraceDepth, 1),
				soct(avp.TraceNETypeList, 0x01), soct(avp.TraceInterfaceList, 0x02), soct(avp.TraceEventList, 0x01, 0x02),
				soct(avp.OMCID, 0x03), sipAddr(avp.TraceCollectionEntity, tgpp, "192.0.2.50")),
			sgrp(avp.GPRSSubscriptionData, senum(avp.CompleteDataListIncludedIndicator, 0),
				sgrp(avp.PDPContext, su32(avp.ContextIdentifier, 1), soct(avp.PDPType, 0x01, 0x21), sipAddr(avp.PDPAddress, tgpp, "10.0.0.2"),
					soct(avp.QoSSubscribed, 0x0b, 0x92, 0x1f), senum(avp.VPLMNDynamicAddressAllowed, 0),
					sa(avp.ServiceSelection, 0, datatype.UTF8String("internet")), sutf8(avp.TGPPChargingCharacteristics, "0800"),
					soct(avp.ExtPDPType, 0x00, 0x8d), sipAddr(avp.ExtPDPAddress, tgpp, "2001:db8::2"), fAmbr(),
					sutf8(avp.APNOIReplacement, "mnc001.mcc001.gprs"))),
			sgrp(avp.CSGSubscriptionData, su32(avp.CSGID, 7), sa(avp.ExpirationDate, tgpp, datatype.Time(when)),
				sa(avp.ServiceSelection, 0, datatype.UTF8String("internet")), soct(avp.VisitedPLMNID, plmn...)),
			senum(avp.RoamingRestrictedDueToUnsupportedFeature, 0), su32(avp.SubscribedPeriodicRAUTAUTimer, 54),
			su32(avp.MPSPriority, 1), senum(avp.VPLMNLIPAAllowed, 0), senum(avp.RelayNodeIndicator, 0),
			senum(avp.MDTUserConsent, 1), senum(avp.SubscribedVSRVCC, 0),
		)
	}

	return []pair{
		{"ULR",
			&s6a.UpdateLocationRequest{
				Header: hdr, SessionId: "mme;1;1", AuthSessionState: nsm, OriginHost: mme, OriginRealm: realm,
				DestinationHost: diam.Some(hss), DestinationRealm: realm, UserName: imsi, OcSupportedFeatures: gOcSF,
				SupportedFeatures:   gFeatures,
				TerminalInformation: &s6a.TerminalInformation{Imei: diam.Some("3534900698733"), Tgpp2Meid: diam.Some([]byte{1, 2}), SoftwareVersion: diam.Some("01")},
				RatType:             s6a.RatTypeEutran, UlrFlags: 0x22, UeSrvccCapability: diam.Some(s6a.UeSrvccCapabilityUeSrvccSupported),
				VisitedPlmnId: plmn, SgsnNumber: diam.Some([]byte{0x91, 0x33}),
				HomogeneousSupportOfImsVoiceOverPsSessions: diam.Some(s6a.HomogeneousSupportOfImsVoiceOverPsSessionsSupported),
				GmlcAddress:       diam.Some(netip.MustParseAddr("192.0.2.30")),
				ActiveApn:         []s6a.ActiveApn{{ContextIdentifier: 1, ServiceSelection: diam.Some("internet"), Mip6AgentInfo: gPgw()}},
				MmeNumberForMtSms: diam.Some([]byte{0x91, 0x44}), ProxyInfo: gProxy, RouteRecord: []string{"dra.example.org"},
			},
			fs6a(316, req|pxy, session("mme;1;1"), authState(), ident(avp.OriginHost, mme), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, hss), ident(avp.DestinationRealm, realm), utf8(avp.UserName, imsi), fOcSF(), fFeatures(),
				sgrp(avp.TerminalInformation, sutf8(avp.IMEI, "3534900698733"), soct(avp.TGPP2MEID, 1, 2), sutf8(avp.SoftwareVersion, "01")),
				senum(avp.RATType, 1004), su32(avp.ULRFlags, 0x22), senum(avp.UESRVCCCapability, 1), soct(avp.VisitedPLMNID, plmn...),
				soct(avp.SGSNNumber, 0x91, 0x33), senum(avp.HomogeneousSupportofIMSVoiceOverPSSessions, 1),
				sipAddr(avp.GMLCAddress, tgpp, "192.0.2.30"),
				sgrp(avp.ActiveAPN, su32(avp.ContextIdentifier, 1), sa(avp.ServiceSelection, 0, datatype.UTF8String("internet")), fPgw()),
				soct(avp.MMENumberforMTSMS, 0x91, 0x44), fProxy(), ident(avp.RouteRecord, "dra.example.org")),
		},
		{"ULA",
			&s6a.UpdateLocationAnswer{
				Header: hdr, SessionId: "mme;1;1", ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm,
				OriginHost: hss, OriginRealm: realm, OcSupportedFeatures: gOcSF, OcOlr: gOlr, Load: gLoad,
				SupportedFeatures: gFeatures, UlaFlags: diam.Some(uint32(1)), SubscriptionData: gSub,
				FailedAvp: gFailed, ProxyInfo: gProxy, RouteRecord: []string{"dra.example.org"},
			},
			fs6a(316, pxy, session("mme;1;1"), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), fOcSF(), fOlr(), fLoad(), fFeatures(), su32(avp.ULAFlags, 1), fSub(),
				fFailed(), fProxy(), ident(avp.RouteRecord, "dra.example.org")),
		},
		{"CLR",
			&s6a.CancelLocationRequest{
				Header: hdr, SessionId: "hss;1;2", AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
				DestinationHost: mme, DestinationRealm: realm, UserName: imsi, SupportedFeatures: gFeatures,
				CancellationType: s6a.CancellationTypeSubscriptionWithdrawal, ClrFlags: diam.Some(uint32(2)),
				ProxyInfo: gProxy, RouteRecord: []string{"dra.example.org"},
			},
			fs6a(317, req|pxy, session("hss;1;2"), authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, mme), ident(avp.DestinationRealm, realm), utf8(avp.UserName, imsi), fFeatures(),
				senum(avp.CancellationType, 2), su32(avp.CLRFlags, 2), fProxy(), ident(avp.RouteRecord, "dra.example.org")),
		},
		{"CLA",
			&s6a.CancelLocationAnswer{
				Header: hdr, SessionId: "hss;1;2", SupportedFeatures: gFeatures, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: mme, OriginRealm: realm, FailedAvp: gFailed, ProxyInfo: gProxy,
			},
			fs6a(317, pxy, session("hss;1;2"), fFeatures(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, mme),
				ident(avp.OriginRealm, realm), fFailed(), fProxy()),
		},
		{"AIR",
			&s6a.AuthenticationInformationRequest{
				Header: hdr, SessionId: "mme;1;3", AuthSessionState: nsm, OriginHost: mme, OriginRealm: realm,
				DestinationRealm: realm, UserName: imsi, SupportedFeatures: gFeatures,
				RequestedEutranAuthenticationInfo: &s6a.RequestedEutranAuthenticationInfo{
					NumberOfRequestedVectors: diam.Some(uint32(2)), ImmediateResponsePreferred: diam.Some(uint32(1)),
					ReSynchronizationInfo: diam.Some([]byte{0xaa, 0xbb}),
				},
				RequestedUtranGeranAuthenticationInfo: &s6a.RequestedUtranGeranAuthenticationInfo{NumberOfRequestedVectors: diam.Some(uint32(1))},
				VisitedPlmnId:                         plmn, ProxyInfo: gProxy, RouteRecord: []string{"dra.example.org"},
			},
			fs6a(318, req|pxy, session("mme;1;3"), authState(), ident(avp.OriginHost, mme), ident(avp.OriginRealm, realm),
				ident(avp.DestinationRealm, realm), utf8(avp.UserName, imsi), fFeatures(),
				sgrp(avp.RequestedEUTRANAuthenticationInfo, su32(avp.NumberOfRequestedVectors, 2), su32(avp.ImmediateResponsePreferred, 1),
					soct(avp.ResynchronizationInfo, 0xaa, 0xbb)),
				sgrp(avp.RequestedUTRANGERANAuthenticationInfo, su32(avp.NumberOfRequestedVectors, 1)),
				soct(avp.VisitedPLMNID, plmn...), fProxy(), ident(avp.RouteRecord, "dra.example.org")),
		},
		{"AIA",
			&s6a.AuthenticationInformationAnswer{
				Header: hdr, SessionId: "mme;1;3", ResultCode: diam.Some(uint32(2001)),
				ErrorDiagnostic: diam.Some(s6a.ErrorDiagnosticNoGprsDataSubscribed), AuthSessionState: nsm,
				OriginHost: hss, OriginRealm: realm, OcOlr: gOlr, SupportedFeatures: gFeatures,
				AuthenticationInfo: &s6a.AuthenticationInfo{
					EUtranVector: []s6a.EUtranVector{{ItemNumber: diam.Some(uint32(1)), Rand: []byte{1}, Xres: []byte{2}, Autn: []byte{3}, Kasme: []byte{4}}},
					UtranVector:  []s6a.UtranVector{{Rand: []byte{1}, Xres: []byte{2}, Autn: []byte{3}, ConfidentialityKey: []byte{5}, IntegrityKey: []byte{6}}},
					GeranVector:  []s6a.GeranVector{{Rand: []byte{1}, Sres: []byte{7}, Kc: []byte{8}}},
				},
				FailedAvp: gFailed, ProxyInfo: gProxy,
			},
			fs6a(318, pxy, session("mme;1;3"), u32(avp.ResultCode, 2001), senum(avp.ErrorDiagnostic, 1), authState(),
				ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm), fOlr(), fFeatures(),
				sgrp(avp.AuthenticationInfo,
					sgrp(avp.EUTRANVector, su32(avp.ItemNumber, 1), soct(avp.RAND, 1), soct(avp.XRES, 2), soct(avp.AUTN, 3), soct(avp.KASME, 4)),
					sgrp(avp.UTRANVector, soct(avp.RAND, 1), soct(avp.XRES, 2), soct(avp.AUTN, 3), soct(avp.ConfidentialityKey, 5), soct(avp.IntegrityKey, 6)),
					sgrp(avp.GERANVector, soct(avp.RAND, 1), soct(avp.SRES, 7), soct(avp.Kc, 8))),
				fFailed(), fProxy()),
		},
		{"IDR",
			&s6a.InsertSubscriberDataRequest{
				Header: hdr, SessionId: "hss;1;4", AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
				DestinationHost: mme, DestinationRealm: realm, UserName: imsi, SupportedFeatures: gFeatures,
				SubscriptionData: *gSub, IdrFlags: diam.Some(uint32(0x10)), ProxyInfo: gProxy, RouteRecord: []string{"dra.example.org"},
			},
			fs6a(319, req|pxy, session("hss;1;4"), authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, mme), ident(avp.DestinationRealm, realm), utf8(avp.UserName, imsi), fFeatures(),
				fSub(), su32(avp.IDRFlags, 0x10), fProxy(), ident(avp.RouteRecord, "dra.example.org")),
		},
		{"IDA",
			&s6a.InsertSubscriberDataAnswer{
				Header: hdr, SessionId: "hss;1;4", SupportedFeatures: gFeatures, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: mme, OriginRealm: realm, RatType: diam.Some(s6a.RatTypeUtran),
				FailedAvp: gFailed, ProxyInfo: gProxy,
			},
			fs6a(319, pxy, session("hss;1;4"), fFeatures(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, mme),
				ident(avp.OriginRealm, realm), senum(avp.RATType, 1000), fFailed(), fProxy()),
		},
		{"DSR",
			&s6a.DeleteSubscriberDataRequest{
				Header: hdr, SessionId: "hss;1;5", AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
				DestinationHost: mme, DestinationRealm: realm, UserName: imsi, SupportedFeatures: gFeatures, DsrFlags: 0x1,
				ContextIdentifier: []uint32{1, 2}, TraceReference: diam.Some([]byte{0x00, 0xf1, 0x10, 0x00, 0x00, 0x01}),
				TsCode: [][]byte{{0x21}}, SsCode: [][]byte{{0x11}}, ProxyInfo: gProxy, RouteRecord: []string{"dra.example.org"},
			},
			fs6a(320, req|pxy, session("hss;1;5"), authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, mme), ident(avp.DestinationRealm, realm), utf8(avp.UserName, imsi), fFeatures(),
				su32(avp.DSRFlags, 1), su32(avp.ContextIdentifier, 1), su32(avp.ContextIdentifier, 2),
				soct(avp.TraceReference, 0x00, 0xf1, 0x10, 0x00, 0x00, 0x01), soct(avp.TSCode, 0x21), soct(avp.SSCode, 0x11),
				fProxy(), ident(avp.RouteRecord, "dra.example.org")),
		},
		{"DSA",
			&s6a.DeleteSubscriberDataAnswer{
				Header: hdr, SessionId: "hss;1;5", SupportedFeatures: gFeatures, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: mme, OriginRealm: realm, DsaFlags: diam.Some(uint32(1)),
				FailedAvp: gFailed, ProxyInfo: gProxy,
			},
			fs6a(320, pxy, session("hss;1;5"), fFeatures(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, mme),
				ident(avp.OriginRealm, realm), su32(avp.DSAFlags, 1), fFailed(), fProxy()),
		},
		{"PUR",
			&s6a.PurgeUeRequest{
				Header: hdr, SessionId: "mme;1;6", AuthSessionState: nsm, OriginHost: mme, OriginRealm: realm,
				DestinationHost: diam.Some(hss), DestinationRealm: realm, UserName: imsi, OcSupportedFeatures: gOcSF,
				PurFlags: diam.Some(uint32(1)), SupportedFeatures: gFeatures, ProxyInfo: gProxy, RouteRecord: []string{"dra.example.org"},
			},
			fs6a(321, req|pxy, session("mme;1;6"), authState(), ident(avp.OriginHost, mme), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, hss), ident(avp.DestinationRealm, realm), utf8(avp.UserName, imsi), fOcSF(),
				su32(avp.PURFlags, 1), fFeatures(), fProxy(), ident(avp.RouteRecord, "dra.example.org")),
		},
		{"PUA",
			&s6a.PurgeUeAnswer{
				Header: hdr, SessionId: "mme;1;6", SupportedFeatures: gFeatures, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, OcOlr: gOlr, Load: gLoad,
				PuaFlags: diam.Some(uint32(1)), FailedAvp: gFailed, ProxyInfo: gProxy,
			},
			fs6a(321, pxy, session("mme;1;6"), fFeatures(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), fOlr(), fLoad(), su32(avp.PUAFlags, 1), fFailed(), fProxy()),
		},
		{"RSR",
			&s6a.ResetRequest{
				Header: hdr, SessionId: "hss;1;7", AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
				DestinationHost: mme, DestinationRealm: realm, SupportedFeatures: gFeatures, UserId: []string{"00101"},
				ProxyInfo: gProxy, RouteRecord: []string{"dra.example.org"},
			},
			fs6a(322, req|pxy, session("hss;1;7"), authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, mme), ident(avp.DestinationRealm, realm), fFeatures(), sutf8(avp.UserID, "00101"),
				fProxy(), ident(avp.RouteRecord, "dra.example.org")),
		},
		{"RSA",
			&s6a.ResetAnswer{
				Header: hdr, SessionId: "hss;1;7", SupportedFeatures: gFeatures, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: mme, OriginRealm: realm, FailedAvp: gFailed, ProxyInfo: gProxy,
			},
			fs6a(322, pxy, session("hss;1;7"), fFeatures(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, mme),
				ident(avp.OriginRealm, realm), fFailed(), fProxy()),
		},
		{"NOR",
			&s6a.NotifyRequest{
				Header: hdr, SessionId: "mme;1;8", AuthSessionState: nsm, OriginHost: mme, OriginRealm: realm,
				DestinationHost: diam.Some(hss), DestinationRealm: realm, UserName: imsi, OcSupportedFeatures: gOcSF,
				SupportedFeatures: gFeatures, TerminalInformation: &s6a.TerminalInformation{Imei: diam.Some("3534900698733")},
				Mip6AgentInfo: gPgw(), VisitedNetworkIdentifier: diam.Some([]byte("mnc001.mcc001.3gppnetwork.org")),
				ContextIdentifier: diam.Some(uint32(1)), ServiceSelection: diam.Some("internet"),
				UeSrvccCapability: diam.Some(s6a.UeSrvccCapabilityUeSrvccNotSupported), NorFlags: diam.Some(uint32(1)),
				HomogeneousSupportOfImsVoiceOverPsSessions: diam.Some(s6a.HomogeneousSupportOfImsVoiceOverPsSessionsNotSupported),
				ProxyInfo: gProxy, RouteRecord: []string{"dra.example.org"},
			},
			fs6a(323, req|pxy, session("mme;1;8"), authState(), ident(avp.OriginHost, mme), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, hss), ident(avp.DestinationRealm, realm), utf8(avp.UserName, imsi), fOcSF(), fFeatures(),
				sgrp(avp.TerminalInformation, sutf8(avp.IMEI, "3534900698733")), fPgw(),
				soct(avp.VisitedNetworkIdentifier, []byte("mnc001.mcc001.3gppnetwork.org")...), su32(avp.ContextIdentifier, 1),
				sa(avp.ServiceSelection, 0, datatype.UTF8String("internet")), senum(avp.UESRVCCCapability, 0), su32(avp.NORFlags, 1),
				senum(avp.HomogeneousSupportofIMSVoiceOverPSSessions, 0), fProxy(), ident(avp.RouteRecord, "dra.example.org")),
		},
		{"NOA",
			&s6a.NotifyAnswer{
				Header: hdr, SessionId: "mme;1;8", ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm,
				OriginHost: hss, OriginRealm: realm, OcSupportedFeatures: gOcSF, OcOlr: gOlr, Load: gLoad,
				SupportedFeatures: gFeatures, FailedAvp: gFailed, ProxyInfo: gProxy,
			},
			fs6a(323, pxy, session("mme;1;8"), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), fOcSF(), fOlr(), fLoad(), fFeatures(), fFailed(), fProxy()),
		},
	}
}
