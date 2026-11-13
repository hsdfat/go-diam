package interop

import (
	"testing"
	"time"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/sh"
)

// shApp is the Sh/Dh application id (TS 29.329 §6).
const shApp = 16777217

// go-diameter v4.6.0 ships no Sh/Dh application either: no 16777217
// dictionary and no Sh commands. Of the Sh AVPs it defines MSISDN, Server-Name
// and Session-Priority (Ro/Rf), Supported-Features with Feature-List-ID and
// Feature-List (Ro/Rf, Rx, SWx), and the re-used IETF AVPs (DRMP, OC-*, Load
// and its members) under the base protocol and Rx. The Sh interop below is
// built on those, as the Cx one is (see cx_test.go).
//
// The AVPs go-diameter has nowhere are listed in shRestated with code,
// vendor and M-bit restated from TS 29.329 table 6.3.1 (and TS 29.336 for
// External-Identifier) here, in the test, not read from go-diam's schema.
// Their value encoding is still go-diameter's. TestFiorixShGaps keeps the
// list exact.
var shRestated = map[string]restated{
	"Public-Identity":            {601, tgpp, true, "table 6.3.1"},
	"Wildcarded-Public-Identity": {634, tgpp, false, "table 6.3.1"},
	"Wildcarded-IMPU":            {636, tgpp, false, "table 6.3.1"},
	"User-Identity":              {700, tgpp, true, "table 6.3.1"},
	"User-Data":                  {702, tgpp, true, "table 6.3.1"},
	"Data-Reference":             {703, tgpp, true, "table 6.3.1"},
	"Service-Indication":         {704, tgpp, true, "table 6.3.1"},
	"Subs-Req-Type":              {705, tgpp, true, "table 6.3.1"},
	"Requested-Domain":           {706, tgpp, true, "table 6.3.1"},
	"Current-Location":           {707, tgpp, true, "table 6.3.1"},
	"Identity-Set":               {708, tgpp, false, "table 6.3.1"},
	"Expiry-Time":                {709, tgpp, false, "table 6.3.1"},
	"Send-Data-Indication":       {710, tgpp, false, "table 6.3.1"},
	"DSAI-Tag":                   {711, tgpp, true, "table 6.3.1"},
	"One-Time-Notification":      {712, tgpp, false, "table 6.3.1"},
	"Requested-Nodes":            {713, tgpp, false, "table 6.3.1"},
	"Serving-Node-Indication":    {714, tgpp, false, "table 6.3.1"},
	"Repository-Data-ID":         {715, tgpp, false, "table 6.3.1"},
	"Sequence-Number":            {716, tgpp, false, "table 6.3.1"},
	"Pre-paging-Supported":       {717, tgpp, false, "table 6.3.1"},
	"Local-Time-Zone-Indication": {718, tgpp, false, "table 6.3.1"},
	"UDR-Flags":                  {719, tgpp, false, "table 6.3.1"},
	"Call-Reference-Info":        {720, tgpp, false, "table 6.3.1"},
	"Call-Reference-Number":      {721, tgpp, false, "table 6.3.1"},
	"AS-Number":                  {722, tgpp, false, "table 6.3.1"},
	"External-Identifier":        {3111, tgpp, true, "TS 29.336 §6.4.11, table 6.3/2 Must set"},
}

// shRequestM: Supported-Features carries the M-bit in requests and not in
// answers (TS 29.329 §7.1, which applies TS 29.229 §7.2.1).
var shRequestM = map[[2]uint32]bool{{avp.SupportedFeatures, tgpp}: true}

// fsh builds a go-diameter message of the Sh application.
func fsh(cmd uint32, flags uint8, avps ...*fd.AVP) *fd.Message {
	m := fd.NewMessage(cmd, flags, shApp, hopByHop, endToEnd, dict.Default)
	for _, a := range avps {
		m.AddAVP(a)
	}
	return m
}

