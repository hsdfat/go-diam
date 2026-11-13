package sh

import (
	"time"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
)

const (
	as, hss, realm = "as.ims.example.org", "hss.ims.example.org", "ims.example.org"
	impi           = "001010000000001@ims.example.org"
	impu           = "sip:+15551234567@ims.example.org"
	asURI          = "sip:as.ims.example.org"
	wildPSI        = "sip:chatroom-!.*!@ims.example.org"
	wildIMPU       = "sip:+1555!.*!@ims.example.org"
)

var (
	hdr = diam.Header{HopByHop: 0x11, EndToEnd: 0x22}
	// §6: Auth-Application-Id 16777217 inside the 3GPP Vendor-Specific-Application-Id.
	vsai = base.VendorSpecificApplicationId{VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777217))}
	nsm  = base.AuthSessionStateNoStateMaintained
	// Times are whole seconds: the Time type has no finer resolution.
	expiry = time.Date(2026, 12, 13, 8, 0, 0, 0, time.UTC)

	proxyInfo = []base.ProxyInfo{{ProxyHost: "dra.ims.example.org", ProxyState: []byte{1, 2, 3}}}
	// Feature-List-ID 1 (table 7.1.1): Notif-Eff, Update-Eff, Update-Eff-Enhance.
	features  = []SupportedFeatures{{VendorId: 10415, FeatureListId: 1, FeatureList: 0x07}}
	failedAvp = &base.FailedAvp{Unknown: []diam.RawAVP{
		{Code: AVPUdrFlags, Vendor: 10415, Flags: diam.AVPFlagVendor, Data: []byte{1, 2}},
	}}
	// DIAMETER_ERROR_TRANSPARENT_DATA OUT_OF_SYNC (§6.2.2.7).
	outOfSync = &base.ExperimentalResult{VendorId: 10415, ExperimentalResultCode: 5105}
	ocSF      = &OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))}
	ocOlr     = &OcOlr{OcSequenceNumber: 9, OcReportType: OcReportTypeRealmReport,
		OcReductionPercentage: diam.Some(uint32(50)), OcValidityDuration: diam.Some(uint32(30))}
	load = []Load{{LoadType: diam.Some(LoadTypeHost), LoadValue: diam.Some(uint64(1000)), Sourceid: diam.Some(hss)}}

	// TBCD-encoded +15551234567 (§6.3.2).
	msisdn   = []byte{0x51, 0x55, 0x21, 0x43, 0x65, 0xf7}
	userData = []byte(`<?xml version="1.0" encoding="UTF-8"?><Sh-Data><RepositoryData><ServiceIndication>svc</ServiceIndication><SequenceNumber>3</SequenceNumber></RepositoryData></Sh-Data>`)
)

