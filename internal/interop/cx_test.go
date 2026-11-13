package interop

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/cx"
	"github.com/hsdfat/go-diam/diam"
)

// cxApp is the Cx/Dx application id (TS 29.229 §6).
const cxApp = 16777216

// go-diameter v4.6.0 ships no Cx/Dx application: no 16777216 dictionary and
// no Cx commands. What it does have is the definition (code, vendor, flags,
// type) of most Cx AVPs under the applications that re-use them: SWx
// (SIP-Auth-Data-Item and its AVPs, Server-Assignment-Type,
// Deregistration-Reason), Ro/Rf (Server-Name, Server-Capabilities,
// User-Data, Session-Priority), NASREQ (Framed-*), S6a, Rx and the base
// protocol (OC-*, DRMP, Load). The Cx interop below is built on those.
//
// The AVPs go-diameter has nowhere are listed in cxRestated with their code,
// vendor and M-bit restated from TS 29.229 here, in the test, not read from
// go-diam's schema. On the go-diameter side they are still encoded by
// go-diameter's own data types, so their framing, padding and value encoding
// is checked, but their code and flags are only as independent as this
// table. TestFiorixCxGaps keeps the list exact.
type restated struct {
	code, vendor uint32
	m            bool
	clause       string
}

var cxRestated = map[string]restated{
	"Public-Identity":                             {601, tgpp, true, "table 6.3.0.1"},
	"SIP-Authentication-Context":                  {611, tgpp, true, "table 6.3.0.1"},
	"Charging-Information":                        {618, tgpp, true, "table 6.3.0.1"},
	"Primary-Event-Charging-Function-Name":        {619, tgpp, true, "table 6.3.0.1"},
	"Secondary-Event-Charging-Function-Name":      {620, tgpp, true, "table 6.3.0.1"},
	"Primary-Charging-Collection-Function-Name":   {621, tgpp, true, "table 6.3.0.1"},
	"Secondary-Charging-Collection-Function-Name": {622, tgpp, true, "table 6.3.0.1"},
	"User-Authorization-Type":                     {623, tgpp, true, "table 6.3.0.1"},
	"User-Data-Already-Available":                 {624, tgpp, true, "table 6.3.0.1"},
	"Associated-Identities":                       {632, tgpp, false, "table 6.3.0.1"},
	"Originating-Request":                         {633, tgpp, true, "table 6.3.0.1"},
	"Wildcarded-Public-Identity":                  {634, tgpp, false, "table 6.3.0.1"},
	"SIP-Digest-Authenticate":                     {635, tgpp, false, "table 6.3.0.1"},
	"UAR-Flags":                                   {637, tgpp, false, "table 6.3.0.1"},
	"Loose-Route-Indication":                      {638, tgpp, false, "table 6.3.0.1"},
	"SCSCF-Restoration-Info":                      {639, tgpp, false, "table 6.3.0.1"},
	"Path":                                        {640, tgpp, false, "table 6.3.0.1"},
	"Contact":                                     {641, tgpp, false, "table 6.3.0.1"},
	"Subscription-Info":                           {642, tgpp, false, "table 6.3.0.1"},
	"Call-ID-SIP-Header":                          {643, tgpp, false, "table 6.3.0.1"},
	"From-SIP-Header":                             {644, tgpp, false, "table 6.3.0.1"},
	"To-SIP-Header":                               {645, tgpp, false, "table 6.3.0.1"},
	"Record-Route":                                {646, tgpp, false, "table 6.3.0.1"},
	"Associated-Registered-Identities":            {647, tgpp, false, "table 6.3.0.1"},
	"Multiple-Registration-Indication":            {648, tgpp, false, "table 6.3.0.1"},
	"Restoration-Info":                            {649, tgpp, false, "table 6.3.0.1"},
	"Identity-with-Emergency-Registration":        {651, tgpp, false, "table 6.3.0.1"},
	"Priviledged-Sender-Indication":               {652, tgpp, false, "table 6.3.0.1"},
	"LIA-Flags":                                   {653, tgpp, false, "table 6.3.0.1"},
	"Initial-CSeq-Sequence-Number":                {654, tgpp, false, "table 6.3.0.1"},
	"SAR-Flags":                                   {655, tgpp, false, "table 6.3.0.1"},
	"Allowed-WAF-WWSF-Identities":                 {656, tgpp, false, "table 6.3.0.1"},
	"WebRTC-Authentication-Function-Name":         {657, tgpp, false, "table 6.3.0.1"},
	"WebRTC-Web-Server-Function-Name":             {658, tgpp, false, "table 6.3.0.1"},
	"RTR-Flags":                                   {659, tgpp, false, "table 6.3.0.1"},
	"P-CSCF-Subscription-Info":                    {660, tgpp, false, "table 6.3.0.1"},
	"Registration-Time-Out":                       {661, tgpp, false, "table 6.3.0.1"},
	"Alternate-Digest-Algorithm":                  {662, tgpp, false, "table 6.3.0.1"},
	"Alternate-Digest-HA1":                        {663, tgpp, false, "table 6.3.0.1"},
	"Failed-PCSCF":                                {664, tgpp, false, "table 6.3.0.1"},
	"PCSCF-FQDN":                                  {665, tgpp, false, "table 6.3.0.1"},
	"PCSCF-IP-Address":                            {666, tgpp, false, "table 6.3.0.1"},
	"Digest-Realm":                                {104, 0, true, "table 6.3.0.1"},
	"Digest-QoP":                                  {110, 0, true, "table 6.3.0.1"},
	"Digest-Algorithm":                            {111, 0, true, "table 6.3.0.1"},
	"Digest-HA1":                                  {121, 0, true, "table 6.3.0.1"},
	"Line-Identifier":                             {500, 13019, false, "§6.3.42, no flag rule"},
}

