package gentest

import (
	"testing"

	"github.com/hsdfat/go-diam/diam"
	"github.com/hsdfat/go-diam/internal/diamtest"
)

// FuzzTestRequest covers every data type and cardinality the generator
// knows, which the base protocol alone does not.
func FuzzTestRequest(f *testing.F) {
	b, err := sampleRequest().Marshal()
	if err != nil {
		f.Fatal(err)
	}
	empty, _ := (&TestRequest{}).Marshal()
	diamtest.FuzzMessage(f, func() diam.Message { return new(TestRequest) }, b, empty)
}
