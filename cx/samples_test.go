package cx

import (
	"net/netip"
	"time"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
)

const (
	icscf, scscf, hss, realm = "icscf.ims.example.org", "scscf.ims.example.org", "hss.ims.example.org", "ims.example.org"
	impi                     = "001010000000001@ims.example.org"
	impu                     = "sip:+15551234567@ims.example.org"
	scscfURI                 = "sip:scscf.ims.example.org:5060"
	akaScheme                = "Digest-AKAv1-MD5"
)

var (
	hdr = diam.Header{HopByHop: 0x11, EndToEnd: 0x22}
	// §5.6: Auth-Application-Id 16777216 inside the 3GPP Vendor-Specific-Application-Id.
	vsai = base.VendorSpecificApplicationId{VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777216))}
	nsm  = base.AuthSessionStateNoStateMaintained
	// Times are whole seconds: the Time type has no finer resolution.
	when = time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)

	proxyInfo = []base.ProxyInfo{{ProxyHost: "dra.ims.example.org", ProxyState: []byte{1, 2, 3}}}
	// Feature-List-ID 1 (table 7.1.1): SiFC, AliasInd, IMSRestorationInd, P-CSCF-Restoration-mechanism.
	features  = []SupportedFeatures{{VendorId: 10415, FeatureListId: 1, FeatureList: 0x0f}}
	failedAvp = &base.FailedAvp{Unknown: []diam.RawAVP{
		{Code: AVPUarFlags, Vendor: 10415, Flags: diam.AVPFlagVendor, Data: []byte{1, 2}},
	}}
	// DIAMETER_FIRST_REGISTRATION (§6.2.1.1).
	firstReg = &base.ExperimentalResult{VendorId: 10415, ExperimentalResultCode: 2001}
	ocSF     = &OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))}
	ocOlr    = &OcOlr{OcSequenceNumber: 9, OcReportType: OcReportTypeRealmReport,
		OcReductionPercentage: diam.Some(uint32(50)), OcValidityDuration: diam.Some(uint32(30))}
	load = []Load{{LoadType: diam.Some(LoadTypeHost), LoadValue: diam.Some(uint64(1000)), Sourceid: diam.Some(hss)}}

	capabilities = &ServerCapabilities{MandatoryCapability: []uint32{1, 2}, OptionalCapability: []uint32{7}, ServerName: []string{scscfURI}}
	charging     = &ChargingInformation{
		PrimaryEventChargingFunctionName:        diam.Some("aaa://ocs1.ims.example.org"),
		SecondaryEventChargingFunctionName:      diam.Some("aaa://ocs2.ims.example.org"),
		PrimaryChargingCollectionFunctionName:   diam.Some("aaa://cdf1.ims.example.org"),
		SecondaryChargingCollectionFunctionName: diam.Some("aaa://cdf2.ims.example.org"),
	}
	allowedWaf = &AllowedWafWwsfIdentities{
		WebrtcAuthenticationFunctionName: []string{"waf.example.org"},
		WebrtcWebServerFunctionName:      []string{"wwsf1.example.org", "wwsf2.example.org"},
	}
	userData = []byte(`<?xml version="1.0" encoding="UTF-8"?><IMSSubscription><PrivateID>` + impi + `</PrivateID></IMSSubscription>`)
)

// restoration is an SCSCF-Restoration-Info that reaches Restoration-Info,
// Subscription-Info and P-CSCF-Subscription-Info.
func restoration() ScscfRestorationInfo {
	return ScscfRestorationInfo{
		UserName: impi,
		RestorationInfo: []RestorationInfo{{
			Path:                      []byte("<sip:pcscf.ims.example.org;lr>"),
			Contact:                   []byte("<sip:ue@192.0.2.50:5060>"),
			InitialCseqSequenceNumber: diam.Some(uint32(1)),
			CallIdSipHeader:           diam.Some([]byte("a84b4c76e66710")),
			SubscriptionInfo: &SubscriptionInfo{
				CallIdSipHeader: []byte("sub-1"), FromSipHeader: []byte("<" + impu + ">;tag=1"), ToSipHeader: []byte("<" + impu + ">"),
				RecordRoute: []byte("<sip:pcscf.ims.example.org;lr>"), Contact: []byte("<sip:ue@192.0.2.50:5060>"),
			},
			PCscfSubscriptionInfo: &PCscfSubscriptionInfo{
				CallIdSipHeader: []byte("sub-2"), FromSipHeader: []byte("<sip:pcscf.ims.example.org>;tag=2"),
				ToSipHeader: []byte("<" + impu + ">"), Contact: []byte("<sip:pcscf.ims.example.org>"),
			},
		}},
		RegistrationTimeOut:     diam.Some(when),
		SipAuthenticationScheme: diam.Some(akaScheme),
	}
}

