package s13_test

import (
	"fmt"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/s13"
	"github.com/hsdfat/go-diam/s6a"
)

// An MME checks a handset with an ME-Identity-Check-Request; the EIR
// decodes it, checks it and answers with the equipment status.
func Example() {
	ecr := &s13.MeIdentityCheckRequest{
		Header:           diam.Header{HopByHop: 1, EndToEnd: 1},
		SessionId:        "mme.epc.example.org;1;42",
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       "mme.epc.example.org",
		OriginRealm:      "epc.example.org",
		DestinationRealm: "epc.example.org",
		TerminalInformation: s6a.TerminalInformation{
			Imei: diam.Some("35349006987331"), SoftwareVersion: diam.Some("03"),
		},
		UserName: diam.Some("001010000000001"),
	}
	wire, err := ecr.Marshal()
	if err != nil {
		panic(err)
	}

	// EIR side.
	m, err := diam.Decode(wire)
	if err != nil {
		panic(err)
	}
	req := m.(*s13.MeIdentityCheckRequest)
	if err := req.Validate(); err != nil {
		panic(err)
	}
	eca := s13.NewMeIdentityCheckAnswerFor(req)
	eca.ResultCode = diam.Some(uint32(2001))
	eca.AuthSessionState = base.AuthSessionStateNoStateMaintained
	eca.OriginHost, eca.OriginRealm = "eir.epc.example.org", "epc.example.org"
	eca.EquipmentStatus = diam.Some(s6a.EquipmentStatusPermittedlisted)
	answer, _ := eca.Marshal()

	// MME side.
	var got s13.MeIdentityCheckAnswer
	if err := got.Unmarshal(answer); err != nil {
		panic(err)
	}
	fmt.Println(req.TerminalInformation.Imei.V, got.SessionId, got.ResultCode.V, got.EquipmentStatus.V)

	// Output:
	// 35349006987331 mme.epc.example.org;1;42 2001 PERMITTEDLISTED
}
