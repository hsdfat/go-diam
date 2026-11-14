package swx_test

import (
	"fmt"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/cx"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6a"
	"github.com/hsdfat/go-diam/swx"
)

// A 3GPP AAA Server asks the HSS for an EAP-AKA' vector with a
// Multimedia-Auth-Request; the HSS decodes it, checks it and answers.
func Example() {
	vsai := base.VendorSpecificApplicationId{VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777265))}
	mar := &swx.MultimediaAuthRequest{
		Header:                      diam.Header{HopByHop: 1, EndToEnd: 1},
		SessionId:                   "aaa.epc.example.org;1;42",
		VendorSpecificApplicationId: vsai,
		AuthSessionState:            base.AuthSessionStateNoStateMaintained,
		OriginHost:                  "aaa.epc.example.org",
		OriginRealm:                 "epc.example.org",
		DestinationRealm:            "epc.example.org",
		UserName:                    "0001010000000001@wlan.mnc001.mcc001.3gppnetwork.org",
		RatType:                     diam.Some(s6a.RatTypeWlan),
		Anid:                        diam.Some("WLAN"),
		SipAuthDataItem:             swx.SipAuthDataItem{SipAuthenticationScheme: diam.Some("EAP-AKA'")},
		SipNumberAuthItems:          1,
		SupportedFeatures:           []cx.SupportedFeatures{{VendorId: 10415, FeatureListId: 1, FeatureList: 0x01}},
	}
	wire, err := mar.Marshal()
	if err != nil {
		panic(err)
	}

	// HSS side.
	m, err := diam.Decode(wire)
	if err != nil {
		panic(err)
	}
	req := m.(*swx.MultimediaAuthRequest)
	if err := req.Validate(); err != nil {
		panic(err)
	}
	maa := swx.NewMultimediaAuthAnswerFor(req)
	maa.VendorSpecificApplicationId = vsai
	maa.ResultCode = diam.Some(uint32(2001))
	maa.AuthSessionState = base.AuthSessionStateNoStateMaintained
	maa.OriginHost, maa.OriginRealm, maa.UserName = "hss.epc.example.org", "epc.example.org", req.UserName
	maa.SipNumberAuthItems = diam.Some(uint32(1))
	maa.SipAuthDataItem = []swx.SipAuthDataItem{{
		SipAuthenticationScheme: diam.Some("EAP-AKA'"),
		SipAuthenticate:         diam.Some(make([]byte, 32)), // RAND || AUTN
		SipAuthorization:        diam.Some(make([]byte, 16)), // XRES
		ConfidentialityKey:      diam.Some(make([]byte, 16)), // CK'
		IntegrityKey:            diam.Some(make([]byte, 16)), // IK'
	}}
	answer, _ := maa.Marshal()

	// 3GPP AAA Server side.
	var got swx.MultimediaAuthAnswer
	if err := got.Unmarshal(answer); err != nil {
		panic(err)
	}
	fmt.Println(req.RatType.V, got.SessionId, got.ResultCode.V, got.SipAuthDataItem[0].SipAuthenticationScheme.V)

	// Output:
	// WLAN aaa.epc.example.org;1;42 2001 EAP-AKA'
}
