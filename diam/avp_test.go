package diam

import (
	"bytes"
	"errors"
	"testing"
)

func wantResult(t *testing.T, err error, code uint32) *AVPError {
	t.Helper()
	var ae *AVPError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v, want *AVPError with result-code %d", err, code)
	}
	if ae.ResultCode != code {
		t.Fatalf("result-code = %d, want %d (%v)", ae.ResultCode, code, err)
	}
	return ae
}

func TestAVPRoundTrip(t *testing.T) {
	b, s := BeginAVP(nil, 264, AVPFlagMandatory, 0)
	b = append(b, "host"...)
	b = EndAVP(b, s)
	b, s = BeginAVP(b, 1407, AVPFlagMandatory, 10415)
	b = append(b, 1, 2, 3)
	b = EndAVP(b, s)

	a, rest, err := NextAVP(b)
	if err != nil {
		t.Fatal(err)
	}
	if a.Code != 264 || a.Vendor != 0 || a.Flags != AVPFlagMandatory || string(a.Data) != "host" || !a.Mandatory() {
		t.Fatalf("first AVP = %+v", a)
	}
	a, rest, err = NextAVP(rest)
	if err != nil {
		t.Fatal(err)
	}
	if a.Code != 1407 || a.Vendor != 10415 || a.Flags != AVPFlagMandatory|AVPFlagVendor || !bytes.Equal(a.Data, []byte{1, 2, 3}) {
		t.Fatalf("second AVP = %+v", a)
	}
	if len(rest) != 0 {
		t.Fatalf("rest = %x", rest)
	}
	if len(b)%4 != 0 {
		t.Fatalf("encoded length %d is not padded", len(b))
	}
}

func TestNextAVPMalformed(t *testing.T) {
	cases := map[string][]byte{
		"empty header":                {0, 0, 1, 8},
		"length 7":                    {0, 0, 1, 8, 0, 0, 0, 7},
		"vendor flag with 10 bytes":   {0, 0, 1, 8, 0x80, 0, 0, 12, 0, 0},
		"vendor flag, length 8":       {0, 0, 1, 8, 0x80, 0, 0, 8, 0, 0, 0x28, 0xaf},
		"length over remaining bytes": {0, 0, 1, 8, 0, 0, 0, 16, 1, 2, 3, 4},
		"length far over the buffer":  {0, 0, 1, 8, 0, 0xff, 0xff, 0xff},
	}
	for name, b := range cases {
		_, _, err := NextAVP(b)
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		var ae *AVPError
		if !errors.As(err, &ae) || ae.ResultCode != ResultInvalidAVPLength {
			t.Errorf("%s: err = %v, want result-code 5014", name, err)
		}
	}
}

func TestNextAVPPadding(t *testing.T) {
	// Code 1, no flags, length 9, one data byte, then padding.
	clean := []byte{0, 0, 0, 1, 0, 0, 0, 9, 0x41, 0, 0, 0}
	a, rest, err := NextAVP(clean)
	if err != nil || string(a.Data) != "A" || len(rest) != 0 {
		t.Fatalf("clean: %+v rest=%x err=%v", a, rest, err)
	}

	noPad := clean[:9]
	a, rest, err = NextAVP(noPad)
	if err != nil || string(a.Data) != "A" || len(rest) != 0 {
		t.Fatalf("last AVP without padding: %+v rest=%x err=%v", a, rest, err)
	}

	dirty := []byte{0, 0, 0, 1, 0, 0, 0, 9, 0x41, 0xff, 0xee, 0xdd, 0, 0, 0, 2, 0, 0, 0, 8}
	a, rest, err = NextAVP(dirty)
	if err != nil || string(a.Data) != "A" {
		t.Fatalf("non-zero padding: %+v err=%v", a, err)
	}
	next, rest, err := NextAVP(rest)
	if err != nil || next.Code != 2 || len(rest) != 0 {
		t.Fatalf("AVP after non-zero padding: %+v rest=%x err=%v", next, rest, err)
	}

	if out := AppendRaw(nil, a); !bytes.Equal(out, clean) {
		t.Fatalf("AppendRaw = %x, want zero padding %x", out, clean)
	}
}

func TestAppendRawVendor(t *testing.T) {
	in := RawAVP{Code: 1407, Flags: AVPFlagVendor | AVPFlagMandatory, Vendor: 10415, Data: []byte{9}}
	b := AppendRaw(nil, in)
	got, rest, err := NextAVP(b)
	if err != nil || len(rest) != 0 {
		t.Fatal(err, rest)
	}
	if got.Code != in.Code || got.Flags != in.Flags || got.Vendor != in.Vendor || !bytes.Equal(got.Data, in.Data) {
		t.Fatalf("got %+v, want %+v", got, in)
	}
}

func TestCloneDetaches(t *testing.T) {
	buf := []byte{0, 0, 0, 1, 0, 0, 0, 12, 1, 2, 3, 4}
	a, _, err := NextAVP(buf)
	if err != nil {
		t.Fatal(err)
	}
	c := Clone(a)
	buf[8] = 0xff
	if c.Data[0] != 1 {
		t.Fatal("Clone still aliases the input buffer")
	}
}
