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
	"github.com/hsdfat/go-diam/swx"
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
Command:  go test -tags peer -run '%s$' -update ./internal/interop/
Content:  %s

Generated in CI against a synthetic subscriber (IMSI %s, random test K and
OPc in ci/open5gs/subscriber.js); no operator traffic.
`, cea.OriginHost, cea.ProductName, cea.VendorId, cea.FirmwareRevision.V,
		os.Getenv("DIAM_PEER_SOFTWARE"), os.Getenv("DIAM_PEER_IMAGE"),
		time.Now().UTC().Format("2006-01-02"), run, what, hssIMSI)
}

// hssCER is the CER of a client of the HSS: S6a, Cx and SWx, all under 3GPP
// (TS 29.272 §7.1.8, TS 29.229 §5.6, TS 29.273 §8.1).
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
			{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(swxApp))},
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

// success reports whether an answer carries a success code (RFC 6733
// §7.1.2; TS 29.229 §6.2.1 and TS 29.272 §7.4.2 for Experimental-Result).
func success(rc diam.Opt[uint32], er *base.ExperimentalResult) bool {
	if rc.Set {
		return rc.V/1000 == 2
	}
	return er != nil && er.ExperimentalResultCode/1000 == 2
}

// TestPeerS6a acts as two MMEs towards the Open5GS HSS at DIAM_HSS_ADDR.
// The first sends AIR and ULR for the provisioned subscriber and an AIR for
// an unknown one. The second then sends a ULR for the same subscriber, which
// makes the HSS cancel the first with a CLR (TS 29.272 §5.2.1.1.3), and a
// PUR. A NOR is only probed.
func TestPeerS6a(t *testing.T) {
	const realm = "test"
	p := dialPeer(t, "DIAM_HSS_ADDR", "mme.test", realm, "s6a")
	cea := p.capabilities("cea", hssCER(p))
	hss := string(cea.OriginHost)
	vsai := func() *base.VendorSpecificApplicationId {
		return &base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(s6aApp))}
	}
	air := func(imsi string) *s6a.AuthenticationInformationRequest {
		return &s6a.AuthenticationInformationRequest{
			Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai(),
			AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost:       p.host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			UserName: imsi,
			RequestedEutranAuthenticationInfo: &s6a.RequestedEutranAuthenticationInfo{
				NumberOfRequestedVectors:   diam.Some(uint32(1)),
				ImmediateResponsePreferred: diam.Some(uint32(1)),
			},
			VisitedPlmnId: testPLMN,
		}
	}
	ulr := func(q *peerConn) *s6a.UpdateLocationRequest {
		return &s6a.UpdateLocationRequest{
			Header: q.next(), SessionId: q.sessionID(), VendorSpecificApplicationId: vsai(),
			AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost:       q.host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			UserName:      hssIMSI,
			RatType:       s6a.RatTypeEutran,
			UlrFlags:      1<<1 | 1<<5, // S6a/S6d-Indicator, Initial-Attach-Indicator (TS 29.272 §7.3.7)
			VisitedPlmnId: testPLMN,
		}
	}

	if aia, ok := p.exchange("aia", air(hssIMSI)).(*s6a.AuthenticationInformationAnswer); ok {
		t.Logf("AIA: %s", resultOf(aia.ResultCode, aia.ExperimentalResult))
		if aia.ResultCode.V != 2001 || aia.AuthenticationInfo == nil || len(aia.AuthenticationInfo.EUtranVector) == 0 {
			t.Errorf("AIA: no E-UTRAN vector: %+v", aia)
		}
	}

	if ula, ok := p.exchange("ula", ulr(p)).(*s6a.UpdateLocationAnswer); ok {
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

	// A second MME registers the same subscriber. The CLR to the first one
	// may come before or after the ULA, so it is read concurrently.
	type result struct {
		frame []byte
		err   error
	}
	clr := make(chan result, 1)
	go func() {
		f, err := p.read(func(h diam.Header) bool { return h.CommandCode == 317 && h.Flags&diam.FlagRequest != 0 })
		clr <- result{f, err}
	}()
	p2 := dialPeer(t, "DIAM_HSS_ADDR", "mme2.test", realm, "s6a")
	p2.id = 100
	cer2 := hssCER(p2)
	cer2.Header = p2.next()
	if _, ok := p2.probe("cea-second-mme", cer2).(*base.CapabilitiesExchangeAnswer); !ok {
		t.Fatal("second MME: no valid CEA")
	}
	if ula, ok := p2.probe("ula-second-mme", ulr(p2)).(*s6a.UpdateLocationAnswer); ok {
		t.Logf("ULA (second MME): %s", resultOf(ula.ResultCode, ula.ExperimentalResult))
	}
	select {
	case r := <-clr:
		if r.err != nil {
			t.Errorf("waiting for a CLR: %v", r.err)
			break
		}
		if req, ok := p.check("clr", r.frame).(*s6a.CancelLocationRequest); ok {
			t.Logf("CLR: Cancellation-Type %d, CLR-Flags %v", req.CancellationType, req.ClrFlags)
			cla := s6a.NewCancelLocationAnswerFor(req)
			cla.VendorSpecificApplicationId = req.VendorSpecificApplicationId
			cla.ResultCode = diam.Some(uint32(2001))
			cla.AuthSessionState = base.AuthSessionStateNoStateMaintained
			cla.OriginHost, cla.OriginRealm = p.host, realm
			p.send(cla)
		}
	case <-time.After(10 * time.Second):
		t.Error("no CLR from the HSS within 10 s of the second ULR")
	}

	if pua, ok := p2.exchange("pua", &s6a.PurgeUeRequest{
		Header: p2.next(), SessionId: p2.sessionID(), VendorSpecificApplicationId: vsai(),
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       p2.host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
		UserName: hssIMSI,
	}).(*s6a.PurgeUeAnswer); ok {
		t.Logf("PUA: %s", resultOf(pua.ResultCode, pua.ExperimentalResult))
	}

	// Open5GS answers NOR with 3001 (E bit) and clears the P bit: kept as a
	// deviation (docs/design.md §14).
	if noa, ok := p2.deviant("noa", &s6a.NotifyRequest{
		Header: p2.next(), SessionId: p2.sessionID(), VendorSpecificApplicationId: vsai(),
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       p2.host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
		UserName: hssIMSI,
		NorFlags: diam.Some(uint32(1 << 3)), // UE Reachable from MME (TS 29.272 table 7.3.49/1)
	}).(*s6a.NotifyAnswer); ok {
		t.Logf("NOA: %s, flags %#x", resultOf(noa.ResultCode, noa.ExperimentalResult), noa.Header.Flags)
	}

	p.deviations = p2.deviations
	p.writeSource(open5gsSource(cea, "TestPeerS6a",
		"cea.bin (CEA), aia.bin (AIA, one E-UTRAN vector), ula.bin (ULA with\n"+
			"          Subscription-Data), aia-user-unknown.bin (AIA, Experimental-Result\n"+
			"          5001), clr.bin (CLR the HSS sent to the first MME after a second\n"+
			"          MME's ULR), pua.bin (PUA). In testdata/deviations/s6a,\n"+
			"          noa.bin is the 3001 error answer to a NOR, with the P bit cleared."))
}

// TestPeerCx acts as an I-CSCF and S-CSCF towards the Open5GS HSS at
// DIAM_HSS_ADDR: UAR/UAA, MAR/MAA, SAR/SAA and LIR/LIA for the provisioned
// subscriber.
//
// The private identity is the bare IMSI: Open5GS v2.8.0 answers a UAR whose
// User-Name is the IMSI-derived NAI of TS 23.003 §13.3
// (<IMSI>@ims.mnc<MNC>.mcc<MCC>.3gppnetwork.org) with Experimental-Result
// 5005 and logs "Invalid User-Name BCD". That UAR is probed and logged.
func TestPeerCx(t *testing.T) {
	const host, realm = "scscf.test", "test"
	p := dialPeer(t, "DIAM_HSS_ADDR", host, realm, "cx")
	cea := p.capabilities("cea", hssCER(p))
	hss := string(cea.OriginHost)
	vsai := base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(cxApp))}
	serverName := "sip:" + host + ":6060"
	impi := hssIMSI
	impu := "sip:" + hssIMSI + "@" + hssIMSDomain
	uar := func(impi string) *cx.UserAuthorizationRequest {
		return &cx.UserAuthorizationRequest{
			Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai,
			AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			UserName: impi, PublicIdentity: impu, VisitedNetworkIdentifier: []byte(hssIMSDomain),
		}
	}

	if uaa, ok := p.probe("uaa-nai", uar(hssIMSI+"@"+hssIMSDomain)).(*cx.UserAuthorizationAnswer); ok {
		t.Logf("UAA for the NAI form of the private identity: %s", resultOf(uaa.ResultCode, uaa.ExperimentalResult))
	}

	if uaa, ok := p.exchange("uaa", uar(impi)).(*cx.UserAuthorizationAnswer); ok {
		t.Logf("UAA: %s", resultOf(uaa.ResultCode, uaa.ExperimentalResult))
		if !success(uaa.ResultCode, uaa.ExperimentalResult) {
			t.Errorf("UAA: %s", resultOf(uaa.ResultCode, uaa.ExperimentalResult))
		}
	}

	if maa, ok := p.exchange("maa", &cx.MultimediaAuthRequest{
		Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai,
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
		UserName: impi, PublicIdentity: impu,
		SipAuthDataItem:    cx.SipAuthDataItem{SipAuthenticationScheme: diam.Some("Digest-AKAv1-MD5")},
		SipNumberAuthItems: 1,
		ServerName:         serverName,
	}).(*cx.MultimediaAuthAnswer); ok {
		t.Logf("MAA: %s, %d SIP-Auth-Data-Item", resultOf(maa.ResultCode, maa.ExperimentalResult), len(maa.SipAuthDataItem))
		if !success(maa.ResultCode, maa.ExperimentalResult) || len(maa.SipAuthDataItem) == 0 {
			t.Errorf("MAA: no SIP-Auth-Data-Item: %s", resultOf(maa.ResultCode, maa.ExperimentalResult))
		}
	}

	if saa, ok := p.exchange("saa", &cx.ServerAssignmentRequest{
		Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai,
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
		UserName: diam.Some(impi), PublicIdentity: []string{impu},
		ServerName:               serverName,
		ServerAssignmentType:     cx.ServerAssignmentTypeRegistration,
		UserDataAlreadyAvailable: cx.UserDataAlreadyAvailableUserDataNotAvailable,
	}).(*cx.ServerAssignmentAnswer); ok {
		t.Logf("SAA: %s, User-Data %q", resultOf(saa.ResultCode, saa.ExperimentalResult), saa.UserData.V)
		if !success(saa.ResultCode, saa.ExperimentalResult) || !saa.UserData.Set {
			t.Errorf("SAA: no User-Data: %s", resultOf(saa.ResultCode, saa.ExperimentalResult))
		}
	}

	if lia, ok := p.exchange("lia", &cx.LocationInfoRequest{
		Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai,
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
		PublicIdentity: impu,
	}).(*cx.LocationInfoAnswer); ok {
		t.Logf("LIA: %s, Server-Name %q", resultOf(lia.ResultCode, lia.ExperimentalResult), lia.ServerName.V)
		if !success(lia.ResultCode, lia.ExperimentalResult) {
			t.Errorf("LIA: %s", resultOf(lia.ResultCode, lia.ExperimentalResult))
		}
	}

	p.writeSource(open5gsSource(cea, "TestPeerCx", fmt.Sprintf(
		"cea.bin (CEA), uaa.bin (UAA), maa.bin (MAA, one Digest-AKAv1-MD5\n"+
			"          SIP-Auth-Data-Item), saa.bin (SAA with User-Data), lia.bin (LIA),\n"+
			"          for User-Name %q, Public-Identity %q", impi, impu)))
}

// TestPeerSWx acts as a 3GPP AAA server towards the Open5GS HSS at
// DIAM_HSS_ADDR: MAR/MAA for an EAP-AKA' vector and SAR/SAA for the
// provisioned subscriber (TS 29.273 §8.1.2.1, §8.1.2.2). PPR and RTR are
// HSS-initiated and not triggered here.
func TestPeerSWx(t *testing.T) {
	const host, realm = "aaa.test", "test"
	p := dialPeer(t, "DIAM_HSS_ADDR", host, realm, "swx")
	cea := p.capabilities("cea", hssCER(p))
	hss := string(cea.OriginHost)
	vsai := base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(swxApp))}

	mar := func(scheme string, anid diam.Opt[string]) *swx.MultimediaAuthRequest {
		return &swx.MultimediaAuthRequest{
			Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai,
			AuthSessionState: base.AuthSessionStateNoStateMaintained,
			OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
			UserName:           hssIMSI,
			RatType:            diam.Some(s6a.RatTypeWlan),
			Anid:               anid,
			SipAuthDataItem:    swx.SipAuthDataItem{SipAuthenticationScheme: diam.Some(scheme)},
			SipNumberAuthItems: 1,
		}
	}
	// TS 29.273 §8.1.2.1: the method is EAP-AKA or EAP-AKA' (TS 33.402), and
	// ANID (TS 24.302 §8.1.1.2, "WLAN" for a WLAN) comes with EAP-AKA'. The
	// first run of the HSS answered "EAP-AKA'" with 5006, so the scheme
	// strings are probed first.
	type method struct {
		scheme string
		anid   diam.Opt[string]
	}
	use := method{"EAP-AKA'", diam.Some("WLAN")}
	for i, c := range []method{use, {"EAP-AKA", diam.Opt[string]{}}, {"EAP-AKA'", diam.Opt[string]{}}, {"Digest-AKAv1-MD5", diam.Opt[string]{}}} {
		if maa, ok := p.probe(fmt.Sprintf("maa-candidate-%d", i), mar(c.scheme, c.anid)).(*swx.MultimediaAuthAnswer); ok {
			t.Logf("MAA for scheme %q, ANID %v: %s", c.scheme, c.anid, resultOf(maa.ResultCode, maa.ExperimentalResult))
			if success(maa.ResultCode, maa.ExperimentalResult) {
				use = c
				break
			}
		}
	}
	if maa, ok := p.exchange("maa", mar(use.scheme, use.anid)).(*swx.MultimediaAuthAnswer); ok {
		t.Logf("MAA: %s, %d SIP-Auth-Data-Item", resultOf(maa.ResultCode, maa.ExperimentalResult), len(maa.SipAuthDataItem))
		if !success(maa.ResultCode, maa.ExperimentalResult) || len(maa.SipAuthDataItem) == 0 {
			t.Errorf("MAA: no SIP-Auth-Data-Item: %s", resultOf(maa.ResultCode, maa.ExperimentalResult))
		}
	}

	if saa, ok := p.exchange("saa", &swx.ServerAssignmentRequest{
		Header: p.next(), SessionId: p.sessionID(), VendorSpecificApplicationId: vsai,
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       host, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
		UserName:             hssIMSI,
		ServerAssignmentType: cx.ServerAssignmentTypeRegistration,
	}).(*swx.ServerAssignmentAnswer); ok {
		t.Logf("SAA: %s, Non-3GPP-User-Data present: %v", resultOf(saa.ResultCode, saa.ExperimentalResult), saa.Non3GppUserData != nil)
		if !success(saa.ResultCode, saa.ExperimentalResult) {
			t.Errorf("SAA: %s", resultOf(saa.ResultCode, saa.ExperimentalResult))
		}
	}

	p.writeSource(open5gsSource(cea, "TestPeerSWx",
		fmt.Sprintf("cea.bin (CEA), maa.bin (MAA, scheme %q), saa.bin (SAA), for\n"+
			"          User-Name %q", use.scheme, hssIMSI)))
}
