package interop

import (
	"strings"
	"testing"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s13"
	"github.com/hsdfat/go-diam/s6a"
)

// s13App is the S13/S13' application id (TS 29.272 §7.1.8).
const s13App = 16777252

// go-diameter v4.6.0 has an S13 application (16777252, its tgpp_s13.xml):
// command 324 and Terminal-Information, IMEI, 3GPP2-MEID, Software-Version
// and Equipment-Status, with the base protocol (DRMP included) inherited. So
// the S13 interop is built like the S6a one, every AVP from that
// application, and go-diameter decodes go-diam's messages with it. Its ECR
// grammar has { User-Name } where TS 29.272 §7.2.19 has [ User-Name ], and
// it names the Equipment-Status values with the Release 8 names; neither
// changes the bytes (the ECR sample carries User-Name), and go-diameter does
// not enforce its grammar on read.

// ea builds a go-diameter AVP for an S13 message from go-diameter's own S13
// dictionary: code from its constants, flags from that dictionary.
func ea(code, vendor uint32, data datatype.Type) *fd.AVP {
	def, err := dict.Default.FindAVPByCode(s13App, code, vendor)
	if err != nil {
		panic(err)
	}
	var flags uint8
	if strings.Contains(def.Must, "M") {
		flags = avp.Mbit
	}
	return fd.NewAVP(code, flags, vendor, data)
}

// fs13 builds a go-diameter message of the S13 application.
func fs13(cmd uint32, flags uint8, avps ...*fd.AVP) *fd.Message {
	m := fd.NewMessage(cmd, flags, s13App, hopByHop, endToEnd, dict.Default)
	for _, a := range avps {
		m.AddAVP(a)
	}
	return m
}

