package s6c_test

import (
	"fmt"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6c"
)

// An SMS-GMSC asks the HSS where to deliver a short message with a
// Send-Routing-Info-for-SM-Request; the HSS decodes it, checks it and
// returns the IMSI and the serving MME.
func Example() {
	srr := &s6c.SendRoutingInfoForSmRequest{
		Header:           diam.Header{HopByHop: 1, EndToEnd: 1},
		SessionId:        "sms-gmsc.example.org;1;42",
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       "sms-gmsc.example.org",
		OriginRealm:      "example.org",
		DestinationRealm: "example.org",
		Msisdn:           diam.Some([]byte{0x51, 0x55, 0x21, 0x43, 0x65, 0xf7}), // TBCD +15551234567
		SmRpMti:          diam.Some(s6c.SmRpMtiSmDeliver),
	}
	wire, err := srr.Marshal()
	if err != nil {
		panic(err)
	}

	// HSS side.
	m, err := diam.Decode(wire)
	if err != nil {
		panic(err)
	}
	req := m.(*s6c.SendRoutingInfoForSmRequest)
	if err := req.Validate(); err != nil {
		panic(err)
	}
	sra := s6c.NewSendRoutingInfoForSmAnswerFor(req)
	sra.ResultCode = diam.Some(uint32(2001))
	sra.AuthSessionState = base.AuthSessionStateNoStateMaintained
	sra.OriginHost, sra.OriginRealm = "hss.example.org", "example.org"
	sra.UserName = diam.Some("001010000000001")
	sra.ServingNode = &s6c.ServingNode{MmeName: diam.Some("mme.example.org"), MmeRealm: diam.Some("example.org")}
	answer, _ := sra.Marshal()

	// SMS-GMSC side.
	var got s6c.SendRoutingInfoForSmAnswer
	if err := got.Unmarshal(answer); err != nil {
		panic(err)
	}
	fmt.Println(req.SmRpMti.V, got.SessionId, got.ResultCode.V, got.UserName.V, got.ServingNode.MmeName.V)

	// Output:
	// SM_DELIVER sms-gmsc.example.org;1;42 2001 001010000000001 mme.example.org
}
