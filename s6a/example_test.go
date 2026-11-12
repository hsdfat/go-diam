package s6a_test

import (
	"fmt"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s6a"
)

// An MME sends an Update-Location-Request; the HSS decodes it, checks it
// and answers with the subscriber's APN profile.
func Example() {
	ulr := &s6a.UpdateLocationRequest{
		Header:           diam.Header{HopByHop: 1, EndToEnd: 1},
		SessionId:        "mme.example.org;1;42",
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       "mme.example.org",
		OriginRealm:      "example.org",
		DestinationRealm: "example.org",
		UserName:         "001010000000001",
		RatType:          s6a.RatTypeEutran,
		UlrFlags:         0x22,
		VisitedPlmnId:    []byte{0x00, 0xf1, 0x10},
	}
	wire, err := ulr.Marshal()
	if err != nil {
		panic(err)
	}

	// HSS side.
	m, err := diam.Decode(wire)
	if err != nil {
		panic(err)
	}
	req := m.(*s6a.UpdateLocationRequest)
	if err := req.Validate(); err != nil {
		panic(err)
	}
	ula := s6a.NewUpdateLocationAnswerFor(req)
	ula.ResultCode = diam.Some(uint32(2001))
	ula.AuthSessionState = base.AuthSessionStateNoStateMaintained
	ula.OriginHost, ula.OriginRealm = "hss.example.org", "example.org"
	ula.SubscriptionData = &s6a.SubscriptionData{
		ApnConfigurationProfile: &s6a.ApnConfigurationProfile{
			ContextIdentifier: 1,
			ApnConfiguration: []s6a.ApnConfiguration{{
				ContextIdentifier: 1,
				PdnType:           s6a.PdnTypeIpv4v6,
				ServiceSelection:  "internet",
			}},
		},
	}
	answer, _ := ula.Marshal()

	// MME side.
	var got s6a.UpdateLocationAnswer
	if err := got.Unmarshal(answer); err != nil {
		panic(err)
	}
	apn := got.SubscriptionData.ApnConfigurationProfile.ApnConfiguration[0]
	fmt.Println(req.UserName, req.RatType, got.SessionId, got.ResultCode.V, apn.ServiceSelection, apn.PdnType)

	// Output:
	// 001010000000001 EUTRAN mme.example.org;1;42 2001 internet IPV4V6
}