// cxRequestM is the Cx M-bit where it depends on the message, not the AVP:
// Supported-Features is set in requests and clear in answers (TS 29.229
// §7.2.1). go-diameter's dictionary has "may M", so its own rule gives a
// clear M-bit; requests override it. Everything else about the AVP comes
// from go-diameter.
var cxRequestM = map[[2]uint32]bool{{avp.SupportedFeatures, tgpp}: true}

// fiorixHome returns the go-diameter application that defines an AVP: the
// base protocol if it does, otherwise the first application in go-diameter's
// own order.
func fiorixHome(code, vendor uint32) (uint32, *dict.AVP) {
	if a, err := dict.Default.FindAVPByCode(0, code, vendor); err == nil {
		return 0, a
	}
	for _, app := range dict.Default.Apps() {
		if a, err := dict.Default.FindAVPByCode(app.ID, code, vendor); err == nil {
			return app.ID, a
		}
	}
	return 0, nil
}

// perAVP builds the go-diameter AVPs of one message of an application that
// go-diameter has no dictionary for (Cx, Sh): restated holds the AVPs it
// defines nowhere, requestM the M-bit rules that depend on the message.
type perAVP struct {
	request  bool
	restated map[string]restated
	requestM map[[2]uint32]bool
}

// a builds an AVP that go-diameter defines: code from its constants, flags
// from its dictionary (with the per-message rule of requestM).
func (c perAVP) a(code, vendor uint32, data datatype.Type) *fd.AVP {
	_, def := fiorixHome(code, vendor)
	if def == nil {
		panic(fmt.Sprintf("AVP %d vendor %d is in no go-diameter application: add it to the restated list", code, vendor))
	}
	var flags uint8
	if strings.Contains(def.Must, "M") {
		flags = avp.Mbit
	}
	if m, ok := c.requestM[[2]uint32{code, vendor}]; ok && c.request {
		flags = 0
		if m {
			flags = avp.Mbit
		}
	}
	return fd.NewAVP(code, flags, vendor, data)
}

// r builds an AVP that go-diameter does not define, from the restated list.
func (c perAVP) r(name string, data datatype.Type) *fd.AVP {
	d, ok := c.restated[name]
	if !ok {
		panic("not in the restated list: " + name)
	}
	var flags uint8
	if d.m {
		flags = avp.Mbit
	}
	return fd.NewAVP(d.code, flags, d.vendor, data)
}

func (c perAVP) g(code, vendor uint32, avps ...*fd.AVP) *fd.AVP {
	return c.a(code, vendor, &fd.GroupedAVP{AVP: avps})
}

func (c perAVP) rg(name string, avps ...*fd.AVP) *fd.AVP {
	return c.r(name, &fd.GroupedAVP{AVP: avps})
}

func ip(s string) datatype.Address {
	a := net.ParseIP(s)
	if v4 := a.To4(); v4 != nil {
		a = v4
	}
	return datatype.Address(a)
}

// fcx builds a go-diameter message of the Cx application.
func fcx(cmd uint32, flags uint8, avps ...*fd.AVP) *fd.Message {
	m := fd.NewMessage(cmd, flags, cxApp, hopByHop, endToEnd, dict.Default)
	for _, a := range avps {
		m.AddAVP(a)
	}
	return m
}