// shPairs is one message per Sh/Dh command, built with go-diam and with
// go-diameter, covering every grouped AVP and every Sh AVP a command can
// carry.
func shPairs() []pair {
	const (
		as, hss, realm = "as.ims.example.org", "hss.ims.example.org", "ims.example.org"
		impi           = "001010000000001@ims.example.org"
		impu           = "sip:+15551234567@ims.example.org"
		asURI          = "sip:as.ims.example.org"
		wildPSI        = "sip:room-!.*!@ims.example.org"
		wildIMPU       = "sip:+1555!.*!@ims.example.org"
		extID          = "device-1@iot.example.org"
	)
	expiry := time.Date(2026, 12, 13, 8, 0, 0, 0, time.UTC)
	msisdn := []byte{0x51, 0x55, 0x21, 0x43, 0x65, 0xf7}
	userData := []byte("<Sh-Data><RepositoryData><ServiceIndication>svc</ServiceIndication></RepositoryData></Sh-Data>")
	nsm := base.AuthSessionStateNoStateMaintained
	q, s := perAVP{true, shRestated, shRequestM}, perAVP{false, shRestated, shRequestM}

	session := func(v string) *fd.AVP { return fa(avp.SessionID, datatype.UTF8String(v)) }
	authState := func() *fd.AVP { return enum(avp.AuthSessionState, 1) }
	gVSA := base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(shApp))}
	fVSA := func() *fd.AVP {
		return group(avp.VendorSpecificApplicationID, u32(avp.VendorID, tgpp), u32(avp.AuthApplicationID, shApp))
	}
	gProxy := []base.ProxyInfo{{ProxyHost: "dra.ims.example.org", ProxyState: []byte{1, 2, 3}}}
	fProxy := func() *fd.AVP {
		return group(avp.ProxyInfo, ident(avp.ProxyHost, "dra.ims.example.org"), octets(avp.ProxyState, 1, 2, 3))
	}
	fRoute := func() *fd.AVP { return ident(avp.RouteRecord, "dra.ims.example.org") }
	gFailed := &base.FailedAvp{Unknown: []diam.RawAVP{{Code: base.AVPOriginStateId, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 9}}}}
	fFailed := func() *fd.AVP { return group(avp.FailedAVP, u32(avp.OriginStateID, 9)) }
	// DIAMETER_ERROR_TRANSPARENT_DATA OUT_OF_SYNC (§6.2.2.7).
	gExp := &base.ExperimentalResult{VendorId: tgpp, ExperimentalResultCode: 5105}
	fExp := func() *fd.AVP {
		return group(avp.ExperimentalResult, u32(avp.VendorID, tgpp), u32(avp.ExperimentalResultCode, 5105))
	}

	gFeatures := []sh.SupportedFeatures{{VendorId: tgpp, FeatureListId: 1, FeatureList: 0x07}}
	fFeatures := func(b perAVP) *fd.AVP {
		return b.g(avp.SupportedFeatures, tgpp, b.a(avp.VendorID, 0, datatype.Unsigned32(tgpp)),
			b.a(avp.FeatureListID, tgpp, datatype.Unsigned32(1)), b.a(avp.FeatureList, tgpp, datatype.Unsigned32(0x07)))
	}
	gOcSF := &sh.OcSupportedFeatures{OcFeatureVector: diam.Some(uint64(1))}
	fOcSF := func() *fd.AVP {
		return s.g(avp.OCSupportedFeatures, 0, s.a(avp.OCFeatureVector, 0, datatype.Unsigned64(1)))
	}
	gOlr := &sh.OcOlr{OcSequenceNumber: 9, OcReportType: sh.OcReportTypeRealmReport,
		OcReductionPercentage: diam.Some(uint32(50)), OcValidityDuration: diam.Some(uint32(30))}
	fOlr := func() *fd.AVP {
		return s.g(avp.OCOLR, 0, s.a(avp.OCSequenceNumber, 0, datatype.Unsigned64(9)), s.a(avp.OCReportType, 0, datatype.Enumerated(1)),
			s.a(avp.OCReductionPercentage, 0, datatype.Unsigned32(50)), s.a(avp.OCValidityDuration, 0, datatype.Unsigned32(30)))
	}
	// Load-Value is left out: go-diameter types it Unsigned32, RFC 8583
	// §7.3 says Unsigned64 (see dictcheck-allow.txt).
	gLoad := []sh.Load{{LoadType: diam.Some(sh.LoadTypeHost), Sourceid: diam.Some(hss)}}
	fLoad := func() *fd.AVP {
		return s.g(avp.Load, 0, s.a(avp.LoadType, 0, datatype.Enumerated(0)), s.a(avp.SourceID, 0, datatype.DiameterIdentity(hss)))
	}
	fUserData := func(b perAVP) *fd.AVP { return b.r("User-Data", datatype.OctetString(userData)) }
	fImpu := func(b perAVP) *fd.AVP {
		return b.rg("User-Identity", b.r("Public-Identity", datatype.UTF8String(impu)))
	}

	const req, pxy = fd.RequestFlag, fd.ProxiableFlag
	return []pair{
		{"UDR",
			&sh.UserDataRequest{
				Header: hdr, SessionId: "as;1;1", Drmp: diam.Some(sh.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				AuthSessionState: nsm, OriginHost: as, OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm,
				SupportedFeatures: gFeatures, UserIdentity: sh.UserIdentity{PublicIdentity: diam.Some(impu)},
				WildcardedImpu: diam.Some(wildIMPU), ServerName: diam.Some(asURI), ServiceIndication: [][]byte{[]byte("svc")},
				DataReference:   []sh.DataReference{sh.DataReferenceRepositoryData, sh.DataReferenceCsrn},
				IdentitySet:     []sh.IdentitySet{sh.IdentitySetRegisteredIdentities},
				RequestedDomain: diam.Some(sh.RequestedDomainPsDomain), CurrentLocation: diam.Some(sh.CurrentLocationInitiateActiveLocationRetrieval),
				DsaiTag: [][]byte{[]byte("dsai-1")}, SessionPriority: diam.Some(sh.SessionPriorityPriority1), UserName: diam.Some(impi),
				RequestedNodes: diam.Some(uint32(9)), ServingNodeIndication: diam.Some(sh.ServingNodeIndicationOnlyServingNodesRequired),
				PrePagingSupported:      diam.Some(sh.PrePagingSupportedPrepagingSupported),
				LocalTimeZoneIndication: diam.Some(sh.LocalTimeZoneIndicationOnlyLocalTimeZoneRequested),
				UdrFlags:                diam.Some(uint32(3)),
				CallReferenceInfo:       &sh.CallReferenceInfo{CallReferenceNumber: []byte{1, 2, 3}, AsNumber: []byte{0x91, 0x51, 0x55}},
				OcSupportedFeatures:     gOcSF, ProxyInfo: gProxy, RouteRecord: []string{"dra.ims.example.org"},
			},
			fsh(306, req|pxy, session("as;1;1"), q.a(avp.DRMP, 0, datatype.Enumerated(5)), fVSA(), authState(),
				ident(avp.OriginHost, as), ident(avp.OriginRealm, realm), ident(avp.DestinationHost, hss),
				ident(avp.DestinationRealm, realm), fFeatures(q), fImpu(q),
				q.r("Wildcarded-IMPU", datatype.UTF8String(wildIMPU)), q.a(avp.ServerName, tgpp, datatype.UTF8String(asURI)),
				q.r("Service-Indication", datatype.OctetString("svc")),
				q.r("Data-Reference", datatype.Enumerated(0)), q.r("Data-Reference", datatype.Enumerated(30)),
				q.r("Identity-Set", datatype.Enumerated(1)), q.r("Requested-Domain", datatype.Enumerated(1)),
				q.r("Current-Location", datatype.Enumerated(1)), q.r("DSAI-Tag", datatype.OctetString("dsai-1")),
				q.a(avp.SessionPriority, tgpp, datatype.Enumerated(1)), utf8(avp.UserName, impi),
				q.r("Requested-Nodes", datatype.Unsigned32(9)), q.r("Serving-Node-Indication", datatype.Enumerated(0)),
				q.r("Pre-paging-Supported", datatype.Enumerated(1)), q.r("Local-Time-Zone-Indication", datatype.Enumerated(0)),
				q.r("UDR-Flags", datatype.Unsigned32(3)),
				q.rg("Call-Reference-Info", q.r("Call-Reference-Number", datatype.OctetString([]byte{1, 2, 3})),
					q.r("AS-Number", datatype.OctetString([]byte{0x91, 0x51, 0x55}))),
				fOcSF(), fProxy(), fRoute()),
		},
		{"UDA",
			&sh.UserDataAnswer{
				Header: hdr, SessionId: "as;1;1", VendorSpecificApplicationId: gVSA, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, SupportedFeatures: gFeatures,
				WildcardedPublicIdentity: diam.Some(wildPSI), WildcardedImpu: diam.Some(wildIMPU), UserData: diam.Some(userData),
				OcSupportedFeatures: gOcSF, OcOlr: gOlr, Load: gLoad, FailedAvp: gFailed, ProxyInfo: gProxy,
				RouteRecord: []string{"dra.ims.example.org"},
			},
			fsh(306, pxy, session("as;1;1"), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), fFeatures(s), s.r("Wildcarded-Public-Identity", datatype.UTF8String(wildPSI)),
				s.r("Wildcarded-IMPU", datatype.UTF8String(wildIMPU)), fUserData(s), fOcSF(), fOlr(), fLoad(), fFailed(),
				fProxy(), fRoute()),
		},
		{"PUR",
			&sh.ProfileUpdateRequest{
				Header: hdr, SessionId: "as;1;2", VendorSpecificApplicationId: gVSA, AuthSessionState: nsm, OriginHost: as,
				OriginRealm: realm, DestinationHost: diam.Some(hss), DestinationRealm: realm, SupportedFeatures: gFeatures,
				UserIdentity: sh.UserIdentity{Msisdn: diam.Some(msisdn)}, WildcardedPublicIdentity: diam.Some(wildPSI),
				UserName: diam.Some(impi), DataReference: []sh.DataReference{sh.DataReferenceRepositoryData}, UserData: userData,
				OcSupportedFeatures: gOcSF, ProxyInfo: gProxy, RouteRecord: []string{"dra.ims.example.org"},
			},
			fsh(307, req|pxy, session("as;1;2"), fVSA(), authState(), ident(avp.OriginHost, as), ident(avp.OriginRealm, realm),
				ident(avp.DestinationHost, hss), ident(avp.DestinationRealm, realm), fFeatures(q),
				q.rg("User-Identity", q.a(avp.MSISDN, tgpp, datatype.OctetString(msisdn))),
				q.r("Wildcarded-Public-Identity", datatype.UTF8String(wildPSI)), utf8(avp.UserName, impi),
				q.r("Data-Reference", datatype.Enumerated(0)), fUserData(q), fOcSF(), fProxy(), fRoute()),
		},
		{"PUA",
			&sh.ProfileUpdateAnswer{
				Header: hdr, SessionId: "as;1;2", Drmp: diam.Some(sh.DrmpPriority3), VendorSpecificApplicationId: gVSA,
				ExperimentalResult: gExp, AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm,
				WildcardedImpu:   diam.Some(wildIMPU),
				RepositoryDataId: &sh.RepositoryDataId{ServiceIndication: []byte("svc"), SequenceNumber: 3},
				DataReference:    diam.Some(sh.DataReferenceRepositoryData), SupportedFeatures: gFeatures,
				OcSupportedFeatures: gOcSF, OcOlr: gOlr, Load: gLoad, FailedAvp: gFailed, ProxyInfo: gProxy,
			},
			fsh(307, pxy, session("as;1;2"), s.a(avp.DRMP, 0, datatype.Enumerated(3)), fVSA(), fExp(), authState(),
				ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm), s.r("Wildcarded-IMPU", datatype.UTF8String(wildIMPU)),
				s.rg("Repository-Data-ID", s.r("Service-Indication", datatype.OctetString("svc")), s.r("Sequence-Number", datatype.Unsigned32(3))),
				s.r("Data-Reference", datatype.Enumerated(0)), fFeatures(s), fOcSF(), fOlr(), fLoad(), fFailed(), fProxy()),
		},
		{"SNR",
			&sh.SubscribeNotificationsRequest{
				Header: hdr, SessionId: "as;1;3", VendorSpecificApplicationId: gVSA, AuthSessionState: nsm, OriginHost: as,
				OriginRealm: realm, DestinationRealm: realm, SupportedFeatures: gFeatures,
				UserIdentity:             sh.UserIdentity{ExternalIdentifier: diam.Some(extID)},
				WildcardedPublicIdentity: diam.Some(wildPSI), ServiceIndication: [][]byte{[]byte("svc")},
				SendDataIndication: diam.Some(sh.SendDataIndicationUserDataRequested), ServerName: diam.Some(asURI),
				SubsReqType: sh.SubsReqTypeSubscribe, DataReference: []sh.DataReference{sh.DataReferenceUeReachabilityForIp},
				IdentitySet: []sh.IdentitySet{sh.IdentitySetAllIdentities}, ExpiryTime: diam.Some(expiry),
				DsaiTag: [][]byte{[]byte("dsai-2")}, OneTimeNotification: diam.Some(sh.OneTimeNotificationOneTimeNotificationRequested),
				UserName: diam.Some(impi), OcSupportedFeatures: gOcSF, ProxyInfo: gProxy, RouteRecord: []string{"dra.ims.example.org"},
			},
			fsh(308, req|pxy, session("as;1;3"), fVSA(), authState(), ident(avp.OriginHost, as), ident(avp.OriginRealm, realm),
				ident(avp.DestinationRealm, realm), fFeatures(q),
				q.rg("User-Identity", q.r("External-Identifier", datatype.UTF8String(extID))),
				q.r("Wildcarded-Public-Identity", datatype.UTF8String(wildPSI)), q.r("Service-Indication", datatype.OctetString("svc")),
				q.r("Send-Data-Indication", datatype.Enumerated(1)), q.a(avp.ServerName, tgpp, datatype.UTF8String(asURI)),
				q.r("Subs-Req-Type", datatype.Enumerated(0)), q.r("Data-Reference", datatype.Enumerated(25)),
				q.r("Identity-Set", datatype.Enumerated(0)), q.r("Expiry-Time", datatype.Time(expiry)),
				q.r("DSAI-Tag", datatype.OctetString("dsai-2")), q.r("One-Time-Notification", datatype.Enumerated(0)),
				utf8(avp.UserName, impi), fOcSF(), fProxy(), fRoute()),
		},
		{"SNA",
			&sh.SubscribeNotificationsAnswer{
				Header: hdr, SessionId: "as;1;3", VendorSpecificApplicationId: gVSA, AuthSessionState: nsm,
				ResultCode: diam.Some(uint32(2001)), OriginHost: hss, OriginRealm: realm, WildcardedPublicIdentity: diam.Some(wildPSI),
				SupportedFeatures: gFeatures, UserData: diam.Some(userData), ExpiryTime: diam.Some(expiry),
				OcSupportedFeatures: gOcSF, OcOlr: gOlr, Load: gLoad, FailedAvp: gFailed, ProxyInfo: gProxy,
			},
			// SNA: Auth-Session-State comes before Result-Code (§6.1.6).
			fsh(308, pxy, session("as;1;3"), fVSA(), authState(), u32(avp.ResultCode, 2001), ident(avp.OriginHost, hss),
				ident(avp.OriginRealm, realm), s.r("Wildcarded-Public-Identity", datatype.UTF8String(wildPSI)), fFeatures(s),
				fUserData(s), s.r("Expiry-Time", datatype.Time(expiry)), fOcSF(), fOlr(), fLoad(), fFailed(), fProxy()),
		},
		{"PNR",
			&sh.PushNotificationRequest{
				Header: hdr, SessionId: "hss;1;4", Drmp: diam.Some(sh.DrmpPriority4), VendorSpecificApplicationId: gVSA,
				AuthSessionState: nsm, OriginHost: hss, OriginRealm: realm, DestinationHost: as, DestinationRealm: realm,
				SupportedFeatures: gFeatures, UserIdentity: sh.UserIdentity{PublicIdentity: diam.Some(impu)},
				WildcardedImpu: diam.Some(wildIMPU), UserName: diam.Some(impi), UserData: userData, ProxyInfo: gProxy,
				RouteRecord: []string{"dra.ims.example.org"},
			},
			fsh(309, req|pxy, session("hss;1;4"), q.a(avp.DRMP, 0, datatype.Enumerated(4)), fVSA(), authState(),
				ident(avp.OriginHost, hss), ident(avp.OriginRealm, realm), ident(avp.DestinationHost, as),
				ident(avp.DestinationRealm, realm), fFeatures(q), fImpu(q), q.r("Wildcarded-IMPU", datatype.UTF8String(wildIMPU)),
				utf8(avp.UserName, impi), fUserData(q), fProxy(), fRoute()),
		},
		{"PNA",
			&sh.PushNotificationAnswer{
				Header: hdr, SessionId: "hss;1;4", VendorSpecificApplicationId: gVSA, ResultCode: diam.Some(uint32(2001)),
				AuthSessionState: nsm, OriginHost: as, OriginRealm: realm, SupportedFeatures: gFeatures, FailedAvp: gFailed,
				ProxyInfo: gProxy,
			},
			fsh(309, pxy, session("hss;1;4"), fVSA(), u32(avp.ResultCode, 2001), authState(), ident(avp.OriginHost, as),
				ident(avp.OriginRealm, realm), fFeatures(s), fFailed(), fProxy()),
		},
	}
}

