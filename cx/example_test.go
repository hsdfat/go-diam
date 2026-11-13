package cx_test

import (
	"fmt"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/cx"
	"github.com/hsdfat/go-diam/diam"
)

// An S-CSCF asks the HSS for an IMS-AKA vector with a Multimedia-Auth-Request;
// the HSS decodes it, checks it and answers with one SIP-Auth-Data-Item.
func Example() {
	mar := &cx.MultimediaAuthRequest{
		Header:    diam.Header{HopByHop: 1, EndToEnd: 1},
		SessionId: "scscf.ims.example.org;1;42",
		VendorSpecificApplicationId: base.VendorSpecificApplicationId{
			VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777216)),
		},
		AuthSessionState:   base.AuthSessionStateNoStateMaintained,
		OriginHost:         "scscf.ims.example.org",
		OriginRealm:        "ims.example.org",
		DestinationRealm:   "ims.example.org",
		UserName:           "001010000000001@ims.example.org",
		PublicIdentity:     "sip:+15551234567@ims.example.org",
		SipAuthDataItem:    cx.SipAuthDataItem{SipAuthenticationScheme: diam.Some("Digest-AKAv1-MD5")},
		SipNumberAuthItems: 1,
		ServerName:         "sip:scscf.ims.example.org:5060",
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
	req := m.(*cx.MultimediaAuthRequest)
	if err := req.Validate(); err != nil {
		panic(err)
	}
	maa := cx.NewMultimediaAuthAnswerFor(req)
	maa.VendorSpecificApplicationId = req.VendorSpecificApplicationId
	maa.ResultCode = diam.Some(uint32(2001))
	maa.AuthSessionState = base.AuthSessionStateNoStateMaintained
	maa.OriginHost, maa.OriginRealm = "hss.ims.example.org", "ims.example.org"
	maa.SipNumberAuthItems = diam.Some(uint32(1))
	maa.SipAuthDataItem = []cx.SipAuthDataItem{{
		SipItemNumber:           diam.Some(uint32(1)),
		SipAuthenticationScheme: diam.Some("Digest-AKAv1-MD5"),
		SipAuthenticate:         diam.Some(make([]byte, 32)), // RAND || AUTN
		SipAuthorization:        diam.Some(make([]byte, 8)),  // XRES
		ConfidentialityKey:      diam.Some(make([]byte, 16)),
		IntegrityKey:            diam.Some(make([]byte, 16)),
	}}
	answer, _ := maa.Marshal()

	// S-CSCF side.
	var got cx.MultimediaAuthAnswer
	if err := got.Unmarshal(answer); err != nil {
		panic(err)
	}
	item := got.SipAuthDataItem[0]
	fmt.Println(req.PublicIdentity, got.SessionId, got.ResultCode.V, item.SipAuthenticationScheme.V, len(item.SipAuthenticate.V))

	// Output:
	// sip:+15551234567@ims.example.org scscf.ims.example.org;1;42 2001 Digest-AKAv1-MD5 32
}