// cxPairs is one message per Cx/Dx command, built with go-diam and with
// go-diameter, covering every grouped AVP a command can carry.
func cxPairs() []pair {
	const (
		icscf, scscf, hss, realm = "icscf.ims.example.org", "scscf.ims.example.org", "hss.ims.example.org", "ims.example.org"
		impi                     = "001010000000001@ims.example.org"
		impu                     = "sip:+15551234567@ims.example.org"
		scscfURI                 = "sip:scscf.ims.example.org:5060"
		aka                      = "Digest-AKAv1-MD5"
		vni                      = "visited.example.net"
	)
	when := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	nsm := base.AuthSessionStateNoStateMaintained
	q, s := perAVP{true, cxRestated, cxRequestM}, perAVP{false, cxRestated, cxRequestM}

	session := func(v string) *fd.AVP { return fa(avp.SessionID, datatype.UTF8String(v)) }
	authState := func() *fd.AVP { return enum(avp.AuthSessionState, 1) }
	gVSA := base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(cxApp))}
	fVSA := func() *fd.AVP {
		return group(avp.VendorSpecificApplicationID, u32(avp.VendorID, tgpp), u32(avp.AuthApplicationID, cxApp))
	}
	gProxy := []base.ProxyInfo{{ProxyHost: "dra.ims.example.org", ProxyState: []byte{1, 2, 3}}}
	fProxy := func() *fd.AVP {
		return group(avp.ProxyInfo, ident(avp.ProxyHost, "dra.ims.example.org"), octets(avp.ProxyState, 1, 2, 3))
	}
	fRoute := func() *fd.AVP { return ident(avp.RouteRecord, "dra.ims.example.org") }
	gFailed := &base.FailedAvp{Unknown: []diam.RawAVP{{Code: base.AVPOriginStateId, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 9}}}}
	fFailed := func() *fd.AVP { return group(avp.FailedAVP, u32(avp.OriginStateID, 9)) }
	gExp := &base.ExperimentalResult{VendorId: tgpp, ExperimentalResultCode: 2001}
	fExp := func() *fd.AVP {
		return group(avp.ExperimentalResult, u32(avp.VendorID, tgpp), u32(avp.ExperimentalResultCode, 2001))
	}

	gFeatures := []cx.SupportedFeatures{{VendorId: tgpp, FeatureListId: 1, FeatureList: 0x0f}}
	fFeatures := func(b perAVP) *fd.AVP {
		return b.g(avp.SupportedFeatures, tgpp, b.a(avp.VendorID, 0, datatype.Unsigned32(tgpp)),
			b.a(avp.FeatureListID, tgpp, datatype.Unsigned32(1)), b.a(avp.FeatureList, tgpp, datatype.Unsigned32(0x0f)))
	}
	gOcSF := &cx.OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))}
	fOcSF := func() *fd.AVP {
		return s.g(avp.OCSupportedFeatures, 0, s.a(avp.OCFeatureVector, 0, datatype.Unsigned64(1)))
	}
	gOlr := &cx.OcOlr{OcSequenceNumber: 9, OcReportType: cx.OcReportTypeRealmReport,
		OcReductionPercentage: diam.Some(uint32(50)), OcValidityDuration: diam.Some(uint32(30))}
	fOlr := func() *fd.AVP {
		return s.g(avp.OCOLR, 0, s.a(avp.OCSequenceNumber, 0, datatype.Unsigned64(9)), s.a(avp.OCReportType, 0, datatype.Enumerated(1)),
			s.a(avp.OCReductionPercentage, 0, datatype.Unsigned32(50)), s.a(avp.OCValidityDuration, 0, datatype.Unsigned32(30)))
	}
	// Load-Value is left out: go-diameter types it Unsigned32, RFC 8583
	// §7.3 says Unsigned64 (see dictcheck-allow.txt).
	gLoad := []cx.Load{{LoadType: diam.Some(cx.LoadTypeHost), Sourceid: diam.Some(hss)}}
	fLoad := func() *fd.AVP {
		return s.g(avp.Load, 0, s.a(avp.LoadType, 0, datatype.Enumerated(0)), s.a(avp.SourceID, 0, datatype.DiameterIdentity(hss)))
	}
	gCaps := &cx.ServerCapabilities{MandatoryCapability: []uint32{1, 2}, OptionalCapability: []uint32{7}, ServerName: []string{scscfURI}}
	fCaps := func() *fd.AVP {
		return s.g(avp.ServerCapabilities, tgpp, s.a(avp.MandatoryCapability, tgpp, datatype.Unsigned32(1)),
			s.a(avp.MandatoryCapability, tgpp, datatype.Unsigned32(2)), s.a(avp.OptionalCapability, tgpp, datatype.Unsigned32(7)),
			s.a(avp.ServerName, tgpp, datatype.UTF8String(scscfURI)))
	}
	gCharging := &cx.ChargingInformation{
		PrimaryEventChargingFunctionName:        diam.Some("aaa://ocs1.ims.example.org"),
		SecondaryEventChargingFunctionName:      diam.Some("aaa://ocs2.ims.example.org"),
		PrimaryChargingCollectionFunctionName:   diam.Some("aaa://cdf1.ims.example.org"),
		SecondaryChargingCollectionFunctionName: diam.Some("aaa://cdf2.ims.example.org"),
	}
	fCharging := func() *fd.AVP {
		return s.rg("Charging-Information",
			s.r("Primary-Event-Charging-Function-Name", datatype.DiameterURI("aaa://ocs1.ims.example.org")),
			s.r("Secondary-Event-Charging-Function-Name", datatype.DiameterURI("aaa://ocs2.ims.example.org")),
			s.r("Primary-Charging-Collection-Function-Name", datatype.DiameterURI("aaa://cdf1.ims.example.org")),
			s.r("Secondary-Charging-Collection-Function-Name", datatype.DiameterURI("aaa://cdf2.ims.example.org")))
	}
	gWaf := &cx.AllowedWafWwsfIdentities{WebrtcAuthenticationFunctionName: []string{"waf.example.org"},
		WebrtcWebServerFunctionName: []string{"wwsf.example.org"}}
	fWaf := func() *fd.AVP {
		return s.rg("Allowed-WAF-WWSF-Identities", s.r("WebRTC-Authentication-Function-Name", datatype.UTF8String("waf.example.org")),
			s.r("WebRTC-Web-Server-Function-Name", datatype.UTF8String("wwsf.example.org")))
	}
	userData := []byte("<IMSSubscription><PrivateID>" + impi + "</PrivateID></IMSSubscription>")

	gRestoration := cx.ScscfRestorationInfo{
		UserName: impi,
		RestorationInfo: []cx.RestorationInfo{{
			Path: []byte("<sip:pcscf;lr>"), Contact: []byte("<sip:ue@192.0.2.50>"), InitialCseqSequenceNumber: diam.Some(uint32(1)),
			CallIdSipHeader:       diam.Some([]byte("cid")),
			SubscriptionInfo:      &cx.SubscriptionInfo{CallIdSipHeader: []byte("s1"), FromSipHeader: []byte("f"), ToSipHeader: []byte("t"), RecordRoute: []byte("rr"), Contact: []byte("c")},
			PCscfSubscriptionInfo: &cx.PCscfSubscriptionInfo{CallIdSipHeader: []byte("s2"), FromSipHeader: []byte("f"), ToSipHeader: []byte("t"), Contact: []byte("c")},
		}},
		RegistrationTimeOut: diam.Some(when), SipAuthenticationScheme: diam.Some(aka),
	}
	fRestoration := func() *fd.AVP {
		o := func(name, v string) *fd.AVP { return s.r(name, datatype.OctetString(v)) }
		return s.rg("SCSCF-Restoration-Info", utf8(avp.UserName, impi),
			s.rg("Restoration-Info", o("Path", "<sip:pcscf;lr>"), o("Contact", "<sip:ue@192.0.2.50>"),
				s.r("Initial-CSeq-Sequence-Number", datatype.Unsigned32(1)), o("Call-ID-SIP-Header", "cid"),
				s.rg("Subscription-Info", o("Call-ID-SIP-Header", "s1"), o("From-SIP-Header", "f"), o("To-SIP-Header", "t"),
					o("Record-Route", "rr"), o("Contact", "c")),
				s.rg("P-CSCF-Subscription-Info", o("Call-ID-SIP-Header", "s2"), o("From-SIP-Header", "f"), o("To-SIP-Header", "t"),
					o("Contact", "c"))),
			s.r("Registration-Time-Out", datatype.Time(when)),
			s.a(avp.SIPAuthenticationScheme, tgpp, datatype.UTF8String(aka)))
	}

	key16 := bytes.Repeat([]byte{0x5a}, 16)
	gAka := cx.SipAuthDataItem{SipItemNumber: diam.Some(uint32(1)), SipAuthenticationScheme: diam.Some(aka),
		SipAuthenticate: diam.Some(bytes.Repeat([]byte{0xa5}, 32)), SipAuthorization: diam.Some([]byte{1, 2, 3, 4, 5, 6, 7, 8}),
		ConfidentialityKey: diam.Some(key16), IntegrityKey: diam.Some(key16)}
	fAka := func() *fd.AVP {
		return s.g(avp.SIPAuthDataItem, tgpp, s.a(avp.SIPItemNumber, tgpp, datatype.Unsigned32(1)),
			s.a(avp.SIPAuthenticationScheme, tgpp, datatype.UTF8String(aka)),
			s.a(avp.SIPAuthenticate, tgpp, datatype.OctetString(bytes.Repeat([]byte{0xa5}, 32))),
			s.a(avp.SIPAuthorization, tgpp, datatype.OctetString([]byte{1, 2, 3, 4, 5, 6, 7, 8})),
			s.a(avp.ConfidentialityKey, tgpp, datatype.OctetString(key16)), s.a(avp.IntegrityKey, tgpp, datatype.OctetString(key16)))
	}
	gDigest := &cx.SipDigestAuthenticate{DigestRealm: realm, DigestAlgorithm: diam.Some("MD5"), DigestQop: "auth", DigestHa1: "939e7578",
		AlternateDigestAlgorithm: diam.Some("SHA-256"), AlternateDigestHa1: diam.Some("2b2d6c1d")}
	fDigest := func() *fd.AVP {
		return s.rg("SIP-Digest-Authenticate", s.r("Digest-Realm", datatype.UTF8String(realm)),
			s.r("Digest-Algorithm", datatype.UTF8String("MD5")), s.r("Digest-QoP", datatype.UTF8String("auth")),
			s.r("Digest-HA1", datatype.UTF8String("939e7578")), s.r("Alternate-Digest-Algorithm", datatype.UTF8String("SHA-256")),
			s.r("Alternate-Digest-HA1", datatype.UTF8String("2b2d6c1d")))
	}
	gNass := cx.SipAuthDataItem{SipAuthenticationScheme: diam.Some("NASS-Bundled"), SipAuthenticationContext: diam.Some([]byte("REGISTER")),
		FramedIpAddress: diam.Some([]byte{192, 0, 2, 50}), FramedIpv6Prefix: diam.Some([]byte{0, 64, 0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0}),
		FramedInterfaceId: diam.Some(uint64(0x02005efffe005301)), LineIdentifier: [][]byte{[]byte("line-1")}}
	fNass := func() *fd.AVP {
		return s.g(avp.SIPAuthDataItem, tgpp, s.a(avp.SIPAuthenticationScheme, tgpp, datatype.UTF8String("NASS-Bundled")),
			s.r("SIP-Authentication-Context", datatype.OctetString("REGISTER")),
			s.a(avp.FramedIPAddress, 0, datatype.OctetString([]byte{192, 0, 2, 50})),
			s.a(avp.FramedIPv6Prefix, 0, datatype.OctetString([]byte{0, 64, 0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0})),
			s.a(avp.FramedInterfaceID, 0, datatype.Unsigned64(0x02005efffe005301)),
			s.r("Line-Identifier", datatype.OctetString("line-1")))
	}

	const req, pxy = fd.RequestFlag, fd.ProxiableFlag
	return []pair{
		{"UAR",
			&cx.UserAuthorizationRequest{
				Header: hdr, SessionId: "icscf;1;1", Drmp: diam.Some(cx.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				AuthSessionState: nsm, OriginHost: icscf, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
				UserName: impi, OcSupportedFeatures: gOcSF, SupportedFeatures: gFeatures, PublicIdentity: impu,
				VisitedNetworkIdentifier: []byte(vni), UserAuthorizationType: diam.Some(cx.UserAuthorizationTypeRegistrationAndCapabilities),
				UarFlags: diam.Some(uint32(1)), ProxyInfo: gProxy, RouteRecord: []string{"dra.ims.example.org"},
			},
			fcx(300, req|pxy, session("icscf;1;1"), q.a(avp.DRMP, 0, datatype.Enumerated(5)), fVSA(), authState(),
				ident(avp.OriginHost, icscf), ident(avp.OriginRealm, realm), ident(avp.DestinationHost, hss),
				ident(avp.DestinationRealm, realm), utf8(avp.UserName, impi), fOcSF(), fFeatures(q),
				q.r("Public-Identity", datatype.UTF8String(impu)), q.a(avp.VisitedNetworkIdentifier, tgpp, datatype.OctetString(vni)),
				q.r("User-Authorization-Type", datatype.Enumerated(2)), q.r("UAR-Flags", datatype.Unsigned32(1)), fProxy(), fRoute()),
		},
		{"UAA",
			&cx.UserAuthorizationAnswer{
				Header: hdr, SessionId: "icscf;1;1", VendorSpecificApplicationId: gVSA, ExperimentalResult: gExp, AuthSessionState: nsm,
				OriginHost: hss, OriginRealm: realm, OcSupportedFeatures: gOcSF, OcOlr: gOlr, Load: gLoad, SupportedFeatures: gFeatures,
				ServerName: diam.Some(scscfURI), ServerCapabilities: gCaps, FailedAvp: gFailed, ProxyInfo: gProxy,
				RouteRecord: []string{"dra.ims.example.org"},
			},
			fcx(300, pxy, session("icscf;1;1"), fVSA(), fExp(), authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				fOcSF(), fOlr(), fLoad(), fFeatures(s), s.a(avp.ServerName, tgpp, datatype.UTF8String(scscfURI)), fCaps(),
				fFailed(), fProxy(), fRoute()),
		},
		{"SAR",
			&cx.ServerAssignmentRequest{
				Header: hdr, SessionId: "scscf;1;2", VendorSpecificApplicationId: gVSA, AuthSessionState: nsm, OriginHost: scscf,
				OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm, UserName: diam.Some(impi),
				SupportedFeatures: gFeatures, PublicIdentity: []string{impu, "tel:+15551234567"},
				WildcardedPublicIdentity: diam.Some("sip:room-!.*!@ims.example.org"), ServerName: scscfURI,
				ServerAssignmentType: cx.ServerAssignmentTypeRegistration, UserDataAlreadyAvailable: cx.UserDataAlreadyAvailableUserDataNotAvailable,
				ScscfRestorationInfo: &gRestoration, MultipleRegistrationIndication: diam.Some(cx.MultipleRegistrationIndicationMultipleRegistration),
				SessionPriority: diam.Some(cx.SessionPriorityPriority1), SarFlags: diam.Some(uint32(1)),
				FailedPcscf: &cx.FailedPcscf{PcscfFqdn: diam.Some("pcscf.ims.example.org"),
					PcscfIpAddress: []netip.Addr{netip.MustParseAddr("192.0.2.40"), netip.MustParseAddr("2001:db8::40")}},
				ProxyInfo: gProxy, RouteRecord: []string{"dra.ims.example.org"},
			},
			fcx(301, req|pxy, session("scscf;1;2"), fVSA(), authState(), ident(avp.OriginHost, scscf), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, hss), ident(avp.DestinationRealm, realm), utf8(avp.UserName, impi), fFeatures(q),
				q.r("Public-Identity", datatype.UTF8String(impu)), q.r("Public-Identity", datatype.UTF8String("tel:+15551234567")),
				q.r("Wildcarded-Public-Identity", datatype.UTF8String("sip:room-!.*!@ims.example.org")),
				q.a(avp.ServerName, tgpp, datatype.UTF8String(scscfURI)), q.a(avp.ServerAssignmentType, tgpp, datatype.Enumerated(1)),
				q.r("User-Data-Already-Available", datatype.Enumerated(0)), fRestoration(),
				q.r("Multiple-Registration-Indication", datatype.Enumerated(1)), q.a(avp.SessionPriority, tgpp, datatype.Enumerated(1)),
				q.r("SAR-Flags", datatype.Unsigned32(1)),
				q.rg("Failed-PCSCF", q.r("PCSCF-FQDN", datatype.DiameterIdentity("pcscf.ims.example.org")),
					q.r("PCSCF-IP-Address", ip("192.0.2.40")), q.r("PCSCF-IP-Address", ip("2001:db8::40"))),
				fProxy(), fRoute()),
		},
		{"SAA",
			&cx.ServerAssignmentAnswer{
				Header: hdr, SessionId: "scscf;1;2", VendorSpecificApplicationId: gVSA, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, UserName: diam.Some(impi), OcOlr: gOlr,
				SupportedFeatures: gFeatures, UserData: diam.Some(userData), ChargingInformation: gCharging,
				AssociatedIdentities:           &cx.AssociatedIdentities{UserName: []string{impi}},
				LooseRouteIndication:           diam.Some(cx.LooseRouteIndicationLooseRouteRequired),
				ScscfRestorationInfo:           []cx.ScscfRestorationInfo{gRestoration},
				AssociatedRegisteredIdentities: &cx.AssociatedRegisteredIdentities{UserName: []string{impi}},
				ServerName:                     diam.Some(scscfURI), WildcardedPublicIdentity: diam.Some("sip:room-!.*!@ims.example.org"),
				PriviledgedSenderIndication: diam.Some(cx.PriviledgedSenderIndicationPriviledgedSender),
				AllowedWafWwsfIdentities:    gWaf, FailedAvp: gFailed, ProxyInfo: gProxy,
			},
			fcx(301, pxy, session("scscf;1;2"), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), utf8(avp.UserName, impi), fOlr(), fFeatures(s),
				s.a(avp.UserData, tgpp, datatype.OctetString(userData)), fCharging(),
				s.rg("Associated-Identities", utf8(avp.UserName, impi)), s.r("Loose-Route-Indication", datatype.Enumerated(1)),
				fRestoration(), s.rg("Associated-Registered-Identities", utf8(avp.UserName, impi)),
				s.a(avp.ServerName, tgpp, datatype.UTF8String(scscfURI)),
				s.r("Wildcarded-Public-Identity", datatype.UTF8String("sip:room-!.*!@ims.example.org")),
				s.r("Priviledged-Sender-Indication", datatype.Enumerated(1)), fWaf(), fFailed(), fProxy()),
		},
		{"LIR",
			&cx.LocationInfoRequest{
				Header: hdr, SessionId: "icscf;1;3", VendorSpecificApplicationId: gVSA, AuthSessionState: nsm, OriginHost: icscf,
				OriginRealm: realm, DestinationRealm: realm, OriginatingRequest: diam.Some(cx.OriginatingRequestOriginating),
				OcSupportedFeatures: gOcSF, SupportedFeatures: gFeatures, PublicIdentity: impu,
				UserAuthorizationType: diam.Some(cx.UserAuthorizationTypeRegistration), SessionPriority: diam.Some(cx.SessionPriorityPriority0),
				ProxyInfo: gProxy, RouteRecord: []string{"dra.ims.example.org"},
			},
			fcx(302, req|pxy, session("icscf;1;3"), fVSA(), authState(), ident(avp.OriginHost, icscf), ident(avp.OriginRealm, realm),
				ident(avp.DestinationRealm, realm), q.r("Originating-Request", datatype.Enumerated(0)), fOcSF(), fFeatures(q),
				q.r("Public-Identity", datatype.UTF8String(impu)), q.r("User-Authorization-Type", datatype.Enumerated(0)),
				q.a(avp.SessionPriority, tgpp, datatype.Enumerated(0)), fProxy(), fRoute()),
		},
		{"LIA",
			&cx.LocationInfoAnswer{
				Header: hdr, SessionId: "icscf;1;3", VendorSpecificApplicationId: gVSA, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, Load: gLoad, SupportedFeatures: gFeatures,
				ServerName: diam.Some(scscfURI), ServerCapabilities: gCaps, WildcardedPublicIdentity: diam.Some("sip:room-!.*!@ims.example.org"),
				LiaFlags: diam.Some(uint32(1)), FailedAvp: gFailed, ProxyInfo: gProxy,
			},
			fcx(302, pxy, session("icscf;1;3"), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), fLoad(), fFeatures(s), s.a(avp.ServerName, tgpp, datatype.UTF8String(scscfURI)), fCaps(),
				s.r("Wildcarded-Public-Identity", datatype.UTF8String("sip:room-!.*!@ims.example.org")),
				s.r("LIA-Flags", datatype.Unsigned32(1)), fFailed(), fProxy()),
		},
		{"MAR",
			&cx.MultimediaAuthRequest{
				Header: hdr, SessionId: "scscf;1;4", VendorSpecificApplicationId: gVSA, AuthSessionState: nsm, OriginHost: scscf,
				OriginRealm: realm, DestinationRealm: realm, DestinationHost: diam.Some(hss), UserName: impi,
				SupportedFeatures: gFeatures, PublicIdentity: impu,
				SipAuthDataItem:    cx.SipAuthDataItem{SipAuthenticationScheme: diam.Some(aka), SipAuthorization: diam.Some(make([]byte, 30))},
				SipNumberAuthItems: 1, ServerName: scscfURI, ProxyInfo: gProxy, RouteRecord: []string{"dra.ims.example.org"},
			},
			fcx(303, req|pxy, session("scscf;1;4"), fVSA(), authState(), ident(avp.OriginHost, scscf), ident(avp.OriginRealm, realm),
				ident(avp.DestinationRealm, realm), ident(avp.DestinationHost, hss), utf8(avp.UserName, impi), fFeatures(q),
				q.r("Public-Identity", datatype.UTF8String(impu)),
				q.g(avp.SIPAuthDataItem, tgpp, q.a(avp.SIPAuthenticationScheme, tgpp, datatype.UTF8String(aka)),
					q.a(avp.SIPAuthorization, tgpp, datatype.OctetString(make([]byte, 30)))),
				q.a(avp.SIPNumberAuthItems, tgpp, datatype.Unsigned32(1)), q.a(avp.ServerName, tgpp, datatype.UTF8String(scscfURI)),
				fProxy(), fRoute()),
		},
		{"MAA",
			&cx.MultimediaAuthAnswer{
				Header: hdr, SessionId: "scscf;1;4", VendorSpecificApplicationId: gVSA, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, UserName: diam.Some(impi), SupportedFeatures: gFeatures,
				PublicIdentity: diam.Some(impu), SipNumberAuthItems: diam.Some(uint32(3)),
				SipAuthDataItem: []cx.SipAuthDataItem{gAka, {SipAuthenticationScheme: diam.Some("SIP Digest"), SipDigestAuthenticate: gDigest}, gNass},
				FailedAvp:       gFailed, ProxyInfo: gProxy,
			},
			fcx(303, pxy, session("scscf;1;4"), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), utf8(avp.UserName, impi), fFeatures(s), s.r("Public-Identity", datatype.UTF8String(impu)),
				s.a(avp.SIPNumberAuthItems, tgpp, datatype.Unsigned32(3)), fAka(),
				s.g(avp.SIPAuthDataItem, tgpp, s.a(avp.SIPAuthenticationScheme, tgpp, datatype.UTF8String("SIP Digest")), fDigest()),
				fNass(), fFailed(), fProxy()),
		},
		{"RTR",
			&cx.RegistrationTerminationRequest{
				Header: hdr, SessionId: "hss;1;5", VendorSpecificApplicationId: gVSA, AuthSessionState: nsm, OriginHost: hss,
				OriginRealm: realm, DestinationHost: diam.Some(scscf), DestinationRealm: realm, UserName: impi,
				AssociatedIdentities: &cx.AssociatedIdentities{UserName: []string{"001010000000002@ims.example.org"}},
				SupportedFeatures:    gFeatures, PublicIdentity: []string{impu},
				DeregistrationReason: cx.DeregistrationReason{ReasonCode: cx.ReasonCodeNewServerAssigned, ReasonInfo: diam.Some("moved")},
				RtrFlags:             diam.Some(uint32(1)), ProxyInfo: gProxy, RouteRecord: []string{"dra.ims.example.org"},
			},
			fcx(304, req|pxy, session("hss;1;5"), fVSA(), authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, scscf), ident(avp.DestinationRealm, realm), utf8(avp.UserName, impi),
				q.rg("Associated-Identities", utf8(avp.UserName, "001010000000002@ims.example.org")), fFeatures(q),
				q.r("Public-Identity", datatype.UTF8String(impu)),
				q.g(avp.DeregistrationReason, tgpp, q.a(avp.ReasonCode, tgpp, datatype.Enumerated(1)),
					q.a(avp.ReasonInfo, tgpp, datatype.UTF8String("moved"))),
				q.r("RTR-Flags", datatype.Unsigned32(1)), fProxy(), fRoute()),
		},
		{"RTA",
			&cx.RegistrationTerminationAnswer{
				Header: hdr, SessionId: "hss;1;5", VendorSpecificApplicationId: gVSA, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: scscf, OriginRealm: realm,
				AssociatedIdentities:              &cx.AssociatedIdentities{UserName: []string{"001010000000002@ims.example.org"}},
				SupportedFeatures:                 gFeatures,
				IdentityWithEmergencyRegistration: []cx.IdentityWithEmergencyRegistration{{UserName: impi, PublicIdentity: "sip:sos@ims.example.org"}},
				FailedAvp:                         gFailed, ProxyInfo: gProxy,
			},
			fcx(304, pxy, session("hss;1;5"), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, scscf),
				ident(avp.OriginRealm, realm), s.rg("Associated-Identities", utf8(avp.UserName, "001010000000002@ims.example.org")),
				fFeatures(s), s.rg("Identity-with-Emergency-Registration", utf8(avp.UserName, impi),
					s.r("Public-Identity", datatype.UTF8String("sip:sos@ims.example.org"))),
				fFailed(), fProxy()),
		},
		{"PPR",
			&cx.PushProfileRequest{
				Header: hdr, SessionId: "hss;1;6", VendorSpecificApplicationId: gVSA, AuthSessionState: nsm, OriginHost: hss,
				OriginRealm: realm, DestinationHost: scscf, DestinationRealm: realm, UserName: impi, SupportedFeatures: gFeatures,
				UserData: diam.Some(userData), ChargingInformation: gCharging,
				SipAuthDataItem: &cx.SipAuthDataItem{SipDigestAuthenticate: gDigest}, AllowedWafWwsfIdentities: gWaf,
				ProxyInfo: gProxy, RouteRecord: []string{"dra.ims.example.org"},
			},
			fcx(305, req|pxy, session("hss;1;6"), fVSA(), authState(), ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, scscf), ident(avp.DestinationRealm, realm), utf8(avp.UserName, impi), fFeatures(q),
				q.a(avp.UserData, tgpp, datatype.OctetString(userData)), fCharging(), q.g(avp.SIPAuthDataItem, tgpp, fDigest()),
				fWaf(), fProxy(), fRoute()),
		},
		{"PPA",
			&cx.PushProfileAnswer{
				Header: hdr, SessionId: "hss;1;6", VendorSpecificApplicationId: gVSA, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: scscf, OriginRealm: realm, SupportedFeatures: gFeatures, FailedAvp: gFailed,
				ProxyInfo: gProxy,
			},
			fcx(305, pxy, session("hss;1;6"), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, scscf),
				ident(avp.OriginRealm, realm), fFeatures(s), fFailed(), fProxy()),
		},
	}
}

