package diam

import (
	"math"
	"net/netip"
	"testing"
	"time"
)

func raw(data ...byte) RawAVP { return RawAVP{Code: 1, Data: data} }

func TestOpt(t *testing.T) {
	var o Opt[uint32]
	if v, ok := o.Get(); ok || v != 0 {
		t.Fatalf("zero Opt = %d, %v", v, ok)
	}
	if v, ok := Some(uint32(0)).Get(); !ok || v != 0 {
		t.Fatalf("Some(0) = %d, %v", v, ok)
	}
}

func TestTimeWrap(t *testing.T) {
	got, err := Time(raw(0x80, 0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(1968, 1, 20, 3, 14, 8, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("0x80000000 = %v, want %v", got, want)
	}

	// RFC 6733 §4.3.1: with the high bit clear the value is after 2036-02-07.
	got, err = Time(raw(0, 0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2036, 2, 7, 6, 28, 17, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("0x00000001 = %v, want %v", got, want)
	}

	in := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	got, err = Time(RawAVP{Data: AppendTime(nil, in)})
	if err != nil || !got.Equal(in) {
		t.Fatalf("round trip = %v, %v", got, err)
	}
	after := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err = Time(RawAVP{Data: AppendTime(nil, after)})
	if err != nil || !got.Equal(after) {
		t.Fatalf("round trip past 2036 = %v, %v", got, err)
	}

	if _, err := Time(raw(1, 2, 3)); err == nil {
		t.Fatal("3-byte Time: no error")
	} else {
		wantResult(t, err, ResultInvalidAVPLength)
	}
}

func TestAddress(t *testing.T) {
	for _, s := range []string{"10.0.0.1", "2001:db8::1"} {
		in := netip.MustParseAddr(s)
		got, err := Address(RawAVP{Data: AppendAddress(nil, in)})
		if err != nil || got != in {
			t.Fatalf("%s: got %v, %v", s, got, err)
		}
	}

	// Family 8 is E.164: valid on the wire, not representable as netip.Addr.
	_, err := Address(raw(0, 8, '1', '2', '3'))
	wantResult(t, err, ResultInvalidAVPValue)

	// Family 1 (IPv4) with 5 bytes of data in total.
	_, err = Address(raw(0, 1, 10, 0, 0))
	wantResult(t, err, ResultInvalidAVPLength)

	_, err = Address(raw(0, 2, 1, 2, 3, 4))
	wantResult(t, err, ResultInvalidAVPLength)

	_, err = Address(raw(0))
	wantResult(t, err, ResultInvalidAVPLength)
}

func TestIntegerSigns(t *testing.T) {
	i32, err := Integer32(RawAVP{Data: AppendInteger32(nil, -2)})
	if err != nil || i32 != -2 {
		t.Fatalf("Integer32 = %d, %v", i32, err)
	}
	i64, err := Integer64(RawAVP{Data: AppendInteger64(nil, -12345)})
	if err != nil || i64 != -12345 {
		t.Fatalf("Integer64 = %d, %v", i64, err)
	}
	u32, err := Unsigned32(RawAVP{Data: AppendUnsigned32(nil, math.MaxUint32)})
	if err != nil || u32 != math.MaxUint32 {
		t.Fatalf("Unsigned32 = %d, %v", u32, err)
	}
	u64, err := Unsigned64(RawAVP{Data: AppendUnsigned64(nil, 1<<40)})
	if err != nil || u64 != 1<<40 {
		t.Fatalf("Unsigned64 = %d, %v", u64, err)
	}

	_, err = Unsigned32(raw(1, 2, 3))
	ae := wantResult(t, err, ResultInvalidAVPLength)
	if ae.AVP.Code != 1 {
		t.Fatalf("error does not carry the AVP: %+v", ae.AVP)
	}
	_, err = Integer64(raw(1, 2, 3, 4))
	wantResult(t, err, ResultInvalidAVPLength)
}

func TestFloats(t *testing.T) {
	f32, err := Float32(RawAVP{Data: AppendFloat32(nil, 1.5)})
	if err != nil || f32 != 1.5 {
		t.Fatalf("Float32 = %v, %v", f32, err)
	}
	f64, err := Float64(RawAVP{Data: AppendFloat64(nil, -1.5)})
	if err != nil || f64 != -1.5 {
		t.Fatalf("Float64 = %v, %v", f64, err)
	}
	_, err = Float64(raw(1, 2, 3, 4))
	wantResult(t, err, ResultInvalidAVPLength)
}

func TestStringAndBytesCopy(t *testing.T) {
	buf := []byte("abc")
	a := RawAVP{Data: buf}
	s, err := String(a)
	if err != nil || s != "abc" {
		t.Fatal(s, err)
	}
	b, err := Bytes(a)
	if err != nil || string(b) != "abc" {
		t.Fatal(b, err)
	}
	buf[0] = 'z'
	if b[0] != 'a' {
		t.Fatal("Bytes aliases the input buffer")
	}

	empty, err := Bytes(RawAVP{})
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty OctetString = %#v, %v; want a non-nil empty slice", empty, err)
	}
}
