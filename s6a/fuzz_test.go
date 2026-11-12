package s6a

import (
	"testing"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
)

// seeds returns the encoded round-trip samples: every S6a command, so each
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

// One target per S6a/S6d command.

func FuzzULR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(UpdateLocationRequest) }, seeds(f)...)
}

func FuzzULA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(UpdateLocationAnswer) }, seeds(f)...)
}

func FuzzCLR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(CancelLocationRequest) }, seeds(f)...)
}

func FuzzCLA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(CancelLocationAnswer) }, seeds(f)...)
}

func FuzzAIR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(AuthenticationInformationRequest) }, seeds(f)...)
}

func FuzzAIA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(AuthenticationInformationAnswer) }, seeds(f)...)
}

func FuzzIDR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(InsertSubscriberDataRequest) }, seeds(f)...)
}

func FuzzIDA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(InsertSubscriberDataAnswer) }, seeds(f)...)
}

func FuzzDSR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(DeleteSubscriberDataRequest) }, seeds(f)...)
}

func FuzzDSA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(DeleteSubscriberDataAnswer) }, seeds(f)...)
}

func FuzzPUR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(PurgeUeRequest) }, seeds(f)...)
}

func FuzzPUA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(PurgeUeAnswer) }, seeds(f)...)
}

func FuzzRSR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ResetRequest) }, seeds(f)...)
}

func FuzzRSA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(ResetAnswer) }, seeds(f)...)
}

func FuzzNOR(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(NotifyRequest) }, seeds(f)...)
}

func FuzzNOA(f *testing.F) {
	diamtest.FuzzMessage(f, func() diam.Message { return new(NotifyAnswer) }, seeds(f)...)
}