// TestFiorixCxGaps states what go-diameter v4.6.0 lacks for Cx and keeps
// cxRestated exact: an entry go-diameter gains must move to the
// independent side.
func TestFiorixCxGaps(t *testing.T) {
	if _, err := dict.Default.App(cxApp); err == nil {
		t.Errorf("go-diameter now has a Cx (%d) dictionary: build the Cx interop on it and drop the per-AVP lookup", cxApp)
	}
	for name, d := range cxRestated {
		if _, def := fiorixHome(d.code, d.vendor); def != nil {
			t.Errorf("go-diameter now defines %s (%d vendor %d) in application %s: remove it from cxRestated", name, d.code, d.vendor, def.App.Name)
		}
	}
}

// go-diameter encodes; go-diam decodes, validates and re-encodes the same
// bytes, and its own message of the same content is the same bytes.
func TestCxFiorixToGoDiam(t *testing.T) {
	for _, p := range cxPairs() {
		t.Run(p.name, func(t *testing.T) { checkFiorixToGoDiam(t, p) })
	}
}

// fiorixDecodeAVP decodes one encoded AVP the way go-diameter would inside
// the application that defines it, and does the same for each member of a
// grouped AVP that the enclosing application does not know. An AVP that
// go-diameter defines nowhere is decoded in app, which go-diameter has no
// dictionary for, so it stays datatype.Unknown.
func fiorixDecodeAVP(raw []byte, app uint32) (*fd.AVP, error) {
	if len(raw) < 8 {
		return nil, fmt.Errorf("short AVP")
	}
	code, vendor := binary.BigEndian.Uint32(raw), uint32(0)
	if raw[4]&avp.Vbit != 0 && len(raw) >= 12 {
		vendor = binary.BigEndian.Uint32(raw[8:])
	}
	home, def := fiorixHome(code, vendor)
	if def == nil {
		home = app // decodes as datatype.Unknown
	}
	a, err := fd.DecodeAVP(raw, home, dict.Default)
	if err != nil {
		return nil, fmt.Errorf("AVP %d vendor %d in application %d: %w", code, vendor, home, err)
	}
	if g, ok := a.Data.(*fd.GroupedAVP); ok {
		for i, m := range g.AVP {
			if _, unknown := m.Data.(datatype.Unknown); !unknown {
				continue
			}
			b, err := m.Serialize()
			if err != nil {
				return nil, err
			}
			if g.AVP[i], err = fiorixDecodeAVP(b, app); err != nil {
				return nil, err
			}
		}
	}
	return a, nil
}

