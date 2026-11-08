// Cross-checks go-diam against fiorix/go-diameter, an independent encoder.
package interop

import (
	"bytes"
	"testing"
	"time"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/avp"
	"github.com/fiorix/go-diameter/v4/diam/datatype"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/spike/base"
	"github.com/hsdfat/go-diam/spike/cc"
	"github.com/hsdfat/go-diam/spike/s6a"
)

func fiorixULR(t testing.TB) []byte {
	m := fd.NewRequest(316, 16777251, dict.Default)
	m.Header.CommandFlags |= fd.ProxiableFlag
	m.Header.HopByHopID, m.Header.EndToEndID = 7, 9
	m.NewAVP(avp.SessionID, avp.Mbit, 0, datatype.UTF8String("mme;1"))
	m.NewAVP(avp.AuthSessionState, avp.Mbit, 0, datatype.Enumerated(1))
	m.NewAVP(avp.OriginHost, avp.Mbit, 0, datatype.DiameterIdentity("mme.example.org"))
	m.NewAVP(avp.OriginRealm, avp.Mbit, 0, datatype.DiameterIdentity("example.org"))
	m.NewAVP(avp.DestinationRealm, avp.Mbit, 0, datatype.DiameterIdentity("example.org"))
	m.NewAVP(avp.UserName, avp.Mbit, 0, datatype.UTF8String("001010000000001"))
	m.NewAVP(avp.SupportedFeatures, avp.Vbit, 10415, &fd.GroupedAVP{AVP: []*fd.AVP{
		fd.NewAVP(avp.VendorID, avp.Mbit, 0, datatype.Unsigned32(10415)),
		fd.NewAVP(avp.FeatureListID, avp.Vbit, 10415, datatype.Unsigned32(1)),
		fd.NewAVP(avp.FeatureList, avp.Vbit, 10415, datatype.Unsigned32(0x0bff)),
	}})
	m.NewAVP(avp.TerminalInformation, avp.Mbit|avp.Vbit, 10415, &fd.GroupedAVP{AVP: []*fd.AVP{
		fd.NewAVP(avp.IMEI, avp.Mbit|avp.Vbit, 10415, datatype.UTF8String("3534900698733")),
	}})
	m.NewAVP(avp.RATType, avp.Mbit|avp.Vbit, 10415, datatype.Enumerated(1004))
	m.NewAVP(avp.ULRFlags, avp.Mbit|avp.Vbit, 10415, datatype.Unsigned32(0x22))
	m.NewAVP(avp.VisitedPLMNID, avp.Mbit|avp.Vbit, 10415, datatype.OctetString([]byte{0, 0xf1, 0x10}))
	b, err := m.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// go-diameter encodes, go-diam decodes and re-encodes byte-identically.
func TestFiorixToGoDiamULR(t *testing.T) {
	b := fiorixULR(t)
	m, err := diam.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	ulr := m.(*s6a.UpdateLocationRequest)
	if err := ulr.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if ulr.UserName != "001010000000001" || ulr.RatType != s6a.RatTypeEutran || ulr.UlrFlags != 0x22 ||
		len(ulr.SupportedFeatures) != 1 || ulr.SupportedFeatures[0].FeatureList != 0x0bff ||
		ulr.TerminalInformation.Imei.V != "3534900698733" || ulr.Header.HopByHop != 7 || len(ulr.Unknown) != 0 {
		t.Fatalf("decoded wrong: %+v", ulr)
	}
	out, _ := ulr.Marshal()
	if !bytes.Equal(out, b) {
		t.Fatalf("go-diam re-encode differs from go-diameter bytes\n%x\n%x", b, out)
	}
}

// go-diam encodes, go-diameter decodes with its own dictionary.
func TestGoDiamToFiorixCCR(t *testing.T) {
	ts := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	ccr := &cc.CreditControlRequest{
		SessionId: "pgw;1", OriginHost: "pgw.example.org", OriginRealm: "example.org",
		DestinationRealm: "example.org", AuthApplicationId: 4, ServiceContextId: "32251@3gpp.org",
		CcRequestType: cc.CcRequestTypeInitialRequest, CcRequestNumber: 0,
		EventTimestamp: diam.Some(ts),
		SubscriptionId: []cc.SubscriptionId{{SubscriptionIdType: cc.SubscriptionIdTypeEndUserImsi, SubscriptionIdData: "001010000000001"}},
		MultipleServicesCreditControl: []cc.MultipleServicesCreditControl{{
			UsedServiceUnit: []cc.ServiceUnit{{CcTotalOctets: diam.Some(uint64(1) << 40)}},
			RatingGroup:     diam.Some(uint32(10)),
		}},
		ProxyInfo: []base.ProxyInfo{{ProxyHost: "dra", ProxyState: []byte{1}}},
	}
	b, err := ccr.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	m, err := fd.ReadMessage(bytes.NewReader(b), dict.Default)
	if err != nil {
		t.Fatalf("go-diameter ReadMessage: %v", err)
	}
	get := func(code uint32) *fd.AVP {
		a, err := m.FindAVP(code, 0)
		if err != nil {
			t.Fatalf("FindAVP %d: %v", code, err)
		}
		return a
	}
	if v := get(avp.CCRequestNumber).Data.(datatype.Unsigned32); v != 0 {
		t.Fatalf("CC-Request-Number = %d", v)
	}
	if v := get(avp.EventTimestamp).Data.(datatype.Time); !time.Time(v).Equal(ts) {
		t.Fatalf("Event-Timestamp = %v", time.Time(v))
	}
	if v := get(avp.CCTotalOctets).Data.(datatype.Unsigned64); v != 1<<40 {
		t.Fatalf("CC-Total-Octets = %d", v)
	}
	if v := get(avp.SubscriptionIDData).Data.(datatype.UTF8String); v != "001010000000001" {
		t.Fatalf("Subscription-Id-Data = %q", v)
	}
	again, _ := m.Serialize()
	if !bytes.Equal(again, b) {
		t.Fatalf("go-diameter re-encode differs\n%x\n%x", b, again)
	}
}

func BenchmarkULR(b *testing.B) {
	raw := fiorixULR(b)
	b.Run("go-diam/unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		var m s6a.UpdateLocationRequest
		for i := 0; i < b.N; i++ {
			if err := m.Unmarshal(raw); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("go-diameter/unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := fd.ReadMessage(bytes.NewReader(raw), dict.Default); err != nil {
				b.Fatal(err)
			}
		}
	})
	var ulr s6a.UpdateLocationRequest
	_ = ulr.Unmarshal(raw)
	fm, _ := fd.ReadMessage(bytes.NewReader(raw), dict.Default)
	b.Run("go-diam/marshal", func(b *testing.B) {
		b.ReportAllocs()
		buf := make([]byte, 0, 512)
		for i := 0; i < b.N; i++ {
			buf, _ = ulr.AppendMarshal(buf[:0])
		}
	})
	b.Run("go-diameter/marshal", func(b *testing.B) {
		b.ReportAllocs()
		buf := make([]byte, fm.Len())
		for i := 0; i < b.N; i++ {
			_ = fm.SerializeTo(buf)
		}
	})
}
