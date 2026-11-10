package diamtest

import (
	"strings"
	"testing"

	"github.com/hsdfat/go-diam/diam"
)

func raw(code, vendor uint32, flags uint8, data ...byte) diam.RawAVP {
	return diam.RawAVP{Code: code, Vendor: vendor, Flags: flags, Data: data}
}

func seq(avps ...diam.RawAVP) []byte {
	var b []byte
	for _, a := range avps {
		b = diam.AppendRaw(b, a)
	}
	return b
}

func msg(avps ...diam.RawAVP) []byte {
	b := diam.AppendHeader(nil, &diam.Header{CommandCode: 257})
	b = append(b, seq(avps...)...)
	b, _ = diam.FinishMessage(b, 0)
	return b
}

func TestSameAVPs(t *testing.T) {
	a := raw(264, 0, diam.AVPFlagMandatory, 'h')
	b := raw(296, 0, diam.AVPFlagMandatory, 'r')
	c := raw(1407, 10415, diam.AVPFlagMandatory, 1, 2, 3)
	inner1 := raw(266, 0, diam.AVPFlagMandatory, 0, 0, 0x28, 0xaf)
	inner2 := raw(258, 0, diam.AVPFlagMandatory, 1, 0, 0, 0x23)
	group := func(inner ...diam.RawAVP) diam.RawAVP {
		return diam.RawAVP{Code: 260, Flags: diam.AVPFlagMandatory, Data: seq(inner...)}
	}

	same := map[string][2][]byte{
		"identical":                 {msg(a, b, c), msg(a, b, c)},
		"top level reordered":       {msg(a, b, c), msg(c, a, b)},
		"reordered inside a group":  {msg(a, group(inner1, inner2)), msg(group(inner2, inner1), a)},
		"repeated AVP":              {msg(a, a, b), msg(a, b, a)},
		"empty":                     {msg(), msg()},
		"V-bit implied by a vendor": {msg(raw(1, 10415, 0, 9)), msg(raw(1, 10415, diam.AVPFlagVendor, 9))},
	}
	for name, p := range same {
		if err := SameAVPs(p[0], p[1]); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	different := map[string][2][]byte{
		"different data":             {msg(a, b), msg(a, raw(296, 0, diam.AVPFlagMandatory, 'x'))},
		"missing AVP":                {msg(a, b, c), msg(a, b)},
		"extra AVP":                  {msg(a), msg(a, b)},
		"different occurrence count": {msg(a, a, b), msg(a, b, b)},
		"different M-bit":            {msg(a), msg(raw(264, 0, 0, 'h'))},
		"different vendor":           {msg(c), msg(raw(1407, 10416, diam.AVPFlagMandatory, 1, 2, 3))},
		"different inside a group":   {msg(group(inner1, inner2)), msg(group(inner1, a))},
	}
	for name, p := range different {
		err := SameAVPs(p[0], p[1])
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		if !strings.Contains(err.Error(), "avp") {
			t.Errorf("%s: error %q does not name an AVP", name, err)
		}
	}

	if err := SameAVPs(msg(a)[:10], msg(a)); err == nil {
		t.Error("truncated message: no error")
	}
}

// okMessage round-trips; badMessage re-encodes differently every time.
type okMessage struct {
	h    diam.Header
	body []byte
}

func (m *okMessage) DiamHeader() *diam.Header { return &m.h }
func (m *okMessage) AppendMarshal(b []byte) ([]byte, error) {
	start := len(b)
	b = append(diam.AppendHeader(b, &m.h), m.body...)
	return diam.FinishMessage(b, start)
}
func (m *okMessage) Unmarshal(b []byte) error {
	h, body, err := diam.ParseHeader(b)
	if err != nil {
		return err
	}
	m.h, m.body = h, append([]byte(nil), body...)
	return nil
}
func (m *okMessage) Validate() error { return nil }

type badMessage struct {
	okMessage
	n byte
}

func (m *badMessage) AppendMarshal(b []byte) ([]byte, error) {
	m.n++
	m.h.HopByHop = uint32(m.n)
	return m.okMessage.AppendMarshal(b)
}

func TestCheckRoundTrip(t *testing.T) {
	in := msg(raw(264, 0, diam.AVPFlagMandatory, 'h'))
	if err := CheckRoundTrip(new(okMessage), in); err != nil {
		t.Fatalf("stable message: %v", err)
	}
	if err := CheckRoundTrip(new(okMessage), in[:7]); err != nil {
		t.Fatalf("input that does not unmarshal is not a failure: %v", err)
	}
	if err := CheckRoundTrip(new(badMessage), in); err == nil {
		t.Fatal("unstable re-encoding: no error")
	}
}