// samples returns one message per Sh/Dh command, request then answer, in
// the order of table 6.1.1. Together they carry every AVP of table 6.3.1
// that a command can carry (Supported-Applications is in none) and both
// wildcarded identities.
func samples() []diam.Message {
	return []diam.Message{
		&UserDataRequest{
			Header: hdr, SessionId: "as.ims.example.org;1;1", Drmp: diam.Some(DrmpPriority5), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: as, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			SupportedFeatures: features, UserIdentity: UserIdentity{PublicIdentity: diam.Some(impu)},
			WildcardedImpu: diam.Some(wildIMPU), ServerName: diam.Some(asURI),
			ServiceIndication: [][]byte{[]byte("svc"), []byte("svc2")},
			DataReference:     []DataReference{DataReferenceRepositoryData, DataReferenceLocationInformation, DataReferenceCsrn},
			IdentitySet:       []IdentitySet{IdentitySetRegisteredIdentities, IdentitySetAliasIdentities},
			RequestedDomain:   diam.Some(RequestedDomainPsDomain), CurrentLocation: diam.Some(CurrentLocationInitiateActiveLocationRetrieval),
			DsaiTag: [][]byte{[]byte("dsai-1")}, SessionPriority: diam.Some(SessionPriorityPriority1), UserName: diam.Some(impi),
			RequestedNodes:          diam.Some(uint32(0x09)), // MME and AMF (table 6.3.7A/1)
			ServingNodeIndication:   diam.Some(ServingNodeIndicationOnlyServingNodesRequired),
			PrePagingSupported:      diam.Some(PrePagingSupportedPrepagingSupported),
			LocalTimeZoneIndication: diam.Some(LocalTimeZoneIndicationLocalTimeZoneWithLocationInfoRequested),
			UdrFlags:                diam.Some(uint32(0x03)), // table 6.3.28/1
			CallReferenceInfo:       &CallReferenceInfo{CallReferenceNumber: []byte{0x01, 0x02, 0x03}, AsNumber: []byte{0x91, 0x51, 0x55}},
			OcSupportedFeatures:     ocSF, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&UserDataAnswer{
			Header: hdr, SessionId: "as.ims.example.org;1;1", Drmp: diam.Some(DrmpPriority5), VendorSpecificApplicationId: vsai,
			ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			SupportedFeatures: features, WildcardedPublicIdentity: diam.Some(wildPSI), WildcardedImpu: diam.Some(wildIMPU),
			UserData: diam.Some(userData), OcSupportedFeatures: ocSF, OcOlr: ocOlr, Load: load,
			FailedAvp: failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&ProfileUpdateRequest{
			Header: hdr, SessionId: "as.ims.example.org;1;2", Drmp: diam.Some(DrmpPriority3), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: as, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			SupportedFeatures: features, UserIdentity: UserIdentity{Msisdn: diam.Some(msisdn)}, WildcardedImpu: diam.Some(wildIMPU),
			UserName: diam.Some(impi), DataReference: []DataReference{DataReferenceRepositoryData, DataReferencePsiActivation},
			UserData: userData, OcSupportedFeatures: ocSF, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&ProfileUpdateAnswer{
			Header: hdr, SessionId: "as.ims.example.org;1;2", VendorSpecificApplicationId: vsai, ExperimentalResult: outOfSync,
			AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, WildcardedPublicIdentity: diam.Some(wildPSI),
			WildcardedImpu:    diam.Some(wildIMPU),
			RepositoryDataId:  &RepositoryDataId{ServiceIndication: []byte("svc"), SequenceNumber: 3},
			DataReference:     diam.Some(DataReferenceRepositoryData),
			SupportedFeatures: features, OcSupportedFeatures: ocSF, OcOlr: ocOlr, Load: load,
			FailedAvp: failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&SubscribeNotificationsRequest{
			Header: hdr, SessionId: "as.ims.example.org;1;3", Drmp: diam.Some(DrmpPriority2), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: as, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			SupportedFeatures: features, UserIdentity: UserIdentity{ExternalIdentifier: diam.Some("device-1@iot.example.org")},
			WildcardedPublicIdentity: diam.Some(wildPSI), ServiceIndication: [][]byte{[]byte("svc")},
			SendDataIndication: diam.Some(SendDataIndicationUserDataRequested), ServerName: diam.Some(asURI),
			SubsReqType: SubsReqTypeSubscribe, DataReference: []DataReference{DataReferenceUeReachabilityForIp, DataReferenceDsai},
			IdentitySet: []IdentitySet{IdentitySetImplicitIdentities}, ExpiryTime: diam.Some(expiry),
			DsaiTag: [][]byte{[]byte("dsai-1"), []byte("dsai-2")}, OneTimeNotification: diam.Some(OneTimeNotificationOneTimeNotificationRequested),
			UserName: diam.Some(impi), OcSupportedFeatures: ocSF, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&SubscribeNotificationsAnswer{
			Header: hdr, SessionId: "as.ims.example.org;1;3", VendorSpecificApplicationId: vsai, AuthSessionState: nsm,
			ResultCode: diam.Some(uint32(2001)), OriginHost: hss, OriginRealm: realm, WildcardedPublicIdentity: diam.Some(wildPSI),
			SupportedFeatures: features, UserData: diam.Some(userData), ExpiryTime: diam.Some(expiry),
			OcSupportedFeatures: ocSF, OcOlr: ocOlr, Load: load, FailedAvp: failedAvp, ProxyInfo: proxyInfo,
			RouteRecord: []string{"dra.ims.example.org"},
		},
		&PushNotificationRequest{
			Header: hdr, SessionId: "hss.ims.example.org;1;4", Drmp: diam.Some(DrmpPriority4), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, DestinationHost: as, DestinationRealm: realm,
			SupportedFeatures: features, UserIdentity: UserIdentity{PublicIdentity: diam.Some(impu)}, WildcardedImpu: diam.Some(wildIMPU),
			UserName: diam.Some(impi), UserData: userData, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&PushNotificationAnswer{
			Header: hdr, SessionId: "hss.ims.example.org;1;4", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: nsm, OriginHost: as, OriginRealm: realm, SupportedFeatures: features,
			FailedAvp: failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
	}
}
