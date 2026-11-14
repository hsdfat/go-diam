package slh

import (
	"net/netip"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6a"
)

const (
	gmlc, hss, realm = "gmlc.epc.example.org", "hss.epc.example.org", "epc.example.org"
	imsi             = "001010000000001"
	mme, sgsn        = "mme.epc.example.org", "sgsn.epc.example.org"
	aaa              = "aaa.epc.example.org"
)

var (
	hdr = diam.Header{HopByHop: 0x11, EndToEnd: 0x22}
	// §6.1.8: Auth-Application-Id 16777291 inside the 3GPP
	// Vendor-Specific-Application-Id (sent only by non-compliant agents,
	// §6.2.2, but in the grammar).
	vsai = &base.VendorSpecificApplicationId{VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777291))}
	nsm  = base.AuthSessionStateNoStateMaintained

	// E.164 numbers as TBCD strings (TS 29.002): MSISDN +15551234567, the
	// GMLC +15550000001, the SGSN +15550000002 and the MSC +15550000003.
	msisdn     = []byte{0x51, 0x55, 0x21, 0x43, 0x65, 0xf7}
	gmlcNumber = []byte{0x51, 0x55, 0x00, 0x00, 0x00, 0xf1}
	sgsnNumber = []byte{0x51, 0x55, 0x00, 0x00, 0x00, 0xf2}
	mscNumber  = []byte{0x51, 0x55, 0x00, 0x00, 0x00, 0xf3}
	// LMSI is four octets (TS 23.003 §2.9).
	lmsi = []byte{0x01, 0x02, 0x03, 0x04}

	hGMLC = netip.MustParseAddr("192.0.2.10")
	vGMLC = netip.MustParseAddr("2001:db8::10")
	ppr   = netip.MustParseAddr("192.0.2.20")

	// §6.4.10-6.4.11: Feature-List-ID 1, and no feature is defined, so
	// Feature-List is 0.
	features  = []s6a.SupportedFeatures{{VendorId: 10415, FeatureListId: 1, FeatureList: 0}}
	proxyInfo = []base.ProxyInfo{{ProxyHost: "dra.epc.example.org", ProxyState: []byte{1, 2, 3}}}
	failedAvp = &base.FailedAvp{Unknown: []diam.RawAVP{
		{Code: AVPLmsi, Vendor: 10415, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte{9}},
	}}
)

// samples returns one message per SLh command, request then answer (table
// 6.2.2/1). Together they carry every AVP an SLh command can carry.
func samples() []diam.Message {
	return []diam.Message{
		&LcsRoutingInfoRequest{
			Header: hdr, SessionId: "gmlc.epc.example.org;1;1", VendorSpecificApplicationId: vsai, AuthSessionState: nsm,
			OriginHost: gmlc, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			UserName: diam.Some(imsi), Msisdn: diam.Some(msisdn), GmlcNumber: diam.Some(gmlcNumber),
			SupportedFeatures: features, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.epc.example.org"},
		},
		&LcsRoutingInfoAnswer{
			Header: hdr, SessionId: "gmlc.epc.example.org;1;1", VendorSpecificApplicationId: vsai,
			ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
			SupportedFeatures: features, UserName: diam.Some(imsi), Msisdn: diam.Some(msisdn), Lmsi: diam.Some(lmsi),
			ServingNode: &ServingNode{
				SgsnNumber: diam.Some(sgsnNumber), SgsnName: diam.Some(sgsn), SgsnRealm: diam.Some(realm),
				MmeName: diam.Some(mme), MmeRealm: diam.Some(realm), MscNumber: diam.Some(mscNumber),
				LcsCapabilitiesSets: diam.Some(uint32(0x10)), GmlcAddress: diam.Some(vGMLC),
			},
			AdditionalServingNode: []AdditionalServingNode{
				{TgppAaaServerName: diam.Some(aaa), GmlcAddress: diam.Some(hGMLC)},
				{MscNumber: diam.Some(mscNumber), MmeName: diam.Some(mme), SgsnName: diam.Some(sgsn), SgsnRealm: diam.Some(realm),
					MmeRealm: diam.Some(realm), SgsnNumber: diam.Some(sgsnNumber), LcsCapabilitiesSets: diam.Some(uint32(0x1f))},
			},
			GmlcAddress: diam.Some(hGMLC), PprAddress: diam.Some(ppr), RiaFlags: diam.Some(uint32(1)),
			FailedAvp: failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.epc.example.org"},
		},
	}
}
