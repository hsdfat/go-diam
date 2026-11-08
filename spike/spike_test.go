package spike_test

import (
	"bytes"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/spike/base"
	"github.com/hsdfat/go-diam/spike/cc"
	"github.com/hsdfat/go-diam/spike/s6a"
)

func sampleULR() *s6a.UpdateLocationRequest {
	return &s6a.UpdateLocationRequest{
		Header:           diam.Header{HopByHop: 0x1111, EndToEnd: 0x2222},
		SessionId:        "mme.example.org;1;2",
		AuthSessionState: base.AuthSessionStateNoStateMaintained,
		OriginHost:       "mme.example.org",
		OriginRealm:      "example.org",
		DestinationHost:  diam.Some("hss.example.org"),
		DestinationRealm: "example.org",
		UserName:         "001010000000001",
		SupportedFeatures: []s6a.SupportedFeatures{
			{VendorId: 10415, FeatureListId: 1, FeatureList: 0x0bff},
			{VendorId: 10415, FeatureListId: 2, FeatureList: 0x01},
		},
		TerminalInformation: &s6a.TerminalInformation{Imei: diam.Some("3534900698733"), SoftwareVersion: diam.Some("01")},
		RatType:             s6a.RatTypeEutran,
		UlrFlags:            0x22,
		UeSrvccCapability:   diam.Some(s6a.UeSrvccCapabilityUeSrvccSupported),
		VisitedPlmnId:       []byte{0x00, 0xf1, 0x10},
		ProxyInfo:           []base.ProxyInfo{{ProxyHost: "dra.example.org", ProxyState: []byte{1, 2}}},
		RouteRecord:         []string{"dra.example.org"},
	}
}

func TestULRRoundTripBytes(t *testing.T) {
	b1, err := sampleULR().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	m, err := diam.Decode(b1)
	if err != nil {
		t.Fatal(err)
	}
	ulr, ok := m.(*s6a.UpdateLocationRequest)
	if !ok {
		t.Fatalf("Decode returned %T", m)
	}
	if err := ulr.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if ulr.UserName != "001010000000001" || ulr.RatType != s6a.RatTypeEutran || len(ulr.SupportedFeatures) != 2 ||
		ulr.TerminalInformation.Imei.V != "3534900698733" || !ulr.UeSrvccCapability.Set || ulr.Header.HopByHop != 0x1111 {
		t.Fatalf("decoded fields wrong: %+v", ulr)
	}
	if ulr.Header.Flags != diam.FlagRequest|diam.FlagProxiable {
		t.Fatalf("flags = %#x", ulr.Header.Flags)
	}
	b2, err := ulr.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("re-encode differs\n%x\n%x", b1, b2)
	}
}

func TestUnknownAVPs(t *testing.T) {
	ulr := sampleULR()
	ulr.Unknown = []diam.RawAVP{
		{Code: 9999, Vendor: 12345, Data: []byte("opaque")},                  // M clear: keep, ignore
		{Code: 9998, Flags: diam.AVPFlagMandatory, Data: []byte{0, 0, 0, 1}}, // M set: 5001
	}
	b, _ := ulr.Marshal()
	var got s6a.UpdateLocationRequest
	if err := got.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if len(got.Unknown) != 2 || string(got.Unknown[0].Data) != "opaque" || got.Unknown[0].Vendor != 12345 {
		t.Fatalf("unknown AVPs not preserved: %+v", got.Unknown)
	}
	var ae *diam.AVPError
	if err := got.Validate(); !errors.As(err, &ae) || ae.ResultCode != diam.ResultAVPUnsupported || ae.AVP.Code != 9998 {
		t.Fatalf("Validate = %v, want 5001 for AVP 9998", err)
	}
	b2, _ := got.Marshal()
	if !bytes.Equal(b, b2) {
		t.Fatal("unknown AVPs not re-encoded verbatim")
	}
}

