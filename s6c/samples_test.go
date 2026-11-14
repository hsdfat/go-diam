package s6c

import (
	"time"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
)

const (
	gmsc, hss, realm = "sms-gmsc.example.org", "hss.example.org", "example.org"
	mme              = "mme.example.org"
	imsi             = "001010000000001"
)

var (
	hdr = diam.Header{HopByHop: 0x11, EndToEnd: 0x22}
	nsm = base.AuthSessionStateNoStateMaintained
	// §4.7: the 3GPP vendor and the S6c application id. Not sent by S6c
	// nodes (§5.3.2.2), but listed in every ABNF and accepted.
	vsai = &base.VendorSpecificApplicationId{VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777312))}
	// Times are whole seconds: the Time type has no finer resolution.
	available = time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)

	proxyInfo = []base.ProxyInfo{{ProxyHost: "dra.example.org", ProxyState: []byte{1, 2, 3}}}
	// Feature-List-ID 1 (table 5.3.3.13/1): SMSF-Support.
	features  = []SupportedFeatures{{VendorId: 10415, FeatureListId: 1, FeatureList: 0x01}}
	failedAvp = &base.FailedAvp{Unknown: []diam.RawAVP{
		{Code: AVPSrrFlags, Vendor: 10415, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte{1, 2}},
	}}
	// DIAMETER_ERROR_ABSENT_USER (§7.3.3).
	absentUser = &base.ExperimentalResult{VendorId: 10415, ExperimentalResultCode: 5550}

	// TBCD-encoded numbers (TS 29.329 §6.3.2, TS 29.338 §6.3.3.2).
	msisdn     = []byte{0x51, 0x55, 0x21, 0x43, 0x65, 0xf7}
	scAddress  = []byte{0x51, 0x55, 0x00, 0x00, 0x10, 0xf0}
	mscNumber  = []byte{0x51, 0x55, 0x00, 0x00, 0x20, 0xf0}
	sgsnNumber = []byte{0x51, 0x55, 0x00, 0x00, 0x30, 0xf0}
	mmeNumber  = []byte{0x51, 0x55, 0x00, 0x00, 0x40, 0xf0}
	ipsmNumber = []byte{0x51, 0x55, 0x00, 0x00, 0x50, 0xf0}
	smsfNumber = []byte{0x51, 0x55, 0x00, 0x00, 0x60, 0xf0}
	lmsi       = []byte{0x00, 0x00, 0x00, 0x2a}
	smea       = []byte{0x0b, 0x91, 0x51, 0x55}

	correlation = &SmsmiCorrelationId{HssId: diam.Some("hss-1"), OriginatingSipUri: diam.Some("sip:alice@ims.example.org"),
		DestinationSipUri: diam.Some("sip:bob@ims.example.org")}
	servingNode = &ServingNode{
		SgsnName: diam.Some("sgsn.example.org"), SgsnRealm: diam.Some(realm), SgsnNumber: diam.Some(sgsnNumber),
		MmeName: diam.Some(mme), MmeRealm: diam.Some(realm), MmeNumberForMtSms: diam.Some(mmeNumber),
		MscNumber: diam.Some(mscNumber), IpSmGwNumber: diam.Some(ipsmNumber), IpSmGwName: diam.Some("ipsmgw.example.org"),
		IpSmGwRealm: diam.Some(realm), IpSmGwSbiSupportIndicator: diam.Some(IpSmGwSbiSupportIndicatorSupportSbi),
	}
	userID = UserIdentifier{UserName: diam.Some(imsi), Msisdn: diam.Some(msisdn),
		ExternalIdentifier: diam.Some("device-1@iot.example.org"), Lmsi: diam.Some(lmsi)}
)

