package interop

import (
	"bytes"
	"testing"

	fd "github.com/fiorix/go-diameter/v4/diam"
	"github.com/fiorix/go-diameter/v4/diam/dict"

	"github.com/hsdfat/go-diam/diam"
)

// benchPair measures both libraries on the same wire bytes: decoding them,
// and encoding the message decoded from them.
func benchPair(b *testing.B, name string) {
	var p pair
	for _, c := range allPairs() {
		if c.name == name {
			p = c
		}
	}
	if p.godiam == nil {
		b.Fatalf("no sample %q", name)
	}
	wire := serialize(b, p.fiorix)
	fm, err := fd.ReadMessage(bytes.NewReader(wire), dict.Default)
	if err != nil {
		b.Fatal(err)
	}
	gm, err := diam.Decode(wire)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("go-diam/unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(wire)))
		for i := 0; i < b.N; i++ {
			if err := gm.Unmarshal(wire); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("go-diameter/unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(wire)))
		for i := 0; i < b.N; i++ {
			if _, err := fd.ReadMessage(bytes.NewReader(wire), dict.Default); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("go-diam/marshal", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(wire)))
		buf := make([]byte, 0, len(wire))
		for i := 0; i < b.N; i++ {
			var err error
			if buf, err = gm.AppendMarshal(buf[:0]); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("go-diameter/marshal", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(wire)))
		buf := make([]byte, fm.Len())
		for i := 0; i < b.N; i++ {
			if err := fm.SerializeTo(buf); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCER(b *testing.B) { benchPair(b, "CER") }

func BenchmarkSTR(b *testing.B) { benchPair(b, "STR") }

func BenchmarkULR(b *testing.B) { benchPair(b, "ULR") }

// ULA with a Subscription-Data that carries APN, GPRS, LCS, trace and CSG
// data (about 2.3 kB on the wire).
func BenchmarkULA(b *testing.B) { benchPair(b, "ULA") }

// S13 ECR: Terminal-Information with IMEI, 3GPP2-MEID and Software-Version.
func BenchmarkECR(b *testing.B) { benchPair(b, "ECR") }

// SWx MAA: an EAP-AKA' vector (SIP-Auth-Data-Item with CK' and IK'),
// OC-OLR, Load and Supported-Features.
func BenchmarkMAA(b *testing.B) { benchPair(b, "MAA") }
