package s6t

import (
	"net/netip"
	"time"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
)

const (
	scef, hss, realm = "scef.example.org", "hss.example.org", "example.org"
	imsi             = "001010000000001"
	extID            = "device-1@iot.example.org"
	groupID          = "group-1@iot.example.org"
	apn              = "iot.apn"
)

var (
	hdr = diam.Header{HopByHop: 0x11, EndToEnd: 0x22}
	nsm = base.AuthSessionStateNoStateMaintained
	// §8.1.7: the 3GPP vendor and the S6t application id. Not sent by S6t
	// nodes (§8.2.2), but accepted when an intermediate agent adds it.
	vsai = &base.VendorSpecificApplicationId{VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777345))}
	// Times are whole seconds: the Time type has no finer resolution.
	expiry  = time.Date(2026, 12, 13, 8, 0, 0, 0, time.UTC)
	granted = time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	stamp   = time.Date(2026, 11, 14, 9, 30, 0, 0, time.UTC)

	proxyInfo = []base.ProxyInfo{{ProxyHost: "dra.example.org", ProxyState: []byte{1, 2, 3}}}
	// Feature-List-ID 1 (table 8.4.23-1): MONTE, AESE-Communication-Pattern,
	// NIDD-Authorization, Extended Reference IDs.
	features  = []SupportedFeatures{{VendorId: 10415, FeatureListId: 1, FeatureList: 0x207}}
	failedAvp = &base.FailedAvp{Unknown: []diam.RawAVP{
		{Code: AVPCirFlags, Vendor: 10415, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte{1, 2}},
	}}
	// DIAMETER_ERROR_UNAUTHORIZED_REQUESTING_ENTITY (§8.3.3.2).
	unauthorized = &base.ExperimentalResult{VendorId: 10415, ExperimentalResultCode: 5510}
	ocSF         = &OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))}
	ocOlr        = &OcOlr{OcSequenceNumber: 9, OcReportType: OcReportTypeHostReport,
		OcReductionPercentage: diam.Some(uint32(50)), OcValidityDuration: diam.Some(uint32(30))}
	load = []Load{{LoadType: diam.Some(LoadTypeHost), LoadValue: diam.Some(uint64(1000)), Sourceid: diam.Some(hss)}}

	// TBCD-encoded +15551234567 (TS 29.329 §6.3.2).
	msisdn = []byte{0x51, 0x55, 0x21, 0x43, 0x65, 0xf7}
	// MCC 001, MNC 01 (TS 29.272 §7.3.9).
	plmn = []byte{0x00, 0xf1, 0x10}

	mtcProvider = &MtcProviderInfo{MtcProviderId: diam.Some("provider-1")}
	userCsg     = &UserCsgInformation{CsgId: 7, CsgAccessMode: CsgAccessModeHybridMode,
		CsgMembershipIndication: diam.Some(CsgMembershipIndicationCsgMember)}
	mmeLocation = &EpsLocationInformation{MmeLocationInformation: &MmeLocationInformation{
		EUtranCellGlobalIdentity: diam.Some([]byte{0x00, 0xf1, 0x10, 0x00, 0x01, 0x02, 0x03}),
		TrackingAreaIdentity:     diam.Some([]byte{0x00, 0xf1, 0x10, 0x00, 0x01}),
		CurrentLocationRetrieved: diam.Some(CurrentLocationRetrievedActiveLocationRetrieval),
		AgeOfLocationInformation: diam.Some(uint32(3)), UserCsgInformation: userCsg,
		EnodebId: diam.Some([]byte{0x00, 0x01, 0x02}), ExtendedEnodebId: diam.Some([]byte{0x00, 0x01, 0x02, 0x03}),
	}}
	sgsnLocation = &EpsLocationInformation{SgsnLocationInformation: &SgsnLocationInformation{
		CellGlobalIdentity:       diam.Some([]byte{0x00, 0xf1, 0x10, 0x00, 0x01, 0x00, 0x02}),
		ServiceAreaIdentity:      diam.Some([]byte{0x00, 0xf1, 0x10, 0x00, 0x01, 0x00, 0x03}),
		RoutingAreaIdentity:      diam.Some([]byte{0x00, 0xf1, 0x10, 0x00, 0x01, 0x04}),
		CurrentLocationRetrieved: diam.Some(CurrentLocationRetrievedActiveLocationRetrieval),
		AgeOfLocationInformation: diam.Some(uint32(0)), UserCsgInformation: userCsg,
	}}
	idle = &IdleStatusIndication{
		IdleStatusTimestamp: diam.Some(stamp), ActiveTime: diam.Some(uint32(60)),
		SubscribedPeriodicRauTauTimer:   diam.Some(uint32(3240)),
		EdrxCycleLength:                 &EdrxCycleLength{RatType: RatTypeEutranNbIot, EdrxCycleLengthValue: []byte{0x05}},
		DlBufferingSuggestedPacketCount: diam.Some(int32(2)),
	}
	pdnReport = []PdnConnectivityStatusReport{
		{ServiceSelection: apn, PdnConnectivityStatusType: 0, PdnType: diam.Some(PdnTypeIpv4v6),
			ServedPartyIpAddress: []netip.Addr{netip.MustParseAddr("192.0.2.10"), netip.MustParseAddr("2001:db8::")}},
		{ServiceSelection: "nidd.apn", PdnConnectivityStatusType: 1,
			NonIpPdnTypeIndicator: diam.Some(NonIpPdnTypeIndicatorTrue), NonIpDataDeliveryMechanism: diam.Some(uint32(1))},
	}
	serviceReport = []ServiceReport{{
		ServiceResult: &ServiceResult{VendorId: diam.Some(uint32(10415)), ServiceResultCode: diam.Some(uint32(5512))},
		NodeType:      diam.Some(uint32(1)), // MME (§8.4.48)
	}}
	updatedNetwork = &UpdatedNetworkConfiguration{
		ScefId: scef, ScefReferenceId: diam.Some(uint32(41)), ScefReferenceIdExt: diam.Some(uint64(1) << 40),
		ScefReferenceIdForDeletion: []uint32{40}, ScefReferenceIdForDeletionExt: []uint64{1<<40 - 1},
		SubscribedPeriodicRauTauTimer: diam.Some(uint32(3240)), ActiveTime: diam.Some(uint32(60)),
		DlBufferingSuggestedPacketCount: diam.Some(int32(-1)),
	}
	suggestedNetwork = &SuggestedNetworkConfiguration{
		ScefReferenceId: 41, ScefReferenceIdExt: diam.Some(uint64(1) << 40), ScefId: scef,
		SubscribedPeriodicRauTauTimer: diam.Some(uint32(3240)), ActiveTime: diam.Some(uint32(60)),
		DlBufferingSuggestedPacketCount: diam.Some(int32(3)), MtcProviderInfo: mtcProvider,
		ScefReferenceIdForDeletion: []uint32{40}, ScefReferenceIdForDeletionExt: []uint64{1<<40 - 1},
	}
	ecRestriction = &EnhancedCoverageRestriction{
		RestrictedPlmnList: &RestrictedPlmnList{VisitedPlmnId: [][]byte{plmn}},
		AllowedPlmnList:    &AllowedPlmnList{VisitedPlmnId: [][]byte{{0x00, 0xf2, 0x10}}},
	}
)

