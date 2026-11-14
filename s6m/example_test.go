package s6m_test

import (
	"fmt"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6m"
)

// An MTC-IWF asks the HSS how to reach a device for a trigger with a
// Subscriber-Information-Request; the HSS decodes it, checks it and returns
// the IMSI and the serving MME.
func Example() {
	sir := &s6m.SubscriberInformationRequest{
		Header:           diam.Header{HopByHop: 1, EndToEnd: 1},
		SessionId:        "mtc-iwf.example.org;1;42",
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       "mtc-iwf.example.org",
		OriginRealm:      "example.org",
		DestinationRealm: "example.org",
		UserIdentifier:   s6m.UserIdentifier{ExternalIdentifier: diam.Some("device-1@iot.example.org")},
		ServiceId:        diam.Some(s6m.ServiceIdDeviceTrigger),
		SirFlags:         0x01, // S6m (TS 29.336 table 6.4.10/1)
	}
	wire, err := sir.Marshal()
	if err != nil {
		panic(err)
	}

	// HSS side.
	m, err := diam.Decode(wire)
	if err != nil {
		panic(err)
	}
	req := m.(*s6m.SubscriberInformationRequest)
	if err := req.Validate(); err != nil {
		panic(err)
	}
	sia := s6m.NewSubscriberInformationAnswerFor(req)
	sia.ResultCode = diam.Some(uint32(2001))
	sia.AuthSessionState = base.AuthSessionStateNoStateMaintained
	sia.OriginHost, sia.OriginRealm = "hss.example.org", "example.org"
	sia.UserIdentifier = []s6m.UserIdentifier{{UserName: diam.Some("001010000000001")}}
	sia.ServiceData = &s6m.ServiceData{T4Data: &s6m.T4Data{ServingNode: &s6m.ServingNode{
		MmeName: diam.Some("mme.example.org"), MmeRealm: diam.Some("example.org"),
	}}}
	answer, _ := sia.Marshal()

	// MTC-IWF side.
	var got s6m.SubscriberInformationAnswer
	if err := got.Unmarshal(answer); err != nil {
		panic(err)
	}
	fmt.Println(req.UserIdentifier.ExternalIdentifier.V, req.ServiceId.V, got.ResultCode.V,
		got.UserIdentifier[0].UserName.V, got.ServiceData.T4Data.ServingNode.MmeName.V)

	// Output:
	// device-1@iot.example.org DEVICE_TRIGGER 2001 001010000000001 mme.example.org
}
