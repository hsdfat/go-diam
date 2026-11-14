package s6m

import (
	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
)

const (
	iwf, hss, realm = "mtc-iwf.example.org", "hss.example.org", "example.org"
	imsi            = "001010000000001"
	extID           = "device-1@iot.example.org"
)

var (
	hdr = diam.Header{HopByHop: 0x11, EndToEnd: 0x22}
	nsm = base.AuthSessionStateNoStateMaintained
	// §6.1.7: the 3GPP vendor and the S6m application id. Not sent by S6m
	// nodes (§6.2.2), but accepted when an intermediate agent adds it.
	vsai = &base.VendorSpecificApplicationId{VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777310))}

	proxyInfo = []base.ProxyInfo{{ProxyHost: "dra.example.org", ProxyState: []byte{1, 2, 3}}}
	features  = []SupportedFeatures{{VendorId: 10415, FeatureListId: 1, FeatureList: 0}}
	failedAvp = &base.FailedAvp{Unknown: []diam.RawAVP{
		{Code: AVPSirFlags, Vendor: 10415, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte{1, 2}},
	}}
	// DIAMETER_ERROR_UNAUTHORIZED_SERVICE (§6.3.3.3).
	unauthorized = &base.ExperimentalResult{VendorId: 10415, ExperimentalResultCode: 5511}
	ocSF         = &OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))}
	ocOlr        = &OcOlr{OcSequenceNumber: 9, OcReportType: OcReportTypeHostReport,
		OcReductionPercentage: diam.Some(uint32(50)), OcValidityDuration: diam.Some(uint32(30))}
	load = []Load{{LoadType: diam.Some(LoadTypeHost), LoadValue: diam.Some(uint64(1000)), Sourceid: diam.Some(hss)}}

	// TBCD-encoded numbers (TS 29.329 §6.3.2, TS 29.173 §6.4.5).
	msisdn     = []byte{0x51, 0x55, 0x21, 0x43, 0x65, 0xf7}
	scsNumber  = []byte{0x51, 0x55, 0x00, 0x00, 0x10, 0xf0}
	mscNumber  = []byte{0x51, 0x55, 0x00, 0x00, 0x20, 0xf0}
	sgsnNumber = []byte{0x51, 0x55, 0x00, 0x00, 0x30, 0xf0}
	mmeNumber  = []byte{0x51, 0x55, 0x00, 0x00, 0x40, 0xf0}
	ipsmNumber = []byte{0x51, 0x55, 0x00, 0x00, 0x50, 0xf0}
	lmsi       = []byte{0x00, 0x00, 0x00, 0x2a}
)

// samples returns SIR then SIA (table 6.2.2/1). Together they carry every
// AVP of tables 6.4.1/1 and 6.4.1/2 and every member of Serving-Node.
func samples() []diam.Message {
	return []diam.Message{
		&SubscriberInformationRequest{
			Header: hdr, SessionId: "mtc-iwf.example.org;1;1", Drmp: diam.Some(DrmpPriority5), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: iwf, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			UserIdentifier: UserIdentifier{ExternalIdentifier: diam.Some(extID)},
			ServiceId:      diam.Some(ServiceIdDeviceTrigger), ScsIdentity: diam.Some(scsNumber),
			ServiceParameters: &ServiceParameters{
				T4Parameters:              &T4Parameters{PriorityIndication: diam.Some(PriorityIndicationPriority), SmRpSmea: diam.Some([]byte{0x0b, 0x91, 0x51, 0x55})},
				ApplicationPortIdentifier: diam.Some(uint32(9200)),
			},
			SirFlags: 0x01, OcSupportedFeatures: ocSF, SupportedFeatures: features,
			ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
		&SubscriberInformationAnswer{
			Header: hdr, SessionId: "mtc-iwf.example.org;1;1", Drmp: diam.Some(DrmpPriority5), VendorSpecificApplicationId: vsai,
			ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			OcSupportedFeatures: ocSF, OcOlr: ocOlr, Load: load, SupportedFeatures: features,
			UserIdentifier: []UserIdentifier{
				{UserName: diam.Some(imsi), Msisdn: diam.Some(msisdn), ExternalIdentifier: diam.Some(extID), Lmsi: diam.Some(lmsi)},
				{UserName: diam.Some("001010000000002")},
			},
			ServiceData: &ServiceData{T4Data: &T4Data{
				HssCause: diam.Some(uint32(0)),
				ServingNode: &ServingNode{
					SgsnName: diam.Some("sgsn.example.org"), SgsnRealm: diam.Some(realm), SgsnNumber: diam.Some(sgsnNumber),
					MmeName: diam.Some("mme.example.org"), MmeRealm: diam.Some(realm), MmeNumberForMtSms: diam.Some(mmeNumber),
					MscNumber: diam.Some(mscNumber), IpSmGwNumber: diam.Some(ipsmNumber), IpSmGwName: diam.Some("ipsmgw.example.org"),
					IpSmGwRealm: diam.Some(realm),
				},
				AdditionalServingNode: []AdditionalServingNode{
					{SgsnName: diam.Some("sgsn2.example.org"), SgsnRealm: diam.Some(realm), SgsnNumber: diam.Some(sgsnNumber),
						MmeName: diam.Some("mme2.example.org"), MmeRealm: diam.Some(realm), MmeNumberForMtSms: diam.Some(mmeNumber),
						MscNumber: diam.Some(mscNumber)},
				},
			}},
			FailedAvp: failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.example.org"},
		},
	}
}
