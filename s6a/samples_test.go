package s6a

import (
	"net/netip"
	"time"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
)

const (
	mme, hss, realm = "mme.example.org", "hss.example.org", "example.org"
	imsi            = "001010000000001"
)

var (
	hdr  = diam.Header{HopByHop: 0x11, EndToEnd: 0x22}
	plmn = []byte{0x00, 0xf1, 0x10} // MCC 001, MNC 01
	// Times are whole seconds: the Time type has no finer resolution.
	when = time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)

	proxyInfo = []base.ProxyInfo{{ProxyHost: "dra.example.org", ProxyState: []byte{1, 2, 3}}}
	features  = []SupportedFeatures{{VendorId: 10415, FeatureListId: 1, FeatureList: 0x0bff}}
	failedAvp = &base.FailedAvp{Unknown: []diam.RawAVP{
		{Code: AVPUlrFlags, Vendor: 10415, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte{1, 2}},
	}}
	expResult = &base.ExperimentalResult{VendorId: 10415, ExperimentalResultCode: 5420} // DIAMETER_ERROR_UNKNOWN_EPS_SUBSCRIPTION
	ocOlr     = &OcOlr{OcSequenceNumber: 9, OcReportType: OcReportTypeRealmReport,
		OcReductionPercentage: diam.Some(uint32(50)), OcValidityDuration: diam.Some(uint32(30))}
	load = []Load{{LoadType: diam.Some(LoadTypeHost), LoadValue: diam.Some(uint64(1000)), Sourceid: diam.Some(hss)}}
)

func pgw() *Mip6AgentInfo {
	return &Mip6AgentInfo{
		MipHomeAgentAddress: []netip.Addr{netip.MustParseAddr("192.0.2.10"), netip.MustParseAddr("2001:db8::10")},
		MipHomeAgentHost:    &MipHomeAgentHost{DestinationRealm: realm, DestinationHost: "pgw.example.org"},
	}
}

func epsLocation() *EpsLocationInformation {
	return &EpsLocationInformation{
		MmeLocationInformation: &MmeLocationInformation{
			EUtranCellGlobalIdentity: diam.Some([]byte{0x00, 0xf1, 0x10, 0x00, 0x00, 0x01, 0x01}),
			TrackingAreaIdentity:     diam.Some([]byte{0x00, 0xf1, 0x10, 0x00, 0x01}),
			CurrentLocationRetrieved: diam.Some(CurrentLocationRetrievedActiveLocationRetrieval),
			AgeOfLocationInformation: diam.Some(uint32(0)),
			UserCsgInformation:       &UserCsgInformation{CsgId: 7, CsgAccessMode: CsgAccessModeHybridMode, CsgMembershipIndication: diam.Some(CsgMembershipIndicationCsgMember)},
			EnodebId:                 diam.Some([]byte{0x00, 0xf1, 0x10, 0x00, 0x00, 0x01}),
		},
		SgsnLocationInformation: &SgsnLocationInformation{
			CellGlobalIdentity:  diam.Some([]byte{0x00, 0xf1, 0x10, 0x00, 0x01, 0x00, 0x02}),
			RoutingAreaIdentity: diam.Some([]byte{0x00, 0xf1, 0x10, 0x00, 0x01, 0x01}),
		},
	}
}

