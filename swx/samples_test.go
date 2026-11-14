package swx

import (
	"net/netip"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/cx"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6a"
)

const (
	aaa, hss, realm = "aaa.epc.example.org", "hss.epc.example.org", "epc.example.org"
	// User-Name on SWx is the NAI of the permanent identity (TS 29.273
	// §8.1.2.1).
	nai  = "0001010000000001@wlan.mnc001.mcc001.3gppnetwork.org"
	imei = "353490069873319"
	apn  = "internet"
)

var (
	hdr = diam.Header{HopByHop: 0x11, EndToEnd: 0x22}
	// §8.2.1: Auth-Application-Id 16777265 inside the 3GPP Vendor-Specific-Application-Id.
	vsai = base.VendorSpecificApplicationId{VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777265))}
	nsm  = base.AuthSessionStateNoStateMaintained

	// §8.2.3.15-8.2.3.16: Feature-List-ID 1, HSS Restoration and Access
	// Network Information Retrieval.
	features = []cx.SupportedFeatures{{VendorId: 10415, FeatureListId: 1, FeatureList: 0x03}}
	ocSF     = &cx.OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))}
	olr      = &cx.OcOlr{OcSequenceNumber: 9, OcReportType: cx.OcReportTypeRealmReport,
		OcReductionPercentage: diam.Some(uint32(50)), OcValidityDuration: diam.Some(uint32(30))}
	load = []cx.Load{{LoadType: diam.Some(cx.LoadTypeHost), LoadValue: diam.Some(uint64(40)), Sourceid: diam.Some(hss)}}

	terminal = &s6a.TerminalInformation{Imei: diam.Some(imei), SoftwareVersion: diam.Some("03")}
	pgw      = &s6a.Mip6AgentInfo{MipHomeAgentAddress: []netip.Addr{netip.MustParseAddr("192.0.2.10")}}

	// An EAP-AKA' vector (§8.1.2.1.1): RAND||AUTN, XRES, CK', IK'.
	vector = SipAuthDataItem{
		SipItemNumber: diam.Some(uint32(1)), SipAuthenticationScheme: diam.Some("EAP-AKA'"),
		SipAuthenticate: diam.Some(make([]byte, 32)), SipAuthorization: diam.Some(make([]byte, 16)),
		ConfidentialityKey: diam.Some(make([]byte, 16)), IntegrityKey: diam.Some(make([]byte, 16)),
	}

	apnConfig = s6a.ApnConfiguration{
		ContextIdentifier: 1, PdnType: s6a.PdnTypeIpv4v6, ServiceSelection: apn,
		EpsSubscribedQosProfile: &s6a.EpsSubscribedQosProfile{
			QosClassIdentifier:          s6a.QosClassIdentifierQci9,
			AllocationRetentionPriority: s6a.AllocationRetentionPriority{PriorityLevel: 8},
		},
		Ambr: &s6a.Ambr{MaxRequestedBandwidthUl: 50000000, MaxRequestedBandwidthDl: 100000000},
	}
	traceData = &s6a.TraceData{
		TraceReference: []byte{0x00, 0xf1, 0x10, 0x00, 0x00, 0x01}, TraceDepth: 1, TraceNeTypeList: []byte{0x08},
		TraceEventList: []byte{0x01}, TraceCollectionEntity: netip.MustParseAddr("192.0.2.20"),
	}

	// The full profile of a SAR with Server-Assignment-Type REGISTRATION.
	newProfile = func() *Non3GppUserData {
		return &Non3GppUserData{
			SubscriptionId:              &SubscriptionId{SubscriptionIdType: diam.Some(SubscriptionIdTypeEndUserE164), SubscriptionIdData: diam.Some("15551234567")},
			Non3GppIpAccess:             diam.Some(Non3GppIpAccessNon3gppSubscriptionAllowed),
			Non3GppIpAccessApn:          diam.Some(Non3GppIpAccessApnNon3gppApnsEnable),
			RatType:                     []s6a.RatType{s6a.RatTypeVirtual},
			SessionTimeout:              diam.Some(uint32(3600)),
			Mip6FeatureVector:           diam.Some(uint64(0x0000400000000000)), // GTPv2_SUPPORTED (§8.2.3.28)
			Ambr:                        &s6a.Ambr{MaxRequestedBandwidthUl: 100000000, MaxRequestedBandwidthDl: 200000000},
			TgppChargingCharacteristics: diam.Some("0800"),
			ContextIdentifier:           diam.Some(uint32(1)),
			ApnOiReplacement:            diam.Some("mnc001.mcc001.gprs"),
			ApnConfiguration:            []s6a.ApnConfiguration{apnConfig},
			TraceInfo:                   &TraceInfo{TraceData: traceData},
			TwanDefaultApnContextId:     diam.Some(uint32(1)),
			TwanAccessInfo: []TwanAccessInfo{{
				AccessAuthorizationFlags: diam.Some(uint32(0x3)), // EPC and NSWO (table 8.2.3.20/1)
				WlanIdentifier:           &WlanIdentifier{Ssid: diam.Some("operator-wifi"), Hessid: diam.Some("00-10-A4-23-19-C0")},
			}},
			UeUsageType:             diam.Some(uint32(7)),
			EmergencyInfo:           &s6a.EmergencyInfo{Mip6AgentInfo: pgw},
			ErpAuthorization:        diam.Some(uint32(1)), // ERP_AUTHORIZED (§8.2.3.27)
			CoreNetworkRestrictions: diam.Some(uint32(0)),
			MpsPriority:             diam.Some(uint32(1)),
		}
	}

	newAccessNetwork = func() *AccessNetworkInfo {
		return &AccessNetworkInfo{
			Ssid: diam.Some("operator-wifi"), Bssid: diam.Some("00-10-A4-23-19-C1"),
			LocationInformation: diam.Some([]byte{0, 0, 0, 1}), LocationData: diam.Some([]byte{'U', 'S'}),
			OperatorName: diam.Some([]byte("1example.org")), LogicalAccessId: diam.Some([]byte("port-7")),
		}
	}
)

