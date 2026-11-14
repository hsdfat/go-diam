package s6m

import (
	"testing"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
)

// seeds returns the encoded round-trip samples: both S6m commands, so each
// target also sees a well-formed message of the other command.
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

// One target per S6m command.

func FuzzSIR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(SubscriberInformationRequest) }, seeds(f)...)
}

func FuzzSIA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(SubscriberInformationAnswer) }, seeds(f)...)
}