// akaVector is an IMS-AKA authentication vector (§6.3.9-6.3.11, 6.3.27-28).
func akaVector(n uint32) SipAuthDataItem {
	return SipAuthDataItem{
		SipItemNumber:           diam.Some(n),
		SipAuthenticationScheme: diam.Some(akaScheme),
		SipAuthenticate:         diam.Some(make([]byte, 32)),
		SipAuthorization:        diam.Some([]byte{0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8}),
		ConfidentialityKey:      diam.Some(make([]byte, 16)),
		IntegrityKey:            diam.Some(make([]byte, 16)),
	}
}

// digestItem is a SIP Digest item (§6.3.36).
func digestItem() SipAuthDataItem {
	return SipAuthDataItem{
		SipAuthenticationScheme: diam.Some("SIP Digest"),
		SipDigestAuthenticate: &SipDigestAuthenticate{
			DigestRealm: realm, DigestAlgorithm: diam.Some("MD5"), DigestQop: "auth",
			DigestHa1: "939e7578ed9e3c518a452acee763bce9", AlternateDigestAlgorithm: diam.Some("SHA-256"),
			AlternateDigestHa1: diam.Some("2b2d6c1dbd5a9f3f62b6f2a5f0b3a2c1d0e9f8a7b6c5d4e3f2a1b0c9d8e7f6a5"),
		},
	}
}

// nassItem is a NASS-Bundled item (§6.3.13, 6.3.42, 6.3.53-6.3.55).
func nassItem() SipAuthDataItem {
	return SipAuthDataItem{
		SipAuthenticationScheme:  diam.Some("NASS-Bundled"),
		SipAuthenticationContext: diam.Some([]byte("REGISTER")),
		FramedIpAddress:          diam.Some([]byte{192, 0, 2, 50}),
		FramedIpv6Prefix:         diam.Some([]byte{0, 64, 0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0}),
		FramedInterfaceId:        diam.Some(uint64(0x0200_5eff_fe00_5301)),
		LineIdentifier:           [][]byte{[]byte("line-1"), []byte("line-2")},
	}
}

