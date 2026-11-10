// Package diamtest holds test helpers shared by the generated packages and
// the interop module: a fuzz property for commands and a semantic comparison
// of encoded messages.
package diamtest

import (
	"bytes"
	"fmt"
	"sort"
	"testing"

	"github.com/hsdfat/go-diam/diam"
)

// CheckRoundTrip is the property every command must hold for any input:
// Unmarshal does not panic, and when it accepts the input, encoding the
// result, decoding that and encoding again gives the same bytes.
//
// The first encoding may differ from the input (grammar order, dropped
// excess occurrences), so stability is checked from the second one.
func CheckRoundTrip(m diam.Message, input []byte) error {
	if err := m.Unmarshal(input); err != nil {
		return nil
	}
	b1, err := m.AppendMarshal(nil)
	if err != nil {
		return fmt.Errorf("marshal after unmarshal: %w", err)
	}
	if err := m.Unmarshal(b1); err != nil {
		return fmt.Errorf("unmarshal of own encoding: %w\n%x", err, b1)
	}
	b2, err := m.AppendMarshal(nil)
	if err != nil {
		return fmt.Errorf("second marshal: %w", err)
	}
	if !bytes.Equal(b1, b2) {
		return fmt.Errorf("re-encoding is not stable\nfirst  %x\nsecond %x", b1, b2)
	}
	// Validate must not panic either; its result depends on the input.
	_ = m.Validate()
	return nil
}

// FuzzMessage fuzzes one command type with CheckRoundTrip.
func FuzzMessage(f *testing.F, newFn func() diam.Message, seeds ...[]byte) {
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		if err := CheckRoundTrip(newFn(), input); err != nil {
			t.Fatal(err)
		}
	})
}

// SameAVPs reports whether two encoded messages carry the same AVPs,
// ignoring their order: the multisets of (code, vendor, flags, data) must be
// equal. The V-bit is ignored because the vendor implies it. Data that itself
// splits into AVPs on both sides is compared the same way, so a reordering
// inside a grouped AVP is not a difference either.
//
// This is the equality a re-encoded capture of a real peer must satisfy: the
// encoder writes grammar order, the peer may not.
func SameAVPs(a, b []byte) error {
	_, da, err := diam.ParseHeader(a)
	if err != nil {
		return fmt.Errorf("first message: %w", err)
	}
	_, db, err := diam.ParseHeader(b)
	if err != nil {
		return fmt.Errorf("second message: %w", err)
	}
	ka, err := keys(da)
	if err != nil {
		return fmt.Errorf("first message: %w", err)
	}
	kb, err := keys(db)
	if err != nil {
		return fmt.Errorf("second message: %w", err)
	}
	return diff(ka, kb)
}

// key is the canonical form of one AVP: its identity, and its data with any
// nested AVPs in a canonical order.
type key struct {
	code, vendor uint32
	flags        uint8
	data         string
}

func (k key) String() string {
	return fmt.Sprintf("avp %d vendor %d flags %#x data %x", k.code, k.vendor, k.flags, k.data)
}

func less(a, b key) bool {
	if a.code != b.code {
		return a.code < b.code
	}
	if a.vendor != b.vendor {
		return a.vendor < b.vendor
	}
	if a.flags != b.flags {
		return a.flags < b.flags
	}
	return a.data < b.data
}

// keys splits an AVP sequence into sorted canonical keys.
func keys(data []byte) ([]key, error) {
	var out []key
	for len(data) > 0 {
		a, rest, err := diam.NextAVP(data)
		if err != nil {
			return nil, err
		}
		data = rest
		out = append(out, key{code: a.Code, vendor: a.Vendor, flags: a.Flags &^ diam.AVPFlagVendor, data: canonical(a.Data)})
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out, nil
}

// canonical returns data unchanged unless it splits cleanly into AVPs, in
// which case the AVPs are put in canonical order. Applying the same rule to
// both messages keeps the comparison sound even when a plain value happens
// to look like a group.
func canonical(data []byte) string {
	if len(data) < 8 || len(data)%4 != 0 {
		return string(data)
	}
	inner, err := keys(data)
	if err != nil {
		return string(data)
	}
	var b []byte
	for _, k := range inner {
		b = diam.AppendRaw(b, diam.RawAVP{Code: k.code, Vendor: k.vendor, Flags: k.flags, Data: []byte(k.data)})
	}
	return string(b)
}

// diff reports the first AVP that one sorted key list has and the other
// lacks.
func diff(a, b []key) error {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			i++
			j++
		case less(a[i], b[j]):
			return fmt.Errorf("only in the first message: %v", a[i])
		default:
			return fmt.Errorf("only in the second message: %v", b[j])
		}
	}
	if i < len(a) {
		return fmt.Errorf("only in the first message: %v", a[i])
	}
	if j < len(b) {
		return fmt.Errorf("only in the second message: %v", b[j])
	}
	return nil
}
