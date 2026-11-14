package zh

import (
	"bytes"
	"time"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/cx"
	"github.com/hsdfat/go-diam/diam"
)

const (
	bsf, hss, realm = "bsf.ims.example.org", "hss.ims.example.org", "ims.example.org"
	impi            = "001010000000001@ims.example.org"
	impu            = "sip:+15551234567@ims.example.org"
	aka             = "Digest-AKAv1-MD5"
)

var (
	hdr = diam.Header{HopByHop: 0x11, EndToEnd: 0x22}
	// §4.2 step 1: Vendor-Id 3GPP and Auth-Application-Id 16777221.
	vsai = base.VendorSpecificApplicationId{VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777221))}
	nsm  = base.AuthSessionStateNoStateMaintained

	gussStamp = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	// A synchronization failure: SIP-Authorization carries RAND and AUTS
	// (TS 29.229 §6.3.10), 16 + 14 octets.
	resync = &cx.SipAuthDataItem{SipAuthenticationScheme: diam.Some(aka), SipAuthorization: diam.Some(bytes.Repeat([]byte{0x5a}, 30))}
	// One Digest-AKAv1-MD5 vector: RAND||AUTN, XRES, CK, IK.
	key16  = bytes.Repeat([]byte{0x3c}, 16)
	vector = &cx.SipAuthDataItem{SipItemNumber: diam.Some(uint32(1)), SipAuthenticationScheme: diam.Some(aka),
		SipAuthenticate: diam.Some(bytes.Repeat([]byte{0xa5}, 32)), SipAuthorization: diam.Some([]byte{1, 2, 3, 4, 5, 6, 7, 8}),
		ConfidentialityKey: diam.Some(key16), IntegrityKey: diam.Some(key16)}
	guss      = []byte(`<?xml version="1.0" encoding="UTF-8"?><guss xmlns="urn:3gpp:gba:GBAGUSSSchema-R9:2010-02" id="1"><bsfInfo/><ussList/></guss>`)
	proxyInfo = []base.ProxyInfo{{ProxyHost: "dra.ims.example.org", ProxyState: []byte{1, 2, 3}}}
)

// samples returns one message per Zh command, request then answer. Together
// they carry every AVP a Zh command can carry. The MAR has both User-Name
// and Public-Identity, which §4.2 step 1 does not combine; the grammar
// allows it.
func samples() []diam.Message {
	return []diam.Message{
		&MultimediaAuthRequest{
			Header: hdr, SessionId: "bsf.ims.example.org;1;1", VendorSpecificApplicationId: vsai, AuthSessionState: nsm,
			OriginHost: bsf, OriginRealm: realm, DestinationRealm: realm, DestinationHost: diam.Some(hss),
			UserName: diam.Some(impi), PublicIdentity: diam.Some(impu), SipAuthDataItem: resync,
			GussTimestamp: diam.Some(gussStamp), ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
		&MultimediaAuthAnswer{
			Header: hdr, SessionId: "bsf.ims.example.org;1;1", VendorSpecificApplicationId: vsai, ResultCode: diam.Some(uint32(2001)),
			AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, UserName: diam.Some(impi), PublicIdentity: diam.Some(impu),
			SipAuthDataItem: vector, GbaUserSecSettings: diam.Some(guss), Sha256Indication: diam.Some(Sha256IndicationUiccSupportSha256),
			ProxyInfo: proxyInfo, RouteRecord: []string{"dra.ims.example.org"},
		},
	}
}