func TestMissingAndDuplicate(t *testing.T) {
	// Encode a ULA, then strip Origin-Realm by decoding into a struct that never sees it.
	ula := &s6a.UpdateLocationAnswer{SessionId: "s", AuthSessionState: base.AuthSessionStateNoStateMaintained, OriginHost: "h", OriginRealm: "r"}
	b, _ := ula.Marshal()
	// Drop the last AVP (Origin-Realm, 8+1 bytes padded to 12).
	b = b[:len(b)-12]
	b[1], b[2], b[3] = 0, byte(len(b)>>8), byte(len(b))
	var got s6a.UpdateLocationAnswer
	if err := got.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	var ae *diam.AVPError
	if err := got.Validate(); !errors.As(err, &ae) || ae.ResultCode != diam.ResultMissingAVP || ae.AVP.Code != 296 {
		t.Fatalf("Validate = %v, want 5005 Origin-Realm", err)
	}

	// Duplicate a single-occurrence AVP via Unknown injection of the same code.
	ula.Unknown = []diam.RawAVP{{Code: 264, Flags: diam.AVPFlagMandatory, Data: []byte("h2")}}
	b, _ = ula.Marshal()
	got = s6a.UpdateLocationAnswer{}
	_ = got.Unmarshal(b)
	if err := got.Validate(); !errors.As(err, &ae) || ae.ResultCode != diam.ResultAVPOccursTooMany {
		t.Fatalf("Validate = %v, want 5009", err)
	}
}

func TestCCRNested(t *testing.T) {
	ts := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	ccr := &cc.CreditControlRequest{
		SessionId:         "pgw;1",
		OriginHost:        "pgw.example.org",
		OriginRealm:       "example.org",
		DestinationRealm:  "example.org",
		AuthApplicationId: 4,
		ServiceContextId:  "32251@3gpp.org",
		CcRequestType:     cc.CcRequestTypeUpdateRequest,
		CcRequestNumber:   0, // zero is a valid required value and must still be encoded
		EventTimestamp:    diam.Some(ts),
		SubscriptionId: []cc.SubscriptionId{
			{SubscriptionIdType: cc.SubscriptionIdTypeEndUserImsi, SubscriptionIdData: "001010000000001"},
		},
		MultipleServicesCreditControl: []cc.MultipleServicesCreditControl{{
			RequestedServiceUnit: &cc.ServiceUnit{},
			UsedServiceUnit: []cc.ServiceUnit{{
				CcTotalOctets: diam.Some(uint64(1) << 40),
				CcMoney:       &cc.CcMoney{UnitValue: cc.UnitValue{ValueDigits: -12345, Exponent: diam.Some(int32(-2))}},
			}},
			RatingGroup: diam.Some(uint32(10)),
		}},
	}
	b1, err := ccr.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	m, err := diam.Decode(b1)
	if err != nil {
		t.Fatal(err)
	}
	got := m.(*cc.CreditControlRequest)
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	usu := got.MultipleServicesCreditControl[0].UsedServiceUnit[0]
	if usu.CcTotalOctets.V != 1<<40 || usu.CcMoney.UnitValue.ValueDigits != -12345 || usu.CcMoney.UnitValue.Exponent.V != -2 {
		t.Fatalf("nested values wrong: %+v", usu)
	}
	if !got.EventTimestamp.V.Equal(ts) {
		t.Fatalf("time = %v", got.EventTimestamp.V)
	}
	if got.MultipleServicesCreditControl[0].RequestedServiceUnit == nil {
		t.Fatal("empty grouped AVP lost")
	}
	b2, _ := got.Marshal()
	if !bytes.Equal(b1, b2) {
		t.Fatal("re-encode differs")
	}
}

func TestCERAddressAndMin(t *testing.T) {
	cer := &base.CapabilitiesExchangeRequest{OriginHost: "a", OriginRealm: "b", VendorId: 10415, ProductName: "go-diam"}
	b, _ := cer.Marshal()
	var got base.CapabilitiesExchangeRequest
	_ = got.Unmarshal(b)
	var ae *diam.AVPError
	if err := got.Validate(); !errors.As(err, &ae) || ae.AVP.Code != 257 {
		t.Fatalf("Validate = %v, want Host-IP-Address 1* violation", err)
	}
	cer.HostIpAddress = []netip.Addr{netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("2001:db8::1")}
	b, _ = cer.Marshal()
	got = base.CapabilitiesExchangeRequest{}
	_ = got.Unmarshal(b)
	if err := got.Validate(); err != nil || len(got.HostIpAddress) != 2 || got.HostIpAddress[1].String() != "2001:db8::1" {
		t.Fatalf("got %v %v", got.HostIpAddress, err)
	}
}