// s13Pairs is ECR and ECA, built with go-diam and with go-diameter, with
// every AVP the two grammars and Terminal-Information allow, and the
// EQUIPMENT_UNKNOWN answer of §6.2.1.3.
func s13Pairs() []pair {
	const (
		mme, eir, realm = "mme.epc.example.org", "eir.epc.example.org", "epc.example.org"
		imsi            = "001010000000001"
		imei            = "353490069873319"
	)
	meid := []byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde}
	nsm := base.AuthSessionStateNoStateMaintained

	session := func(v string) *fd.AVP { return ea(avp.SessionID, 0, datatype.UTF8String(v)) }
	authState := func() *fd.AVP { return ea(avp.AuthSessionState, 0, datatype.Enumerated(1)) }
	host := func(code uint32, v string) *fd.AVP { return ea(code, 0, datatype.DiameterIdentity(v)) }
	gVSA := &base.VendorSpecificApplicationId{VendorId: tgpp, AuthApplicationId: diam.Some(uint32(s13App))}
	fVSA := func() *fd.AVP {
		return ea(avp.VendorSpecificApplicationID, 0, &fd.GroupedAVP{AVP: []*fd.AVP{
			ea(avp.VendorID, 0, datatype.Unsigned32(tgpp)), ea(avp.AuthApplicationID, 0, datatype.Unsigned32(s13App)),
		}})
	}
	gProxy := []base.ProxyInfo{{ProxyHost: "dra.epc.example.org", ProxyState: []byte{1, 2, 3}}}
	fProxy := func() *fd.AVP {
		return ea(avp.ProxyInfo, 0, &fd.GroupedAVP{AVP: []*fd.AVP{
			host(avp.ProxyHost, "dra.epc.example.org"), ea(avp.ProxyState, 0, datatype.OctetString([]byte{1, 2, 3})),
		}})
	}
	fRoute := func() *fd.AVP { return host(avp.RouteRecord, "dra.epc.example.org") }
	gFailed := &base.FailedAvp{Unknown: []diam.RawAVP{{Code: base.AVPOriginStateId, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 9}}}}
	fFailed := func() *fd.AVP {
		return ea(avp.FailedAVP, 0, &fd.GroupedAVP{AVP: []*fd.AVP{ea(avp.OriginStateID, 0, datatype.Unsigned32(9))}})
	}

	const req, pxy = fd.RequestFlag, fd.ProxiableFlag
	return []pair{
		{"ECR",
			&s13.MeIdentityCheckRequest{
				Header: hdr, SessionId: "mme;1;1", Drmp: diam.Some(s6a.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				AuthSessionState: nsm, OriginHost: mme, OriginRealm: realm, DestinationHost: diam.Some(eir), DestinationRealm: realm,
				TerminalInformation: s6a.TerminalInformation{Imei: diam.Some(imei), Tgpp2Meid: diam.Some(meid), SoftwareVersion: diam.Some("03")},
				UserName:            diam.Some(imsi), ProxyInfo: gProxy, RouteRecord: []string{"dra.epc.example.org"},
			},
			fs13(324, req|pxy, session("mme;1;1"), ea(avp.DRMP, 0, datatype.Enumerated(5)), fVSA(), authState(),
				host(avp.OriginHost, mme), host(avp.OriginRealm, realm), host(avp.DestinationHost, eir), host(avp.DestinationRealm, realm),
				ea(avp.TerminalInformation, tgpp, &fd.GroupedAVP{AVP: []*fd.AVP{
					ea(avp.IMEI, tgpp, datatype.UTF8String(imei)), ea(avp.TGPP2MEID, tgpp, datatype.OctetString(meid)),
					ea(avp.SoftwareVersion, tgpp, datatype.UTF8String("03")),
				}}),
				ea(avp.UserName, 0, datatype.UTF8String(imsi)), fProxy(), fRoute()),
		},
		{"ECA",
			&s13.MeIdentityCheckAnswer{
				Header: hdr, SessionId: "mme;1;1", Drmp: diam.Some(s6a.DrmpPriority5), VendorSpecificApplicationId: gVSA,
				ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: eir, OriginRealm: realm,
				EquipmentStatus: diam.Some(s6a.EquipmentStatusTrackinglisted), FailedAvp: gFailed, ProxyInfo: gProxy,
				RouteRecord: []string{"dra.epc.example.org"},
			},
			fs13(324, pxy, session("mme;1;1"), ea(avp.DRMP, 0, datatype.Enumerated(5)), fVSA(),
				ea(avp.ResultCode, 0, datatype.Unsigned32(2001)), authState(), host(avp.OriginHost, eir), host(avp.OriginRealm, realm),
				ea(avp.EquipmentStatus, tgpp, datatype.Enumerated(2)), fFailed(), fProxy(), fRoute()),
		},
		// §6.2.1.3: DIAMETER_ERROR_EQUIPMENT_UNKNOWN (5422, §7.4.3.5), no Equipment-Status.
		{"ECA-unknown",
			&s13.MeIdentityCheckAnswer{
				Header: hdr, SessionId: "mme;1;2", ExperimentalResult: &base.ExperimentalResult{VendorId: tgpp, ExperimentalResultCode: 5422},
				AuthSessionState: nsm, OriginHost: eir, OriginRealm: realm,
			},
			fs13(324, pxy, session("mme;1;2"),
				ea(avp.ExperimentalResult, 0, &fd.GroupedAVP{AVP: []*fd.AVP{
					ea(avp.VendorID, 0, datatype.Unsigned32(tgpp)), ea(avp.ExperimentalResultCode, 0, datatype.Unsigned32(5422)),
				}}),
				authState(), host(avp.OriginHost, eir), host(avp.OriginRealm, realm)),
		},
	}
}

// TestFiorixS13App states what the S13 interop relies on: go-diameter's S13
// application with every AVP the samples use, and command 324.
func TestFiorixS13App(t *testing.T) {
	if _, err := dict.Default.App(s13App); err != nil {
		t.Fatalf("go-diameter has no S13 (%d) application: %v", s13App, err)
	}
	if _, err := dict.Default.FindCommand(s13App, 324); err != nil {
		t.Errorf("go-diameter's S13 application has no command 324: %v", err)
	}
	for _, c := range [][2]uint32{
		{avp.TerminalInformation, tgpp}, {avp.IMEI, tgpp}, {avp.TGPP2MEID, tgpp}, {avp.SoftwareVersion, tgpp},
		{avp.EquipmentStatus, tgpp}, {avp.DRMP, 0},
	} {
		if _, err := dict.Default.FindAVPByCode(s13App, c[0], c[1]); err != nil {
			t.Errorf("go-diameter's S13 application lacks AVP %d vendor %d", c[0], c[1])
		}
	}
}
