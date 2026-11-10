package base

import (
	"testing"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
)

// seeds returns the encoded round-trip samples: every base command, so each
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

// One target per base command.

func FuzzCER(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(CapabilitiesExchangeRequest) }, seeds(f)...)
}

func FuzzCEA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(CapabilitiesExchangeAnswer) }, seeds(f)...)
}

func FuzzDWR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(DeviceWatchdogRequest) }, seeds(f)...)
}

func FuzzDWA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(DeviceWatchdogAnswer) }, seeds(f)...)
}

func FuzzDPR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(DisconnectPeerRequest) }, seeds(f)...)
}

func FuzzDPA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(DisconnectPeerAnswer) }, seeds(f)...)
}

func FuzzRAR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ReAuthRequest) }, seeds(f)...)
}

func FuzzRAA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ReAuthAnswer) }, seeds(f)...)
}

func FuzzSTR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(SessionTerminationRequest) }, seeds(f)...)
}

func FuzzSTA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(SessionTerminationAnswer) }, seeds(f)...)
}

func FuzzASR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(AbortSessionRequest) }, seeds(f)...)
}

func FuzzASA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(AbortSessionAnswer) }, seeds(f)...)
}
