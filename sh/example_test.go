package sh_test

import (
	"fmt"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/sh"
)

// An application server reads its repository data for a user with a
// User-Data-Request; the HSS decodes it, checks it and answers with the
// Sh-Data document.
func Example() {
	udr := &sh.UserDataRequest{
		Header:    diam.Header{HopByHop: 1, EndToEnd: 1},
		SessionId: "as.ims.example.org;1;42",
		VendorSpecificApplicationId: base.VendorSpecificApplicationId{
			VendorId: 10415, AuthApplicationId: diam.Some(uint32(16777217)),
		},
		AuthSessionState:  base.AuthSessionStateNoStateMaintained,
		OriginHost:        "as.ims.example.org",
		OriginRealm:       "ims.example.org",
		DestinationRealm:  "ims.example.org",
		UserIdentity:      sh.UserIdentity{PublicIdentity: diam.Some("sip:+15551234567@ims.example.org")},
		ServiceIndication: [][]byte{[]byte("svc")},
		DataReference:     []sh.DataReference{sh.DataReferenceRepositoryData},
	}
	wire, err := udr.Marshal()
	if err != nil {
		panic(err)
	}

	// HSS side.
	m, err := diam.Decode(wire)
	if err != nil {
		panic(err)
	}
	req := m.(*sh.UserDataRequest)
	if err := req.Validate(); err != nil {
		panic(err)
	}
	uda := sh.NewUserDataAnswerFor(req)
	uda.VendorSpecificApplicationId = req.VendorSpecificApplicationId
	uda.ResultCode = diam.Some(uint32(2001))
	uda.AuthSessionState = base.AuthSessionStateNoStateMaintained
	uda.OriginHost, uda.OriginRealm = "hss.ims.example.org", "ims.example.org"
	uda.UserData = diam.Some([]byte("<Sh-Data><RepositoryData>...</RepositoryData></Sh-Data>"))
	answer, _ := uda.Marshal()

	// AS side.
	var got sh.UserDataAnswer
	if err := got.Unmarshal(answer); err != nil {
		panic(err)
	}
	fmt.Println(req.UserIdentity.PublicIdentity.V, req.DataReference[0], got.SessionId, got.ResultCode.V, len(got.UserData.V))

	// Output:
	// sip:+15551234567@ims.example.org REPOSITORY_DATA as.ims.example.org;1;42 2001 55
}
