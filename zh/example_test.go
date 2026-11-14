package zh_test

import (
	"fmt"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/cx"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/zh"
)

// A BSF bootstraps a UE: it asks the HSS for one authentication vector and
// the user's GBA security settings with a Zh Multimedia-Auth-Request.
func Example() {
	vsai := base.VendorSpecificApplicationId{VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777221))}
	mar := &zh.MultimediaAuthRequest{
		Header:                      diam.Header{HopByHop: 1, EndToEnd: 1},
		SessionId:                   "bsf.ims.example.org;1;42",
		VendorSpecificApplicationId: vsai,
		AuthSessionState:            base.AuthSessionStateNoStateMaintained,
		OriginHost:                  "bsf.ims.example.org",
		OriginRealm:                 "ims.example.org",
		DestinationRealm:            "ims.example.org",
		UserName:                    diam.Some("001010000000001@ims.example.org"),
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
	req := m.(*zh.MultimediaAuthRequest)
	if err := req.Validate(); err != nil {
		panic(err)
	}
	maa := zh.NewMultimediaAuthAnswerFor(req)
	maa.VendorSpecificApplicationId = vsai
	maa.ResultCode = diam.Some(uint32(2001))
	maa.AuthSessionState = base.AuthSessionStateNoStateMaintained
	maa.OriginHost, maa.OriginRealm = "hss.ims.example.org", "ims.example.org"
	maa.UserName = req.UserName
	maa.SipAuthDataItem = &cx.SipAuthDataItem{SipAuthenticationScheme: diam.Some("Digest-AKAv1-MD5"),
		SipAuthenticate: diam.Some(make([]byte, 32)), SipAuthorization: diam.Some(make([]byte, 8)),
		ConfidentialityKey: diam.Some(make([]byte, 16)), IntegrityKey: diam.Some(make([]byte, 16))}
	maa.GbaUserSecSettings = diam.Some([]byte("<guss/>"))
	answer, _ := maa.Marshal()

	// BSF side.
	var got zh.MultimediaAuthAnswer
	if err := got.Unmarshal(answer); err != nil {
		panic(err)
	}
	fmt.Println(got.SessionId, got.ResultCode.V, got.SipAuthDataItem.SipAuthenticationScheme.V, string(got.GbaUserSecSettings.V))

	// Output:
	// bsf.ims.example.org;1;42 2001 Digest-AKAv1-MD5 <guss/>
}