// TestFiorixShGaps states what go-diameter v4.6.0 lacks for Sh and keeps
// shRestated exact: an entry go-diameter gains must move to the independent
// side.
func TestFiorixShGaps(t *testing.T) {
	if _, err := dict.Default.App(shApp); err == nil {
		t.Errorf("go-diameter now has an Sh (%d) dictionary: build the Sh interop on it and drop the per-AVP lookup", shApp)
	}
	for name, d := range shRestated {
		if _, def := fiorixHome(d.code, d.vendor); def != nil {
			t.Errorf("go-diameter now defines %s (%d vendor %d) in application %s: remove it from shRestated", name, d.code, d.vendor, def.App.Name)
		}
	}
}

// go-diameter encodes; go-diam decodes, validates and re-encodes the same
// bytes, and its own message of the same content is the same bytes.
func TestShFiorixToGoDiam(t *testing.T) {
	for _, p := range shPairs() {
		t.Run(p.name, func(t *testing.T) { checkFiorixToGoDiam(t, p) })
	}
}

// go-diam encodes; go-diameter reads the message (it has no Sh dictionary)
// and decodes every AVP in the application that defines it
// (checkGoDiamToFiorixByAVP in cx_test.go).
func TestShGoDiamToFiorix(t *testing.T) {
	for _, p := range shPairs() {
		t.Run(p.name, func(t *testing.T) { checkGoDiamToFiorixByAVP(t, p, shApp, shRestated) })
	}
}
