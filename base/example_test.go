package base_test

import (
	"fmt"
	"net/netip"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
)

// Build a CER, and decode and check the peer's CEA.
func Example() {
	cer := &base.CapabilitiesExchangeRequest{
		Header:            diam.Header{HopByHop: 1, EndToEnd: 1},
		OriginHost:        "mme.example.org",
		OriginRealm:       "example.org",
		HostIpAddress:     []netip.Addr{netip.MustParseAddr("192.0.2.1")},
		VendorId:          10415,
		ProductName:       "my-node",
		AuthApplicationId: []uint32{16777251},
	}
	b, err := cer.Marshal()
	if err != nil {
		panic(err)
	}
	fmt.Println("CER bytes:", len(b))

	// The peer's answer, here built locally.
	cea := base.NewCapabilitiesExchangeAnswerFor(cer)
	cea.ResultCode = 2001
	cea.OriginHost, cea.OriginRealm = "hss.example.org", "example.org"
	cea.HostIpAddress = []netip.Addr{netip.MustParseAddr("192.0.2.2")}
	cea.VendorId, cea.ProductName = 10415, "peer"
	answer, _ := cea.Marshal()

	m, err := diam.Decode(answer)
	if err != nil {
		panic(err)
	}
	if got, ok := m.(*base.CapabilitiesExchangeAnswer); ok && got.Validate() == nil {
		fmt.Println(got.ResultCode, got.OriginHost, got.Header.HopByHop)
	}

	// A CEA without Host-IP-Address violates the grammar; Validate says
	// which Result-Code to answer with.
	cea.HostIpAddress = nil
	answer, _ = cea.Marshal()
	m, _ = diam.Decode(answer)
	fmt.Println(m.Validate())

	// Output:
	// CER bytes: 120
	// 2001 hss.example.org 1
	// diam: AVP HOST_IP_ADDRESS needs at least 1 occurrences (result-code 5005, avp 257)
}