// subscriptionData is a Subscription-Data that reaches most of the grouped
// AVPs of §7.3: APN configuration with QoS and PGW identity, GPRS data,
// trace with MDT, LCS, CSG, monitoring, V2X, eDRX.
func subscriptionData() SubscriptionData {
	ambr := &Ambr{MaxRequestedBandwidthUl: 50_000_000, MaxRequestedBandwidthDl: 100_000_000,
		ExtendedMaxRequestedBwUl: diam.Some(uint32(5_000_000)), ExtendedMaxRequestedBwDl: diam.Some(uint32(10_000_000))}
	return SubscriptionData{
		SubscriberStatus:             diam.Some(SubscriberStatusServiceGranted),
		Msisdn:                       diam.Some([]byte{0x21, 0x43, 0x65, 0x87}),
		AMsisdn:                      diam.Some([]byte{0x21, 0x43, 0x65, 0x88}),
		StnSr:                        diam.Some([]byte{0x91, 0x21}),
		IcsIndicator:                 diam.Some(IcsIndicatorTrue),
		NetworkAccessMode:            diam.Some(NetworkAccessModeOnlyPacket),
		OperatorDeterminedBarring:    diam.Some(uint32(0)),
		HplmnOdb:                     diam.Some(uint32(1)),
		RegionalSubscriptionZoneCode: [][]byte{{0x00, 0x01}, {0x00, 0x02}},
		AccessRestrictionData:        diam.Some(uint32(0x20)),
		ApnOiReplacement:             diam.Some("mnc001.mcc001.gprs"),
		LcsInfo: &LcsInfo{
			GmlcNumber: [][]byte{{0x91, 0x11}},
			LcsPrivacyexception: []LcsPrivacyexception{{
				SsCode: []byte{0x11}, SsStatus: []byte{0x05},
				NotificationToUeUser: diam.Some(NotificationToUeUserLocationNotAllowed),
				ExternalClient:       []ExternalClient{{ClientIdentity: []byte{0x91, 0x22}, GmlcRestriction: diam.Some(GmlcRestrictionHomeCountry)}},
				PlmnClient:           []PlmnClient{PlmnClientOAndMHplmn},
				ServiceType:          []ServiceType{{Servicetypeidentity: 3}},
			}},
			MoLr: []MoLr{{SsCode: []byte{0x12}, SsStatus: []byte{0x05}}},
		},
		TeleserviceList:             &TeleserviceList{TsCode: [][]byte{{0x21}, {0x22}}},
		CallBarringInfo:             []CallBarringInfo{{SsCode: []byte{0x92}, SsStatus: []byte{0x01}}},
		TgppChargingCharacteristics: diam.Some("0800"),
		Ambr:                        ambr,
		ApnConfigurationProfile: &ApnConfigurationProfile{
			ContextIdentifier:                     1,
			AdditionalContextId:                   diam.Some(uint32(2)),
			AllApnConfigurationsIncludedIndicator: AllApnConfigurationsIncludedIndicatorAllApnConfigurationsIncluded,
			ApnConfiguration: []ApnConfiguration{
				{
					ContextIdentifier:    1,
					ServedPartyIpAddress: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
					PdnType:              PdnTypeIpv4v6,
					ServiceSelection:     "internet",
					EpsSubscribedQosProfile: &EpsSubscribedQosProfile{
						QosClassIdentifier: QosClassIdentifierQci9,
						AllocationRetentionPriority: AllocationRetentionPriority{
							PriorityLevel:           8,
							PreEmptionCapability:    diam.Some(PreEmptionCapabilityPreEmptionCapabilityDisabled),
							PreEmptionVulnerability: diam.Some(PreEmptionVulnerabilityPreEmptionVulnerabilityEnabled),
						},
					},
					VplmnDynamicAddressAllowed:  diam.Some(VplmnDynamicAddressAllowedAllowed),
					Mip6AgentInfo:               pgw(),
					PdnGwAllocationType:         diam.Some(PdnGwAllocationTypeDynamic),
					TgppChargingCharacteristics: diam.Some("0800"),
					Ambr:                        ambr,
					SpecificApnInfo:             []SpecificApnInfo{{ServiceSelection: "corp", Mip6AgentInfo: *pgw()}},
					SiptoPermission:             diam.Some(SiptoPermissionSiptoAboveRanNotallowed),
					LipaPermission:              diam.Some(LipaPermissionLipaProhibited),
					WlanOffloadability:          &WlanOffloadability{WlanOffloadabilityEutran: diam.Some(uint32(1))},
					Interworking5GsIndicator:    diam.Some(Interworking5GsIndicatorSubscribed),
				},
				{
					ContextIdentifier:     2,
					PdnType:               PdnTypeIpv4,
					ServiceSelection:      "ims",
					NonIpPdnTypeIndicator: diam.Some(NonIpPdnTypeIndicatorFalse),
					RdsIndicator:          diam.Some(RdsIndicatorDisabled),
				},
			},
		},
		RatFrequencySelectionPriorityId: diam.Some(uint32(5)),
		TraceData: &TraceData{
			TraceReference:        []byte{0x00, 0xf1, 0x10, 0x00, 0x00, 0x01},
			TraceDepth:            1,
			TraceNeTypeList:       []byte{0x01},
			TraceEventList:        []byte{0x01, 0x02},
			TraceCollectionEntity: netip.MustParseAddr("192.0.2.50"),
			MdtConfiguration: &MdtConfiguration{
				JobType:   0,
				AreaScope: &AreaScope{TrackingAreaIdentity: [][]byte{{0x00, 0xf1, 0x10, 0x00, 0x01}}},
				MbsfnArea: []MbsfnArea{{MbsfnAreaId: diam.Some(uint32(1)), CarrierFrequency: diam.Some(uint32(3000))}},
			},
		},
		GprsSubscriptionData: &GprsSubscriptionData{
			CompleteDataListIncludedIndicator: CompleteDataListIncludedIndicatorAllPdpContextsIncluded,
			PdpContext: []PdpContext{{
				ContextIdentifier: 1, PdpType: []byte{0x01, 0x21}, PdpAddress: diam.Some(netip.MustParseAddr("10.0.0.2")),
				QosSubscribed: []byte{0x0b, 0x92, 0x1f}, ServiceSelection: "internet",
			}},
		},
		CsgSubscriptionData:                      []CsgSubscriptionData{{CsgId: 7, ExpirationDate: diam.Some(when), ServiceSelection: []string{"internet"}, VisitedPlmnId: diam.Some(plmn)}},
		RoamingRestrictedDueToUnsupportedFeature: diam.Some(RoamingRestrictedDueToUnsupportedFeatureRoamingRestrictedDueToUnsupportedFeature),
		SubscribedPeriodicRauTauTimer:            diam.Some(uint32(54)),
		MpsPriority:                              diam.Some(uint32(1)),
		VplmnLipaAllowed:                         diam.Some(VplmnLipaAllowedLipaNotallowed),
		RelayNodeIndicator:                       diam.Some(RelayNodeIndicatorNotRelayNode),
		MdtUserConsent:                           diam.Some(MdtUserConsentConsentGiven),
		SubscribedVsrvcc:                         diam.Some(SubscribedVsrvccVsrvccSubscribed),
		ProseSubscriptionData:                    &ProseSubscriptionData{ProsePermission: 1},
		SubscriptionDataFlags:                    diam.Some(uint32(0x1)),
		AdjacentAccessRestrictionData:            []AdjacentAccessRestrictionData{{VisitedPlmnId: plmn, AccessRestrictionData: 0x10}},
		DlBufferingSuggestedPacketCount:          diam.Some(int32(-1)),
		ImsiGroupId:                              []ImsiGroupId{{GroupServiceId: 1, GroupPlmnId: plmn, LocalGroupId: []byte{0x01}}},
		UeUsageType:                              diam.Some(uint32(3)),
		AeseCommunicationPattern: []AeseCommunicationPattern{{
			ScefReferenceId: diam.Some(uint32(10)), ScefId: "scef.example.org",
			CommunicationPatternSet: []CommunicationPatternSet{{
				PeriodicCommunicationIndicator: diam.Some(uint32(0)), PeriodicTime: diam.Some(uint32(3600)),
				ScheduledCommunicationTime: []ScheduledCommunicationTime{{DayOfWeekMask: diam.Some(uint32(0x3e)), TimeOfDayStart: diam.Some(uint32(28800)), TimeOfDayEnd: diam.Some(uint32(64800))}},
				ReferenceIdValidityTime:    diam.Some(when),
			}},
			MtcProviderInfo: &MtcProviderInfo{MtcProviderId: diam.Some("provider-1")},
		}},
		MonitoringEventConfiguration: []MonitoringEventConfiguration{{
			ScefReferenceId: diam.Some(uint32(11)), ScefId: "scef.example.org", MonitoringType: 1,
			MaximumNumberOfReports: diam.Some(uint32(1)), MonitoringDuration: diam.Some(when),
			UeReachabilityConfiguration:        &UeReachabilityConfiguration{ReachabilityType: diam.Some(uint32(1)), MaximumResponseTime: diam.Some(uint32(60))},
			LocationInformationConfiguration:   &LocationInformationConfiguration{MonteLocationType: diam.Some(uint32(0)), Accuracy: diam.Some(uint32(1))},
			ScefRealm:                          diam.Some(realm),
			PdnConnectivityStatusConfiguration: &PdnConnectivityStatusConfiguration{ServiceSelection: diam.Some("internet")},
		}},
		EmergencyInfo:          &EmergencyInfo{Mip6AgentInfo: pgw()},
		V2XSubscriptionData:    &V2XSubscriptionData{V2XPermission: diam.Some(uint32(1)), UePc5Ambr: diam.Some(uint32(1000))},
		V2XSubscriptionDataNr:  &V2XSubscriptionDataNr{UePc5Qos: &UePc5Qos{Pc5QosFlow: []Pc5QosFlow{{FiveQi: 23, Pc5FlowBitrates: &Pc5FlowBitrates{GuaranteedFlowBitrates: diam.Some(int32(1000))}}}}},
		EdrxCycleLength:        []EdrxCycleLength{{RatType: RatTypeEutran, EdrxCycleLengthValue: []byte{0x05}}},
		ExternalIdentifier:     diam.Some("ue@example.org"),
		ActiveTime:             diam.Some(uint32(10)),
		PagingTimeWindow:       []PagingTimeWindow{{OperationMode: 1, PagingTimeWindowLength: []byte{0x03}}},
		IabOperationPermission: diam.Some(IabOperationPermissionIabOperationNotallowed),
	}
}