// go-diam encodes; go-diameter reads the message (it has no Cx dictionary,
// so this checks only the framing) and then decodes every AVP, at any
// depth, with its own definition of that AVP from whichever application
// has it. See checkGoDiamToFiorixByAVP.
func TestCxGoDiamToFiorix(t *testing.T) {
	for _, p := range cxPairs() {
		t.Run(p.name, func(t *testing.T) { checkGoDiamToFiorixByAVP(t, p, cxApp, cxRestated) })
	}
}

// checkGoDiamToFiorixByAVP checks the go-diam to go-diameter direction for an
// application go-diameter has no dictionary for. go-diameter reads the
// message (framing only) and decodes every AVP, at any depth, in the
// application that defines it. The result must re-encode to the same bytes,
// carry the values go-diameter's own message carries, and contain no Unknown
// AVP except the restated ones.
func checkGoDiamToFiorixByAVP(t *testing.T, p pair, app uint32, restatedAVPs map[string]restated) {
	t.Helper()
	gaps := map[[2]uint32]string{}
	for name, d := range restatedAVPs {
		gaps[[2]uint32{d.code, d.vendor}] = name
	}
	wire, err := p.godiam.AppendMarshal(nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := fd.ReadMessage(bytes.NewReader(wire), dict.Default)
	if err != nil {
		t.Fatalf("go-diameter ReadMessage: %v", err)
	}
	if m.Header.HopByHopID != hopByHop || m.Header.EndToEndID != endToEnd || m.Header.ApplicationID != app ||
		m.Header.CommandCode != p.fiorix.Header.CommandCode || m.Header.CommandFlags != p.fiorix.Header.CommandFlags {
		t.Fatalf("header = %+v, want %+v", m.Header, p.fiorix.Header)
	}
	if again := serialize(t, m); !bytes.Equal(again, wire) {
		t.Fatalf("go-diameter re-encode differs\n%x\n%x", wire, again)
	}
	if len(m.AVP) != len(p.fiorix.AVP) {
		t.Fatalf("%d AVPs, go-diameter's own message has %d", len(m.AVP), len(p.fiorix.AVP))
	}
	for i, top := range m.AVP {
		raw, err := top.Serialize()
		if err != nil {
			t.Fatal(err)
		}
		a, err := fiorixDecodeAVP(raw, app)
		if err != nil {
			t.Fatal(err)
		}
		if err := noUnknown([]*fd.AVP{a}, "", gaps); err != nil {
			t.Fatal(err)
		}
		again, err := a.Serialize()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(again, raw) {
			t.Fatalf("AVP %d: go-diameter re-encode differs\n%x\n%x", a.Code, raw, again)
		}
		want := p.fiorix.AVP[i]
		wb, err := want.Serialize()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(wb, raw) {
			t.Fatalf("AVP #%d differs from go-diameter's own\n%x\n%x", i, wb, raw)
		}
		// Where go-diameter knows every AVP of the subtree, the values it
		// decodes are the ones it encoded.
		if noUnknown([]*fd.AVP{a}, "", nil) == nil && a.Data.String() != want.Data.String() {
			t.Fatalf("AVP #%d = %v, go-diameter's own message has %v", i, a, want)
		}
	}
}
