//go:build peer

package interop

import (
	"fmt"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/cx"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6a"
)

// The synthetic subscriber provisioned by ci/open5gs/subscriber.js. The
// PLMN is the test network 001/01 (TS 23.003 §2.2, 3GPP test PLMN).
const (
	hssIMSI        = "001010000000001"
	hssUnknownIMSI = "001010000000099" // not provisioned
	hssIMSDomain   = "ims.mnc001.mcc001.3gppnetwork.org"
)

// testPLMN is MCC 001, MNC 01 encoded as in TS 29.272 §7.3.9 (TS 24.008
// table 10.5.3): MCC digit 2 | MCC digit 1, MNC digit 3 (F) | MCC digit 3,
// MNC digit 2 | MNC digit 1.
var testPLMN = []byte{0x00, 0xf1, 0x10}

// open5gsSource is the SOURCE text of a capture directory written from the
// Open5GS HSS. The workflow passes the software and image description.
func open5gsSource(cea *base.CapabilitiesExchangeAnswer, run, what string) string {
	return fmt.Sprintf(`Frames received from a Diameter peer by internal/interop/peer_open5gs_test.go.

Peer:     %s (Product-Name %q, Vendor-Id %d, Firmware-Revision %d)
Software: %s
Image:    %s
Date:     %s
Command:  go test -tags peer -run '%s' -update ./internal/interop/
Content:  %s

Generated in CI against a synthetic subscriber (IMSI %s, random test K and
OPc in ci/open5gs/subscriber.js); no operator traffic.
`, cea.OriginHost, cea.ProductName, cea.VendorId, cea.FirmwareRevision.V,
		os.Getenv("DIAM_PEER_SOFTWARE"), os.Getenv("DIAM_PEER_IMAGE"),
		time.Now().UTC().Format("2006-01-02"), run, what, hssIMSI)
}

// hssCER is the CER of an MME and S-CSCF towards the HSS: S6a and Cx, both
// under 3GPP (TS 29.272 §7.1.8, TS 29.229 §5.6).
func hssCER(p *peerConn) *base.CapabilitiesExchangeRequest {
	return &base.CapabilitiesExchangeRequest{
		OriginHost:        p.host,
		OriginRealm:       p.realm,
		HostIpAddress:     []netip.Addr{p.localAddr()},
		VendorId:          tgpp,
		ProductName:       "go-diam",
		SupportedVendorId: []uint32{tgpp},
		InbandSecurityId:  []uint32{0}, // NO_INBAND_SECURITY
		VendorSpecificApplicationId: []base.VendorSpecificApplicationId{
			{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(s6aApp))},
			{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(cxApp))},
		},
	}
}

func resultOf(rc diam.Opt[uint32], er *base.ExperimentalResult) string {
	switch {
	case rc.Set:
		return fmt.Sprintf("Result-Code %d", rc.V)
	case er != nil:
		return fmt.Sprintf("Experimental-Result %d/%d", er.VendorId, er.ExperimentalResultCode)
	}
	return "no result"
}

// TestPeerS6a acts as an MME towards the Open5GS HSS at DIAM_HSS_ADDR:
// AIR/AIA and ULR/ULA for the provisioned subscriber, an AIR for an unknown
// one, and PUR/PUA.
func TestPeerS6a(t *testing.T) {
	const host, realm = "mme.test", "test"
	p := dialPeer(t, "DIAM_HSS_ADDR", host, realm, "s6a")
	cea := p.capabilities("cea", hssCER(p))
	hss := string(cea.OriginHost)
	vsai := func() *base.VendorSpecificApplicationId {
		return &base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(s6aApp))}
	}
	air := func(imsi string) *s6a.AuthenticationInformationRequest {
		return &s6a.AuthenticationInformationRequest{
			Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai(),
			AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			UserName: imsi,
			RequestedEutranAuthenticationInfo: &s6a.RequestedEutranAuthenticationInfo{
				NumberOfRequestedVectors:   diam.Some(uint32(1)),
				ImmediateResponsePreferred: diam.Some(uint32(1)),
			},
			VisitedPlmnId: testPLMN,
		}
	}

	if aia, ok := p.exchange("aia", air(hssIMSI)).(*s6a.AuthenticationInformationAnswer); ok {
		t.Logf("AIA: %s", resultOf(aia.ResultCode, aia.ExperimentalResult))
		if aia.ResultCode.V != 2001 || aia.AuthenticationInfo == nil || len(aia.AuthenticationInfo.EUtranVector) == 0 {
			t.Errorf("AIA: no E-UTRAN vector: %+v", aia)
		}
	}

	if ula, ok := p.exchange("ula", &s6a.UpdateLocationRequest{
		Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai(),
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
		UserName:      hssIMSI,
		RatType:       s6a.RatTypeEutran,
		UlrFlags:      1<<1 | 1<<5, // S6a/S6d-Indicator, Initial-Attach-Indicator (TS 29.272 §7.3.7)
		VisitedPlmnId: testPLMN,
	}).(*s6a.UpdateLocationAnswer); ok {
		t.Logf("ULA: %s", resultOf(ula.ResultCode, ula.ExperimentalResult))
		if ula.ResultCode.V != 2001 || ula.SubscriptionData == nil {
			t.Errorf("ULA: no Subscription-Data: %+v", ula)
		}
	}

	if aia, ok := p.exchange("aia-user-unknown", air(hssUnknownIMSI)).(*s6a.AuthenticationInformationAnswer); ok {
		t.Logf("AIA (unknown user): %s", resultOf(aia.ResultCode, aia.ExperimentalResult))
		if er := aia.ExperimentalResult; er == nil || er.ExperimentalResultCode != 5001 {
			t.Errorf("AIA for an unknown user: want Experimental-Result 5001, got %s", resultOf(aia.ResultCode, er))
		}
	}

	if pua, ok := p.exchange("pua", &s6a.PurgeUeRequest{
		Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai(),
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
		UserName: hssIMSI,
	}).(*s6a.PurgeUeAnswer); ok {
		t.Logf("PUA: %s", resultOf(pua.ResultCode, pua.ExperimentalResult))
	}

	p.writeSource(open5gsSource(cea, "TestPeerS6a",
		"aia.bin (AIA, one E-UTRAN vector), ula.bin (ULA with Subscription-Data),\n          aia-user-unknown.bin (AIA, Experimental-Result 5001), pua.bin (PUA)"))
}

