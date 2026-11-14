package slh_test

import (
	"fmt"
	"net/netip"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/slh"
)

// A GMLC asks the HSS where a user is served with an
// LCS-Routing-Info-Request; the HSS answers with the serving MME and the
// H-GMLC address.
func Example() {
	rir := &slh.LcsRoutingInfoRequest{
		Header:           diam.Header{HopByHop: 1, EndToEnd: 1},
		SessionId:        "gmlc.epc.example.org;1;42",
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       "gmlc.epc.example.org",
		OriginRealm:      "epc.example.org",
		DestinationRealm: "epc.example.org",
		UserName:         diam.Some("001010000000001"),
	}
	wire, err := rir.Marshal()
	if err != nil {
		panic(err)
	}

	// HSS side.
	m, err := diam.Decode(wire)
	if err != nil {
		panic(err)
	}
	req := m.(*slh.LcsRoutingInfoRequest)
	if err := req.Validate(); err != nil {
		panic(err)
	}
	ria := slh.NewLcsRoutingInfoAnswerFor(req)
	ria.ResultCode = diam.Some(uint32(2001))
	ria.AuthSessionState = base.AuthSessionStateNoStateMaintained
	ria.OriginHost, ria.OriginRealm = "hss.epc.example.org", "epc.example.org"
	ria.Msisdn = diam.Some([]byte{0x51, 0x55, 0x21, 0x43, 0x65, 0xf7})
	ria.ServingNode = &slh.ServingNode{MmeName: diam.Some("mme.epc.example.org"), MmeRealm: diam.Some("epc.example.org")}
	ria.GmlcAddress = diam.Some(netip.MustParseAddr("192.0.2.10"))
	answer, _ := ria.Marshal()

	// GMLC side.
	var got slh.LcsRoutingInfoAnswer
	if err := got.Unmarshal(answer); err != nil {
		panic(err)
	}
	fmt.Println(got.SessionId, got.ResultCode.V, got.ServingNode.MmeName.V, got.GmlcAddress.V)

	// Output:
	// gmlc.epc.example.org;1;42 2001 mme.epc.example.org 192.0.2.10
}
