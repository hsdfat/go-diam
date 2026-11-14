package s13

import (
	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6a"
)

const (
	mme, eir, realm = "mme.epc.example.org", "eir.epc.example.org", "epc.example.org"
	imsi            = "001010000000001"
	// IMEI: 8-digit TAC and 6-digit SNR, with the optional 15th digit
	// (§7.3.4), which the EIR ignores (§6.2.1.3).
	imei = "353490069873319"
	// Software-Version: the 2-digit SVN (§7.3.5).
	svn = "03"
)

var (
	hdr = diam.Header{HopByHop: 0x11, EndToEnd: 0x22}
	// §7.1.8: Auth-Application-Id 16777252 inside the 3GPP Vendor-Specific-Application-Id.
	vsai = &base.VendorSpecificApplicationId{VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777252))}
	nsm  = base.AuthSessionStateNoStateMaintained

	proxyInfo = []base.ProxyInfo{{ProxyHost: "dra.epc.example.org", ProxyState: []byte{1, 2, 3}}}
	failedAvp = &base.FailedAvp{Unknown: []diam.RawAVP{
		{Code: s6a.AVPSoftwareVersion, Vendor: 10415, Flags: diam.AVPFlagVendor | diam.AVPFlagMandatory, Data: []byte("x")},
	}}
	terminal = s6a.TerminalInformation{Imei: diam.Some(imei), SoftwareVersion: diam.Some(svn)}
)

// samples returns one message per S13/S13' command, request then answer
// (table 7.2.2/2). Together they carry every AVP an S13 command can carry.
func samples() []diam.Message {
	return []diam.Message{
		&MeIdentityCheckRequest{
			Header: hdr, SessionId: "mme.epc.example.org;1;1", Drmp: diam.Some(s6a.DrmpPriority5), VendorSpecificApplicationId: vsai,
			AuthSessionState: nsm, OriginHost: mme, OriginRealm: realm, DestinationHost: diam.Some(eir), DestinationRealm: realm,
			TerminalInformation: terminal, UserName: diam.Some(imsi),
			ProxyInfo: proxyInfo, RouteRecord: []string{"dra.epc.example.org"},
		},
		&MeIdentityCheckAnswer{
			Header: hdr, SessionId: "mme.epc.example.org;1;1", Drmp: diam.Some(s6a.DrmpPriority5), VendorSpecificApplicationId: vsai,
			ResultCode: diam.Some(uint32(2001)), AuthSessionState: nsm, OriginHost: eir, OriginRealm: realm,
			EquipmentStatus: diam.Some(s6a.EquipmentStatusTrackinglisted),
			FailedAvp:       failedAvp, ProxyInfo: proxyInfo, RouteRecord: []string{"dra.epc.example.org"},
		},
	}
}
