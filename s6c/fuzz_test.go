package s6c

import (
	"testing"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
)

// seeds returns the encoded round-trip samples: every S6c command, so each
// target also sees well-formed messages of the other commands.
func seeds(f *testing.F) [][]byte {
	f.Helper()
	var out [][]byte
	for _, m := range samples() {
		b, err := m.AppendMarshal(nil)
		if err != nil {
			f.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

// One target per S6c command.

func FuzzSRR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(SendRoutingInfoForSmRequest) }, seeds(f)...)
}

func FuzzSRA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(SendRoutingInfoForSmAnswer) }, seeds(f)...)
}

func FuzzALR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(AlertServiceCentreRequest) }, seeds(f)...)
}

func FuzzALA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(AlertServiceCentreAnswer) }, seeds(f)...)
}

func FuzzRDR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ReportSmDeliveryStatusRequest) }, seeds(f)...)
}

func FuzzRDA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ReportSmDeliveryStatusAnswer) }, seeds(f)...)
}