// samples returns one realistic message per S6a/S6d command, requests and
// answers alternating, in the order of TS 29.272 table 7.2.2/1.
func samples() []diam.Message {
	sd := subscriptionData()
	return []diam.Message{
		&UpdateLocationRequest{
			Header: hdr, SessionId: "mme.example.org;1;1", Drmp: diam.Some(DrmpPriority5),
			AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: mme, OriginRealm: realm,
			DestinationHost: diam.Some(hss), DestinationRealm: realm, UserName: imsi,
			OcSupportedFeatures: &OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))},
			SupportedFeatures:   features,
			TerminalInformation: &TerminalInformation{Imei: diam.Some("3534900698733"), SoftwareVersion: diam.Some("01")},
			RatType:             RatTypeEutran, UlrFlags: 0x22, UeSrvccCapability: diam.Some(UeSrvccCapabilityUeSrvccSupported),
			VisitedPlmnId: plmn, SgsnNumber: diam.Some([]byte{0x91, 0x33}),
			HomogeneousSupportOfImsVoiceOverPsSessions: diam.Some(HomogeneousSupportOfImsVoiceOverPsSessionsSupported),
			GmlcAddress:        diam.Some(netip.MustParseAddr("192.0.2.30")),
			ActiveApn:          []ActiveApn{{ContextIdentifier: 1, ServiceSelection: diam.Some("internet"), Mip6AgentInfo: pgw()}},
			EquivalentPlmnList: &EquivalentPlmnList{VisitedPlmnId: [][]byte{plmn, {0x00, 0xf1, 0x20}}},
			MmeNumberForMtSms:  diam.Some([]byte{0x91, 0x44}), SmsRegisterRequest: diam.Some(SmsRegisterRequestNoPreference),
			SgsMmeIdentity:        diam.Some("mmec01.mmegi0001.mme.epc.mnc001.mcc001.3gppnetwork.org"),
			CoupledNodeDiameterId: diam.Some("sgsn.example.org"), AdjacentPlmns: &AdjacentPlmns{VisitedPlmnId: [][]byte{plmn}},
			SupportedServices: &SupportedServices{SupportedMonitoringEvents: diam.Some(uint64(0x7f))},
			ProxyInfo:         proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&UpdateLocationAnswer{
			Header: hdr, SessionId: "mme.example.org;1;1", ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: hss, OriginRealm: realm,
			OcSupportedFeatures: &OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))}, OcOlr: ocOlr, Load: load,
			SupportedFeatures: features, UlaFlags: diam.Some(uint32(1)), SubscriptionData: &sd,
			ResetId: [][]byte{{0xab}}, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&CancelLocationRequest{
			Header: hdr, SessionId: "hss.example.org;1;2", AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost: hss, OriginRealm: realm, DestinationHost: mme, DestinationRealm: realm, UserName: imsi,
			SupportedFeatures: features, CancellationType: CancellationTypeSubscriptionWithdrawal, ClrFlags: diam.Some(uint32(2)),
			ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&CancelLocationAnswer{
			Header: hdr, SessionId: "hss.example.org;1;2", SupportedFeatures: features, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: mme, OriginRealm: realm,
			FailedAvp: failedAvp, ProxyInfo: proxyInfo,
		},
		&AuthenticationInformationRequest{
			Header: hdr, SessionId: "mme.example.org;1;3", AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost: mme, OriginRealm: realm, DestinationRealm: realm, UserName: imsi, SupportedFeatures: features,
			RequestedEutranAuthenticationInfo: &RequestedEutranAuthenticationInfo{
				NumberOfRequestedVectors: diam.Some(uint32(2)), ImmediateResponsePreferred: diam.Some(uint32(1)),
				ReSynchronizationInfo: diam.Some(make([]byte, 30)),
			},
			RequestedUtranGeranAuthenticationInfo: &RequestedUtranGeranAuthenticationInfo{NumberOfRequestedVectors: diam.Some(uint32(1))},
			VisitedPlmnId:                         plmn, AirFlags: diam.Some(uint32(1)), ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&AuthenticationInformationAnswer{
			Header: hdr, SessionId: "mme.example.org;1;3", ExperimentalResult: expResult,
			ErrorDiagnostic: diam.Some(ErrorDiagnosticNoGprsDataSubscribed), AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost: hss, OriginRealm: realm, OcOlr: ocOlr, Load: load, SupportedFeatures: features,
			AuthenticationInfo: &AuthenticationInfo{
				EUtranVector: []EUtranVector{{ItemNumber: diam.Some(uint32(1)), Rand: make([]byte, 16), Xres: make([]byte, 8), Autn: make([]byte, 16), Kasme: make([]byte, 32)}},
				UtranVector:  []UtranVector{{Rand: make([]byte, 16), Xres: make([]byte, 8), Autn: make([]byte, 16), ConfidentialityKey: make([]byte, 16), IntegrityKey: make([]byte, 16)}},
				GeranVector:  []GeranVector{{Rand: make([]byte, 16), Sres: make([]byte, 4), Kc: make([]byte, 8)}},
			},
			UeUsageType: diam.Some(uint32(3)), FailedAvp: failedAvp, ProxyInfo: proxyInfo,
		},
		&InsertSubscriberDataRequest{
			Header: hdr, SessionId: "hss.example.org;1;4", AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost: hss, OriginRealm: realm, DestinationHost: mme, DestinationRealm: realm, UserName: imsi,
			SupportedFeatures: features, SubscriptionData: sd, IdrFlags: diam.Some(uint32(0x10)), ResetId: [][]byte{{0xab}},
			ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&InsertSubscriberDataAnswer{
			Header: hdr, SessionId: "hss.example.org;1;4", SupportedFeatures: features, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: mme, OriginRealm: realm,
			ImsVoiceOverPsSessionsSupported: diam.Some(ImsVoiceOverPsSessionsSupportedSupported),
			LastUeActivityTime:              diam.Some(when), RatType: diam.Some(RatTypeEutran), IdaFlags: diam.Some(uint32(1)),
			EpsUserState:           &EpsUserState{MmeUserState: &MmeUserState{UserState: diam.Some(UserStateConnectedReachableForPaging)}},
			EpsLocationInformation: epsLocation(),
			LocalTimeZone:          &LocalTimeZone{TimeZone: "+04", DaylightSavingTime: DaylightSavingTimeNoAdjustment},
			SupportedServices:      &SupportedServices{SupportedMonitoringEvents: diam.Some(uint64(3))},
			MonitoringEventReport: []MonitoringEventReport{{
				ScefReferenceId: 11, ScefId: diam.Some("scef.example.org"), ReachabilityInformation: diam.Some(uint32(1)),
				EpsLocationInformation: epsLocation(), MonitoringType: diam.Some(uint32(1)),
				IdleStatusIndication: &IdleStatusIndication{IdleStatusTimestamp: diam.Some(when), ActiveTime: diam.Some(uint32(10)),
					EdrxCycleLength: &EdrxCycleLength{RatType: RatTypeEutran, EdrxCycleLengthValue: []byte{0x05}}},
				MaximumUeAvailabilityTime: diam.Some(when),
				PdnConnectivityStatusReport: []PdnConnectivityStatusReport{{ServiceSelection: "internet", PdnConnectivityStatusType: 0,
					PdnType: diam.Some(PdnTypeIpv4), ServedPartyIpAddress: []netip.Addr{netip.MustParseAddr("10.0.0.1")}}},
			}},
			MonitoringEventConfigStatus: []MonitoringEventConfigStatus{{
				ServiceReport:   []ServiceReport{{ServiceResult: &ServiceResult{VendorId: diam.Some(uint32(10415)), ServiceResultCode: diam.Some(uint32(5513))}, NodeType: diam.Some(uint32(0))}},
				ScefReferenceId: 11, ScefId: diam.Some("scef.example.org"),
			}},
			FailedAvp: failedAvp, ProxyInfo: proxyInfo,
		},
		&DeleteSubscriberDataRequest{
			Header: hdr, SessionId: "hss.example.org;1;5", AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost: hss, OriginRealm: realm, DestinationHost: mme, DestinationRealm: realm, UserName: imsi,
			SupportedFeatures: features, DsrFlags: 0x1, ScefId: diam.Some("scef.example.org"), ContextIdentifier: []uint32{1, 2},
			TraceReference: diam.Some([]byte{0x00, 0xf1, 0x10, 0x00, 0x00, 0x01}), TsCode: [][]byte{{0x21}}, SsCode: [][]byte{{0x11}},
			EdrxRelatedRat: &EdrxRelatedRat{RatType: []RatType{RatTypeEutran, RatTypeUtran}}, ExternalIdentifier: []string{"ue@example.org"},
			ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&DeleteSubscriberDataAnswer{
			Header: hdr, SessionId: "hss.example.org;1;5", SupportedFeatures: features, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: mme, OriginRealm: realm,
			DsaFlags: diam.Some(uint32(1)), FailedAvp: failedAvp, ProxyInfo: proxyInfo,
		},
		&PurgeUeRequest{
			Header: hdr, SessionId: "mme.example.org;1;6", AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost: mme, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm, UserName: imsi,
			OcSupportedFeatures: &OcSupportedFeatures{}, PurFlags: diam.Some(uint32(1)), SupportedFeatures: features,
			EpsLocationInformation: epsLocation(), ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&PurgeUeAnswer{
			Header: hdr, SessionId: "mme.example.org;1;6", SupportedFeatures: features, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: hss, OriginRealm: realm,
			OcOlr: ocOlr, Load: load, PuaFlags: diam.Some(uint32(1)), FailedAvp: failedAvp, ProxyInfo: proxyInfo,
		},
		&ResetRequest{
			Header: hdr, SessionId: "hss.example.org;1;7", AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost: hss, OriginRealm: realm, DestinationHost: mme, DestinationRealm: realm, SupportedFeatures: features,
			UserId: []string{"00101"}, ResetId: [][]byte{{0xab}},
			SubscriptionData:         &SubscriptionData{AccessRestrictionData: diam.Some(uint32(0))},
			SubscriptionDataDeletion: &SubscriptionDataDeletion{DsrFlags: 0x1, ContextIdentifier: []uint32{3}},
			ProxyInfo:                proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&ResetAnswer{
			Header: hdr, SessionId: "hss.example.org;1;7", SupportedFeatures: features, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: mme, OriginRealm: realm,
			FailedAvp: failedAvp, ProxyInfo: proxyInfo,
		},
		&NotifyRequest{
			Header: hdr, SessionId: "mme.example.org;1;8", Drmp: diam.Some(DrmpPriority0),
			AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: mme, OriginRealm: realm,
			DestinationHost: diam.Some(hss), DestinationRealm: realm, UserName: imsi, SupportedFeatures: features,
			TerminalInformation: &TerminalInformation{Imei: diam.Some("3534900698733")}, Mip6AgentInfo: pgw(),
			VisitedNetworkIdentifier: diam.Some([]byte("mnc001.mcc001.3gppnetwork.org")), ContextIdentifier: diam.Some(uint32(1)),
			ServiceSelection: diam.Some("internet"), AlertReason: diam.Some(AlertReasonUeMemoryAvailable),
			UeSrvccCapability: diam.Some(UeSrvccCapabilityUeSrvccNotSupported), NorFlags: diam.Some(uint32(0x1)),
			MaximumUeAvailabilityTime:   diam.Some(when),
			MonitoringEventConfigStatus: []MonitoringEventConfigStatus{{ScefReferenceId: 11}},
			EmergencyServices:           diam.Some(uint32(1)), ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&NotifyAnswer{
			Header: hdr, SessionId: "mme.example.org;1;8", ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: hss, OriginRealm: realm,
			OcSupportedFeatures: &OcSupportedFeatures{}, OcOlr: ocOlr, Load: load, SupportedFeatures: features,
			FailedAvp: failedAvp, ProxyInfo: proxyInfo,
		},
	}
}