// samples returns one message per Cx/Dx command, request then answer, in
// the order of table 6.1.1. Together they reach every grouped AVP that a
// command can carry.
func samples() []diam.Message {
	sar := restoration()
	return []diam.Message{
		&UserAuthorizationRequest{
			Header: hdr, SessionId: "icscf.ims.example.org;1;1", Drmp: diam.Some(DrmpPriority5), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: icscf, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			UserName: impi, OcSupportedFeatures: ocSF, SupportedFeatures: features, PublicIdentity: impu,
			VisitedNetworkIdentifier: []byte("visited.example.net"), UserAuthorizationType: diam.Some(UserAuthorizationTypeRegistrationAndCapabilities),
			UarFlags: diam.Some(uint32(1)), ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&UserAuthorizationAnswer{
			Header: hdr, SessionId: "icscf.ims.example.org;1;1", Drmp: diam.Some(DrmpPriority5), VendorSpecificApplicationId: vsai,
			ExperimentalResult: firstReg, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, OcSupportedFeatures: ocSF,
			OcOlr: ocOlr, Load: load, SupportedFeatures: features, ServerName: diam.Some(scscfURI), ServerCapabilities: capabilities,
			FailedAvp: failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&ServerAssignmentRequest{
			Header: hdr, SessionId: "scscf.ims.example.org;1;2", Drmp: diam.Some(DrmpPriority3), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: scscf, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			UserName: diam.Some(impi), OcSupportedFeatures: ocSF, SupportedFeatures: features, PublicIdentity: []string{impu, "tel:+15551234567"},
			WildcardedPublicIdentity: diam.Some("sip:chatroom-!.*!@ims.example.org"), ServerName: scscfURI,
			ServerAssignmentType: ServerAssignmentTypeRegistration, UserDataAlreadyAvailable: UserDataAlreadyAvailableUserDataNotAvailable,
			ScscfRestorationInfo: &sar, MultipleRegistrationIndication: diam.Some(MultipleRegistrationIndicationMultipleRegistration),
			SessionPriority: diam.Some(SessionPriorityPriority1), SarFlags: diam.Some(uint32(1)),
			FailedPcscf: &FailedPcscf{PcscfFqdn: diam.Some("pcscf.ims.example.org"),
				PcscfIpAddress: []netip.Addr{netip.MustParseAddr("192.0.2.40"), netip.MustParseAddr("2001:db8::40")}},
			ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&ServerAssignmentAnswer{
			Header: hdr, SessionId: "scscf.ims.example.org;1;2", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, UserName: diam.Some(impi), OcSupportedFeatures: ocSF,
			OcOlr: ocOlr, Load: load, SupportedFeatures: features, UserData: diam.Some(userData), ChargingInformation: charging,
			AssociatedIdentities: &AssociatedIdentities{UserName: []string{impi, "001010000000002@ims.example.org"}},
			LooseRouteIndication: diam.Some(LooseRouteIndicationLooseRouteRequired), ScscfRestorationInfo: []ScscfRestorationInfo{sar},
			AssociatedRegisteredIdentities: &AssociatedRegisteredIdentities{UserName: []string{impi}},
			ServerName:                     diam.Some(scscfURI), WildcardedPublicIdentity: diam.Some("sip:chatroom-!.*!@ims.example.org"),
			PriviledgedSenderIndication: diam.Some(PriviledgedSenderIndicationPriviledgedSender),
			AllowedWafWwsfIdentities:    allowedWaf, FailedAvp: failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&LocationInfoRequest{
			Header: hdr, SessionId: "icscf.ims.example.org;1;3", VendorSpecificApplicationId: vsai, AuthSessionState: nsm,
			OriginHost: icscf, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			OriginatingRequest: diam.Some(OriginatingRequestOriginating), OcSupportedFeatures: ocSF, SupportedFeatures: features,
			PublicIdentity: impu, UserAuthorizationType: diam.Some(UserAuthorizationTypeRegistration),
			SessionPriority: diam.Some(SessionPriorityPriority0), ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&LocationInfoAnswer{
			Header: hdr, SessionId: "icscf.ims.example.org;1;3", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, OcSupportedFeatures: ocSF, OcOlr: ocOlr, Load: load,
			SupportedFeatures: features, ServerName: diam.Some(scscfURI), ServerCapabilities: capabilities,
			WildcardedPublicIdentity: diam.Some("sip:chatroom-!.*!@ims.example.org"), LiaFlags: diam.Some(uint32(1)),
			FailedAvp: failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&MultimediaAuthRequest{
			Header: hdr, SessionId: "scscf.ims.example.org;1;4", VendorSpecificApplicationId: vsai, AuthSessionState: nsm,
			OriginHost: scscf, OriginRealm: realm, DestinationRealm: realm, DestinationHost: diam.Some(hss), UserName: impi,
			OcSupportedFeatures: ocSF, SupportedFeatures: features, PublicIdentity: impu,
			SipAuthDataItem: SipAuthDataItem{SipAuthenticationScheme: diam.Some(akaScheme),
				SipAuthorization: diam.Some(make([]byte, 30))}, // RAND || AUTS for resynchronisation (§6.3.11)
			SipNumberAuthItems: 1, ServerName: scscfURI, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&MultimediaAuthAnswer{
			Header: hdr, SessionId: "scscf.ims.example.org;1;4", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, UserName: diam.Some(impi), OcSupportedFeatures: ocSF,
			OcOlr: ocOlr, Load: load, SupportedFeatures: features, PublicIdentity: diam.Some(impu), SipNumberAuthItems: diam.Some(uint32(3)),
			SipAuthDataItem: []SipAuthDataItem{akaVector(1), digestItem(), nassItem()},
			FailedAvp:       failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&RegistrationTerminationRequest{
			Header: hdr, SessionId: "hss.ims.example.org;1;5", Drmp: diam.Some(DrmpPriority2), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, DestinationHost: diam.Some(scscf), DestinationRealm: realm,
			UserName: impi, AssociatedIdentities: &AssociatedIdentities{UserName: []string{"001010000000002@ims.example.org"}},
			SupportedFeatures: features, PublicIdentity: []string{impu},
			DeregistrationReason: DeregistrationReason{ReasonCode: ReasonCodeNewServerAssigned, ReasonInfo: diam.Some("moved")},
			RtrFlags:             diam.Some(uint32(1)), ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&RegistrationTerminationAnswer{
			Header: hdr, SessionId: "hss.ims.example.org;1;5", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: nsm, OriginHost: scscf, OriginRealm: realm,
			AssociatedIdentities:              &AssociatedIdentities{UserName: []string{"001010000000002@ims.example.org"}},
			SupportedFeatures:                 features,
			IdentityWithEmergencyRegistration: []IdentityWithEmergencyRegistration{{UserName: impi, PublicIdentity: "sip:sos@ims.example.org"}},
			FailedAvp:                         failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&PushProfileRequest{
			Header: hdr, SessionId: "hss.ims.example.org;1;6", VendorSpecificApplicationId: vsai, AuthSessionState: nsm,
			OriginHost: hss, OriginRealm: realm, DestinationHost: scscf, DestinationRealm: realm, UserName: impi,
			SupportedFeatures: features, UserData: diam.Some(userData), ChargingInformation: charging,
			SipAuthDataItem: &SipAuthDataItem{SipDigestAuthenticate: digestItem().SipDigestAuthenticate}, AllowedWafWwsfIdentities: allowedWaf,
			ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&PushProfileAnswer{
			Header: hdr, SessionId: "hss.ims.example.org;1;6", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: nsm, OriginHost: scscf, OriginRealm: realm, SupportedFeatures: features,
			FailedAvp: failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
	}
}
