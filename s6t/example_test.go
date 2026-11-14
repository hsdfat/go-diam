package s6t_test

import (
	"fmt"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6t"
)

// An SCEF asks the HSS to monitor the reachability of a UE with a
// Configuration-Information-Request; the HSS decodes it, checks it and
// accepts the configuration.
func Example() {
	cir := &s6t.ConfigurationInformationRequest{
		Header:           diam.Header{HopByHop: 1, EndToEnd: 1},
		SessionId:        "scef.example.org;1;42",
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       "scef.example.org",
		OriginRealm:      "example.org",
		DestinationRealm: "example.org",
		UserIdentifier:   s6t.UserIdentifier{ExternalIdentifier: diam.Some("device-1@iot.example.org")},
		MonitoringEventConfiguration: []s6t.MonitoringEventConfiguration{{
			ScefReferenceId: diam.Some(uint32(7)),
			ScefId:          "scef.example.org",
			MonitoringType:  1, // UE_REACHABILITY (TS 29.336 §8.4.7)
		}},
	}
	wire, err := cir.Marshal()
	if err != nil {
		panic(err)
	}

	// HSS side.
	m, err := diam.Decode(wire)
	if err != nil {
		panic(err)
	}
	req := m.(*s6t.ConfigurationInformationRequest)
	if err := req.Validate(); err != nil {
		panic(err)
	}
	cia := s6t.NewConfigurationInformationAnswerFor(req)
	cia.ResultCode = diam.Some(uint32(2001))
	cia.AuthSessionState = base.AuthSessionStateNoStateMaintained
	cia.OriginHost, cia.OriginRealm = "hss.example.org", "example.org"
	cia.MonitoringEventConfigStatus = []s6t.MonitoringEventConfigStatus{{ScefReferenceId: 7, ScefId: diam.Some("scef.example.org")}}
	answer, _ := cia.Marshal()

	// SCEF side.
	var got s6t.ConfigurationInformationAnswer
	if err := got.Unmarshal(answer); err != nil {
		panic(err)
	}
	fmt.Println(req.UserIdentifier.ExternalIdentifier.V, req.MonitoringEventConfiguration[0].MonitoringType, got.SessionId,
		got.ResultCode.V, got.MonitoringEventConfigStatus[0].ScefReferenceId)

	// Output:
	// device-1@iot.example.org 1 scef.example.org;1;42 2001 7
}
