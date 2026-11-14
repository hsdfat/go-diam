package s6t

import (
	"testing"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
)

// seeds returns the encoded round-trip samples: every S6t command, so each
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

// One target per S6t command.

func FuzzCIR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ConfigurationInformationRequest) }, seeds(f)...)
}

func FuzzCIA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ConfigurationInformationAnswer) }, seeds(f)...)
}

func FuzzRIR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ReportingInformationRequest) }, seeds(f)...)
}

func FuzzRIA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ReportingInformationAnswer) }, seeds(f)...)
}

func FuzzNIR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(NiddInformationRequest) }, seeds(f)...)
}

func FuzzNIA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(NiddInformationAnswer) }, seeds(f)...)
}