// samples returns one message per S6t command, request then answer, in the
// order of table 8.2.2-1. Together they carry every AVP of tables 8.4.1-1
// and 8.4.1-2 that a command can reach.
func samples() []diam.Message {
	return []diam.Message{
		&ConfigurationInformationRequest{
			Header: hdr, SessionId: "scef.example.org;1;1", Drmp: diam.Some(DrmpPriority5), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: scef, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			UserIdentifier:      UserIdentifier{ExternalIdentifier: diam.Some(groupID), TypeOfExternalIdentifier: diam.Some(uint32(1))},
			OcSupportedFeatures: ocSF, SupportedFeatures: features,
			MonitoringEventConfiguration: []MonitoringEventConfiguration{
				{
					ScefReferenceId: diam.Some(uint32(41)), ScefReferenceIdExt: diam.Some(uint64(1) << 40), ScefId: scef,
					MonitoringType:             1, // UE_REACHABILITY
					ScefReferenceIdForDeletion: []uint32{38, 39}, ScefReferenceIdForDeletionExt: []uint64{1<<40 - 1},
					MaximumNumberOfReports: diam.Some(uint32(5)), MonitoringDuration: diam.Some(expiry),
					ChargedParty: diam.Some("sip:charged@example.org"), MaximumDetectionTime: diam.Some(uint32(3600)),
					UeReachabilityConfiguration: &UeReachabilityConfiguration{
						ReachabilityType: diam.Some(uint32(0x02)), MaximumLatency: diam.Some(uint32(120)),
						MaximumResponseTime: diam.Some(uint32(30)), DlBufferingSuggestedPacketCount: diam.Some(int32(4)),
					},
					LocationInformationConfiguration: &LocationInformationConfiguration{
						MonteLocationType: diam.Some(uint32(0)), Accuracy: diam.Some(uint32(1)), PeriodicTime: diam.Some(uint32(600)),
					},
					AssociationType: diam.Some(uint32(1)), DlBufferingSuggestedPacketCount: diam.Some(int32(1)),
					PlmnIdRequested: diam.Some(PlmnIdRequestedFalse), MtcProviderInfo: mtcProvider,
					PdnConnectivityStatusConfiguration: &PdnConnectivityStatusConfiguration{ServiceSelection: diam.Some(apn)},
					ExcludeIdentifiers:                 &ExcludeIdentifiers{ExternalIdentifier: []string{"device-2@iot.example.org"}, Msisdn: [][]byte{msisdn}},
					IncludeIdentifiers:                 &IncludeIdentifiers{ExternalIdentifier: []string{"device-3@iot.example.org"}},
				},
				{ScefId: scef, MonitoringType: 2, ScefReferenceIdForDeletion: []uint32{37}},
			},
			CirFlags: diam.Some(uint32(0x06)),
			AeseCommunicationPattern: []AeseCommunicationPattern{{
				ScefReferenceId: diam.Some(uint32(42)), ScefReferenceIdExt: diam.Some(uint64(42) << 32), ScefId: scef,
				ScefReferenceIdForDeletion: []uint32{36}, ScefReferenceIdForDeletionExt: []uint64{36 << 32},
				CommunicationPatternSet: []CommunicationPatternSet{{
					PeriodicCommunicationIndicator: diam.Some(uint32(0)), CommunicationDurationTime: diam.Some(uint32(60)),
					PeriodicTime: diam.Some(uint32(3600)),
					ScheduledCommunicationTime: []ScheduledCommunicationTime{
						{DayOfWeekMask: diam.Some(uint32(0x3e)), TimeOfDayStart: diam.Some(uint32(28800)), TimeOfDayEnd: diam.Some(uint32(64800))},
						{DayOfWeekMask: diam.Some(uint32(0x41))},
					},
					StationaryIndication: diam.Some(uint32(0)), ReferenceIdValidityTime: diam.Some(expiry),
					TrafficProfile: diam.Some(uint32(2)), BatteryIndicator: diam.Some(uint32(0x06)),
				}},
				MtcProviderInfo: mtcProvider,
			}},
			EnhancedCoverageRestriction: ecRestriction, GroupReportingGuardTimer: diam.Some(uint32(300)),
			AdditionalIdentifiers: &AdditionalIdentifiers{ExternalIdentifier: []string{"group-2@iot.example.org"}},
			ProxyInfo:             proxyInfo, RouteRecord: []string{"dra.example.org"},
			SuggestedNetworkConfiguration: suggestedNetwork,
		},
		&ConfigurationInformationAnswer{
			Header: hdr, SessionId: "scef.example.org;1;1", Drmp: diam.Some(DrmpPriority5), VendorSpecificApplicationId: vsai,
			ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			OcSupportedFeatures: ocSF, OcOlr: ocOlr, Load: load, SupportedFeatures: features,
			UserIdentifier: &UserIdentifier{UserName: diam.Some(imsi), Msisdn: diam.Some(msisdn), ExternalIdentifier: diam.Some(extID),
				TypeOfExternalIdentifier: diam.Some(uint32(0))},
			NumberOfUes: diam.Some(uint32(12)),
			MonitoringEventReport: []MonitoringEventReport{{
				ScefReferenceId: 41, ScefReferenceIdExt: diam.Some(uint64(1) << 40), ScefId: diam.Some(scef),
				ScefReferenceIdForDeletion: diam.Some(uint32(39)), ScefReferenceIdForDeletionExt: diam.Some(uint64(1<<40 - 1)),
				VisitedPlmnId: diam.Some(plmn), RoamingInformation: diam.Some(uint32(1)), ImeiChange: diam.Some(uint32(0x01)),
				TerminalInformation: &TerminalInformation{Imei: diam.Some("3534900698733"), Tgpp2Meid: diam.Some([]byte{0xa1, 0x00, 0x00, 0x01}),
					SoftwareVersion: diam.Some("01")},
				ReachabilityInformation: diam.Some(uint32(1)), ReachabilityCause: diam.Some(uint32(0)),
				MaximumUeAvailabilityTime: diam.Some(expiry), EpsLocationInformation: mmeLocation,
				MonitoringType: diam.Some(uint32(1)), EventHandling: diam.Some(uint32(2)), ServiceReport: serviceReport,
				LossOfConnectivityReason: diam.Some(uint32(4)), IdleStatusIndication: idle, PdnConnectivityStatusReport: pdnReport,
			}},
			MonitoringEventConfigStatus: []MonitoringEventConfigStatus{{
				ServiceReport: serviceReport, ScefReferenceId: 41, ScefReferenceIdExt: diam.Some(uint64(1) << 40), ScefId: diam.Some(scef),
			}},
			AeseCommunicationPatternConfigStatus: []AeseCommunicationPatternConfigStatus{{
				ScefReferenceId: 42, ScefReferenceIdExt: diam.Some(uint64(42) << 32), ScefId: diam.Some(scef),
				AeseErrorReport: &AeseErrorReport{ServiceResult: &ServiceResult{ServiceResultCode: diam.Some(uint32(5004))}},
			}},
			SupportedServices: []SupportedServices{
				{SupportedMonitoringEvents: diam.Some(uint64(0x1ff)), NodeType: diam.Some(uint32(0))},
				{SupportedMonitoringEvents: diam.Some(uint64(0x01e)), NodeType: diam.Some(uint32(1))},
			},
			S6THssCause:                     diam.Some(uint32(0x01)),
			EnhancedCoverageRestrictionData: &EnhancedCoverageRestrictionData{EnhancedCoverageRestriction: *ecRestriction, VisitedPlmnId: diam.Some(plmn)},
			CiaFlags:                        diam.Some(uint32(0x01)),
			ImsiGroupId:                     []ImsiGroupId{{GroupServiceId: 3, GroupPlmnId: plmn, LocalGroupId: []byte{0x12, 0x34}}},
			FailedAvp:                       failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
			SuggestedNetworkConfiguration: suggestedNetwork,
		},
		&ReportingInformationRequest{
			Header: hdr, SessionId: "hss.example.org;1;2", Drmp: diam.Some(DrmpPriority3), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, DestinationHost: scef, DestinationRealm: realm,
			SupportedFeatures: features, UserIdentifier: &UserIdentifier{Msisdn: diam.Some(msisdn)},
			MonitoringEventReport: []MonitoringEventReport{{
				ScefReferenceId: 43, ScefId: diam.Some(scef), EpsLocationInformation: sgsnLocation, MonitoringType: diam.Some(uint32(2)),
			}},
			GroupReport: []GroupReport{{
				ScefReferenceId: 41, ScefReferenceIdExt: diam.Some(uint64(1) << 40), ScefId: diam.Some(scef),
				GroupReportItem: []GroupReportItem{{
					UserIdentifier: UserIdentifier{ExternalIdentifier: diam.Some(extID)}, VisitedPlmnId: diam.Some(plmn),
					RoamingInformation: diam.Some(uint32(0)), ImeiChange: diam.Some(uint32(0x02)),
					ReachabilityInformation: diam.Some(uint32(0)), MaximumUeAvailabilityTime: diam.Some(expiry),
					EpsLocationInformation: mmeLocation, MonitoringType: diam.Some(uint32(8)), ServiceReport: serviceReport,
					S6THssCause: diam.Some(uint32(0x02)), IdleStatusIndication: idle, ReportingTimeStamp: diam.Some(stamp),
					UpdatedNetworkConfiguration: updatedNetwork, ScefReferenceIdForDeletion: []uint32{38},
					ScefReferenceIdForDeletionExt: []uint64{38 << 32}, EventHandling: diam.Some(uint32(0)),
					LossOfConnectivityReason: diam.Some(uint32(2)), PdnConnectivityStatusReport: pdnReport[:1],
				}},
			}},
			UpdatedNetworkConfiguration: updatedNetwork, RirFlags: diam.Some(uint32(0x05)),
			SupportedServices: []SupportedServices{{SupportedMonitoringEvents: diam.Some(uint64(0x06)), NodeType: diam.Some(uint32(2))}},
			ProxyInfo:         proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&ReportingInformationAnswer{
			Header: hdr, SessionId: "hss.example.org;1;2", Drmp: diam.Some(DrmpPriority3), VendorSpecificApplicationId: vsai,
			ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: scef, OriginRealm: realm, SupportedFeatures: features,
			MonitoringEventReportStatus: []MonitoringEventReportStatus{
				{ScefReferenceId: 43, ScefReferenceIdExt: diam.Some(uint64(43) << 32), ScefId: scef, ResultCode: diam.Some(uint32(2001))},
				{ScefReferenceId: 44, ScefId: scef, ExperimentalResultCode: diam.Some(uint32(5514))},
			},
			FailedAvp: failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&NiddInformationRequest{
			Header: hdr, SessionId: "scef.example.org;1;3", Drmp: diam.Some(DrmpPriority2), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: scef, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			UserIdentifier:      UserIdentifier{UserName: diam.Some(imsi)},
			OcSupportedFeatures: ocSF, SupportedFeatures: features,
			NiddAuthorizationRequest: &NiddAuthorizationRequest{ServiceSelection: diam.Some(apn), RequestedValidityTime: diam.Some(expiry)},
			NiddAuthorizationUpdate: &NiddAuthorizationUpdate{ApnValidityTime: []ApnValidityTime{
				{GrantedValidityTime: granted, ServiceSelection: diam.Some(apn)},
			}},
			NirFlags: diam.Some(uint32(0x01)),
			GroupUserIdentifier: []GroupUserIdentifier{
				{UserName: diam.Some(imsi), Msisdn: diam.Some(msisdn), ExternalIdentifier: diam.Some(extID)},
			},
			MtcProviderInfo: mtcProvider, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&NiddInformationAnswer{
			Header: hdr, SessionId: "scef.example.org;1;3", Drmp: diam.Some(DrmpPriority2), VendorSpecificApplicationId: vsai,
			ExperimentalResult: unauthorized, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			OcSupportedFeatures: ocSF, OcOlr: ocOlr, Load: load, SupportedFeatures: features,
			NiddAuthorizationResponse: &NiddAuthorizationResponse{Msisdn: diam.Some(msisdn), UserName: diam.Some(imsi),
				ExternalIdentifier: diam.Some(extID), GrantedValidityTime: diam.Some(granted)},
			GroupUserIdentifier: []GroupUserIdentifier{{UserName: diam.Some(imsi), ExternalIdentifier: diam.Some(extID)}},
			NiaFlags:            diam.Some(uint32(0x01)),
			FailedAvp:           failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
	}
}