// cxSuccess reports whether an answer carries a success code (RFC 6733
// §7.1.2, TS 29.229 §6.2.1).
func cxSuccess(rc diam.Opt[uint32], er *base.ExperimentalResult) bool {
	if rc.Set {
		return rc.V/1000 == 2
	}
	return er != nil && er.ExperimentalResultCode/1000 == 2
}

// TestPeerCx acts as an I-CSCF and S-CSCF towards the Open5GS HSS at
// DIAM_HSS_ADDR: UAR/UAA, MAR/MAA, SAR/SAA and LIR/LIA for the IMS identities
// of the provisioned subscriber.
//
// TS 23.003 §13.3 and §13.4B derive the private and public identities from
// the IMSI when there is no ISIM; an HSS may provision others (MSISDN-based
// ones). The test first probes which pair the HSS knows with a UAR, then
// runs the sequence with it.
func TestPeerCx(t *testing.T) {
	const host, realm = "scscf.test", "test"
	p := dialPeer(t, "DIAM_HSS_ADDR", host, realm, "cx")
	cea := p.capabilities("cea", hssCER(p))
	hss := string(cea.OriginHost)
	vsai := base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(cxApp))}
	serverName := "sip:" + host + ":6060"
	const msisdn = "0000000001"
	uar := func(impi, impu string) *cx.UserAuthorizationRequest {
		return &cx.UserAuthorizationRequest{
			Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai,
			AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			UserName: impi, PublicIdentity: impu, VisitedNetworkIdentifier: []byte(hssIMSDomain),
		}
	}

	type ids struct{ impi, impu string }
	candidates := []ids{
		{hssIMSI + "@" + hssIMSDomain, "sip:" + hssIMSI + "@" + hssIMSDomain},
		{hssIMSI + "@" + hssIMSDomain, "sip:" + msisdn + "@" + hssIMSDomain},
		{hssIMSI + "@" + hssIMSDomain, "tel:" + msisdn},
		{hssIMSI + "@" + hssIMSDomain, "tel:+" + msisdn},
		{hssIMSI, "sip:" + hssIMSI + "@" + hssIMSDomain},
		{msisdn + "@" + hssIMSDomain, "sip:" + msisdn + "@" + hssIMSDomain},
	}
	id := candidates[0]
	for i, c := range candidates {
		if uaa, ok := p.probe(fmt.Sprintf("uaa-candidate-%d", i), uar(c.impi, c.impu)).(*cx.UserAuthorizationAnswer); ok {
			t.Logf("UAA for User-Name %q Public-Identity %q: %s", c.impi, c.impu, resultOf(uaa.ResultCode, uaa.ExperimentalResult))
			if cxSuccess(uaa.ResultCode, uaa.ExperimentalResult) {
				id = c
				break
			}
		}
	}
	t.Logf("using User-Name %q Public-Identity %q", id.impi, id.impu)

	if uaa, ok := p.exchange("uaa", uar(id.impi, id.impu)).(*cx.UserAuthorizationAnswer); ok {
		t.Logf("UAA: %s", resultOf(uaa.ResultCode, uaa.ExperimentalResult))
	}

	if maa, ok := p.exchange("maa", &cx.MultimediaAuthRequest{
		Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai,
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
		UserName: id.impi, PublicIdentity: id.impu,
		SipAuthDataItem:    cx.SipAuthDataItem{SipAuthenticationScheme: diam.Some("Digest-AKAv1-MD5")},
		SipNumberAuthItems: 1,
		ServerName:         serverName,
	}).(*cx.MultimediaAuthAnswer); ok {
		t.Logf("MAA: %s, %d SIP-Auth-Data-Item", resultOf(maa.ResultCode, maa.ExperimentalResult), len(maa.SipAuthDataItem))
	}

	if saa, ok := p.exchange("saa", &cx.ServerAssignmentRequest{
		Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai,
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
		UserName: diam.Some(id.impi), PublicIdentity: []string{id.impu},
		ServerName:               serverName,
		ServerAssignmentType:     cx.ServerAssignmentTypeRegistration,
		UserDataAlreadyAvailable: cx.UserDataAlreadyAvailableUserDataNotAvailable,
	}).(*cx.ServerAssignmentAnswer); ok {
		t.Logf("SAA: %s, User-Data %q", resultOf(saa.ResultCode, saa.ExperimentalResult), saa.UserData.V)
	}

	if lia, ok := p.exchange("lia", &cx.LocationInfoRequest{
		Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai,
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
		PublicIdentity: id.impu,
	}).(*cx.LocationInfoAnswer); ok {
		t.Logf("LIA: %s, Server-Name %q", resultOf(lia.ResultCode, lia.ExperimentalResult), lia.ServerName.V)
	}

	p.writeSource(open5gsSource(cea, "TestPeerCx", fmt.Sprintf(
		"uaa.bin (UAA), maa.bin (MAA), saa.bin (SAA), lia.bin (LIA), for\n          User-Name %q, Public-Identity %q", id.impi, id.impu)))
}