// samples returns one message per S6c command, request then answer, in the
// order of table 5.3.2.2/1. Together they carry every AVP of tables
// 5.3.3.1/1 and 5.3.3.1/2 that a command can carry; SM-Delivery-Failure-Cause
// is in none (TestSmDeliveryFailureCause).
func samples() []diam.Message {
	return []diam.Message{
		&SendRoutingInfoForSmRequest{
			Header: hdr, SessionId: "sms-gmsc.example.org;1;1", Drmp: diam.Some(DrmpPriority5), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: gmsc, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			Msisdn: diam.Some(msisdn), UserName: diam.Some(imsi), SmsmiCorrelationId: correlation, SupportedFeatures: features,
			ScAddress: diam.Some(scAddress), SmRpMti: diam.Some(SmRpMtiSmDeliver), SmRpSmea: diam.Some(smea),
			SrrFlags: diam.Some(uint32(0x07)), SmDeliveryNotIntended: diam.Some(SmDeliveryNotIntendedOnlyImsiRequested),
			ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&SendRoutingInfoForSmAnswer{
			Header: hdr, SessionId: "sms-gmsc.example.org;1;1", Drmp: diam.Some(DrmpPriority5), VendorSpecificApplicationId: vsai,
			ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			UserName: diam.Some(imsi), SupportedFeatures: features, ServingNode: servingNode,
			AdditionalServingNode: &AdditionalServingNode{
				SgsnName: diam.Some("sgsn2.example.org"), SgsnRealm: diam.Some(realm), SgsnNumber: diam.Some(sgsnNumber),
				MmeName: diam.Some("mme2.example.org"), MmeRealm: diam.Some(realm), MmeNumberForMtSms: diam.Some(mmeNumber),
				MscNumber: diam.Some(mscNumber),
			},
			Smsf3GppAddress: &Smsf3GppAddress{Smsf3GppNumber: diam.Some(smsfNumber), Smsf3GppName: diam.Some("smsf.example.org"),
				Smsf3GppRealm: diam.Some(realm), Smsf3GppSbiSupportIndicator: diam.Some(Smsf3GppSbiSupportIndicatorSupportSbi)},
			SmsfNon3GppAddress: &SmsfNon3GppAddress{SmsfNon3GppNumber: diam.Some(smsfNumber), SmsfNon3GppName: diam.Some("smsf-n3.example.org"),
				SmsfNon3GppRealm: diam.Some(realm), SmsfNon3GppSbiSupportIndicator: diam.Some(SmsfNon3GppSbiSupportIndicatorNotSupportSbi)},
			Lmsi: diam.Some(lmsi), UserIdentifier: &userID, MwdStatus: diam.Some(uint32(0x06)),
			MmeAbsentUserDiagnosticSm: diam.Some(uint32(1)), MscAbsentUserDiagnosticSm: diam.Some(uint32(2)),
			SgsnAbsentUserDiagnosticSm: diam.Some(uint32(3)), Smsf3GppAbsentUserDiagnosticSm: diam.Some(uint32(4)),
			SmsfNon3GppAbsentUserDiagnosticSm: diam.Some(uint32(5)),
			FailedAvp:                         failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&AlertServiceCentreRequest{
			Header: hdr, SessionId: "hss.example.org;1;2", Drmp: diam.Some(DrmpPriority3), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, DestinationHost: diam.Some(gmsc), DestinationRealm: realm,
			ScAddress: scAddress, UserIdentifier: UserIdentifier{Msisdn: diam.Some(msisdn)}, SmsmiCorrelationId: correlation,
			MaximumUeAvailabilityTime: diam.Some(available), SmsGmscAlertEvent: diam.Some(uint32(0x03)),
			ServingNode:       &ServingNode{MmeName: diam.Some(mme), MmeRealm: diam.Some(realm), MmeNumberForMtSms: diam.Some(mmeNumber)},
			SupportedFeatures: features, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&AlertServiceCentreAnswer{
			Header: hdr, SessionId: "hss.example.org;1;2", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: nsm, OriginHost: gmsc, OriginRealm: realm, SupportedFeatures: features,
			FailedAvp: failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&ReportSmDeliveryStatusRequest{
			Header: hdr, SessionId: "sms-gmsc.example.org;1;3", Drmp: diam.Some(DrmpPriority2), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: gmsc, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			SupportedFeatures: features, UserIdentifier: UserIdentifier{UserName: diam.Some(imsi)}, SmsmiCorrelationId: correlation,
			ScAddress: scAddress,
			SmDeliveryOutcome: SmDeliveryOutcome{
				MmeSmDeliveryOutcome:         &DeliveryOutcome{SmDeliveryCause: diam.Some(SmDeliveryCauseAbsentUser), AbsentUserDiagnosticSm: diam.Some(uint32(1))},
				MscSmDeliveryOutcome:         &DeliveryOutcome{SmDeliveryCause: diam.Some(SmDeliveryCauseUeMemoryCapacityExceeded)},
				SgsnSmDeliveryOutcome:        &DeliveryOutcome{SmDeliveryCause: diam.Some(SmDeliveryCauseSuccessfulTransfer)},
				IpSmGwSmDeliveryOutcome:      &DeliveryOutcome{AbsentUserDiagnosticSm: diam.Some(uint32(7))},
				Smsf3GppSmDeliveryOutcome:    &DeliveryOutcome{SmDeliveryCause: diam.Some(SmDeliveryCauseAbsentUser)},
				SmsfNon3GppSmDeliveryOutcome: &DeliveryOutcome{SmDeliveryCause: diam.Some(SmDeliveryCauseAbsentUser), AbsentUserDiagnosticSm: diam.Some(uint32(2))},
			},
			RdrFlags: diam.Some(uint32(0x01)), ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&ReportSmDeliveryStatusAnswer{
			Header: hdr, SessionId: "sms-gmsc.example.org;1;3", Drmp: diam.Some(DrmpPriority2), VendorSpecificApplicationId: vsai,
			ExperimentalResult: absentUser, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, SupportedFeatures: features,
			UserIdentifier: &UserIdentifier{Msisdn: diam.Some(msisdn)},
			FailedAvp:      failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
	}
}