// samples returns one message per SWx command, request then answer
// (§8.2.2), MAR, MAA, PPR, PPA, SAR, SAA, RTR, RTA. Together they carry
// every AVP of the SWx dictionaries.
func samples() []diam.Message {
	return []diam.Message{
		&MultimediaAuthRequest{
			Header: hdr, SessionId: "aaa.epc.example.org;1;1", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: aaa, OriginRealm: realm, DestinationRealm: realm, DestinationHost: diam.Some(hss),
			UserName: nai, RatType: diam.Some(s6a.RatTypeWlan), AnTrusted: diam.Some(AnTrustedUntrusted), Anid: diam.Some("WLAN"),
			VisitedNetworkIdentifier: diam.Some([]byte("mnc002.mcc001.3gppnetwork.org")), TerminalInformation: terminal,
			SipAuthDataItem: SipAuthDataItem{SipAuthenticationScheme: diam.Some("EAP-AKA'")}, SipNumberAuthItems: 1,
			AaaFailureIndication: diam.Some(uint32(1)), OcSupportedFeatures: ocSF, SupportedFeatures: features,
		},
		&MultimediaAuthAnswer{
			Header: hdr, SessionId: "aaa.epc.example.org;1;1", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: vsai,
			ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, UserName: nai,
			SipNumberAuthItems: diam.Some(uint32(1)), SipAuthDataItem: []SipAuthDataItem{vector},
			TgppAaaServerName: diam.Some(aaa), OcSupportedFeatures: ocSF, OcOlr: olr, Load: load, SupportedFeatures: features,
		},
		&PushProfileRequest{
			Header: hdr, SessionId: "hss.epc.example.org;1;2", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, DestinationHost: aaa, DestinationRealm: realm, UserName: nai,
			// Trace deactivation: Trace-Reference directly under Trace-Info (§8.2.3.13).
			Non3GppUserData:   &Non3GppUserData{TraceInfo: &TraceInfo{TraceReference: diam.Some(traceData.TraceReference)}},
			PprFlags:          diam.Some(uint32(0x6)), // Access-Network-Info-Request, UE-Local-Time-Zone-Request
			SupportedFeatures: features,
		},
		&PushProfileAnswer{
			Header: hdr, SessionId: "hss.epc.example.org;1;2", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: vsai,
			ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: aaa, OriginRealm: realm,
			AccessNetworkInfo: newAccessNetwork(),
			LocalTimeZone:     &s6a.LocalTimeZone{TimeZone: "+04", DaylightSavingTime: s6a.DaylightSavingTimeNoAdjustment},
			SupportedFeatures: features,
		},
		&ServerAssignmentRequest{
			Header: hdr, SessionId: "aaa.epc.example.org;1;3", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: aaa, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			ServiceSelection: diam.Some(apn), ContextIdentifier: diam.Some(uint32(1)), Mip6AgentInfo: pgw,
			VisitedNetworkIdentifier: diam.Some([]byte("mnc002.mcc001.3gppnetwork.org")), UserName: nai,
			ServerAssignmentType: cx.ServerAssignmentTypePgwUpdate,
			ActiveApn:            []s6a.ActiveApn{{ContextIdentifier: 1, ServiceSelection: diam.Some(apn), Mip6AgentInfo: pgw}},
			OcSupportedFeatures:  ocSF, SupportedFeatures: features, TerminalInformation: terminal,
			EmergencyServices: diam.Some(uint32(1)),
		},
		&ServerAssignmentAnswer{
			Header: hdr, SessionId: "aaa.epc.example.org;1;3", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: vsai,
			ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, UserName: nai,
			Non3GppUserData: newProfile(), TgppAaaServerName: diam.Some(aaa), OcSupportedFeatures: ocSF, OcOlr: olr, Load: load,
			SupportedFeatures: features,
		},
		&RegistrationTerminationRequest{
			Header: hdr, SessionId: "hss.epc.example.org;1;4", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, DestinationHost: aaa, DestinationRealm: realm, UserName: nai,
			DeregistrationReason: cx.DeregistrationReason{ReasonCode: cx.ReasonCodePermanentTermination, ReasonInfo: diam.Some("subscription withdrawn")},
			SupportedFeatures:    features,
		},
		&RegistrationTerminationAnswer{
			Header: hdr, SessionId: "hss.epc.example.org;1;4", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: vsai,
			ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: aaa, OriginRealm: realm,
			SupportedFeatures: features,
		},
	}
}
