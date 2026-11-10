package diam

import (
	"encoding/binary"
	"math"
	"net/netip"
	"time"
)

// Opt is an optional scalar AVP: value plus presence, no allocation.
type Opt[T any] struct {
	V   T
	Set bool
}

// Some returns a present Opt.
func Some[T any](v T) Opt[T] { return Opt[T]{V: v, Set: true} }

// Get returns the value and whether it is present.
func (o Opt[T]) Get() (T, bool) { return o.V, o.Set }

// Encoders for the base data types (RFC 6733 §4.2, §4.3). Each appends the
// AVP data only; BeginAVP and EndAVP write the header and padding.

// AppendUnsigned32 appends an Unsigned32 value.
func AppendUnsigned32(b []byte, v uint32) []byte { return binary.BigEndian.AppendUint32(b, v) }

// AppendUnsigned64 appends an Unsigned64 value.
func AppendUnsigned64(b []byte, v uint64) []byte { return binary.BigEndian.AppendUint64(b, v) }

// AppendInteger32 appends an Integer32 value.
func AppendInteger32(b []byte, v int32) []byte { return binary.BigEndian.AppendUint32(b, uint32(v)) }

// AppendInteger64 appends an Integer64 value.
func AppendInteger64(b []byte, v int64) []byte { return binary.BigEndian.AppendUint64(b, uint64(v)) }

// AppendFloat32 appends a Float32 value.
func AppendFloat32(b []byte, v float32) []byte {
	return binary.BigEndian.AppendUint32(b, math.Float32bits(v))
}

// AppendFloat64 appends a Float64 value.
func AppendFloat64(b []byte, v float64) []byte {
	return binary.BigEndian.AppendUint64(b, math.Float64bits(v))
}

// ntpEpochOffset is the number of seconds from 1900-01-01 to 1970-01-01.
const ntpEpochOffset = 2208988800

// AppendTime appends a Time value: the low 32 bits of the NTP seconds.
func AppendTime(b []byte, t time.Time) []byte {
	return binary.BigEndian.AppendUint32(b, uint32(t.Unix()+ntpEpochOffset))
}

// Address families (IANA Address Family Numbers).
const (
	addrFamilyIPv4 = 1
	addrFamilyIPv6 = 2
)

// AppendAddress appends an Address value: a 2-byte family and the address.
func AppendAddress(b []byte, a netip.Addr) []byte {
	if a.Is4() {
		v := a.As4()
		return append(append(b, 0, addrFamilyIPv4), v[:]...)
	}
	v := a.As16()
	return append(append(b, 0, addrFamilyIPv6), v[:]...)
}

func lenErr(a RawAVP) error {
	return &AVPError{ResultCode: ResultInvalidAVPLength, AVP: Clone(a), Msg: "invalid data length"}
}

// Unsigned32 decodes an Unsigned32 AVP.
func Unsigned32(a RawAVP) (uint32, error) {
	if len(a.Data) != 4 {
		return 0, lenErr(a)
	}
	return binary.BigEndian.Uint32(a.Data), nil
}

// Unsigned64 decodes an Unsigned64 AVP.
func Unsigned64(a RawAVP) (uint64, error) {
	if len(a.Data) != 8 {
		return 0, lenErr(a)
	}
	return binary.BigEndian.Uint64(a.Data), nil
}

// Integer32 decodes an Integer32 or Enumerated AVP.
func Integer32(a RawAVP) (int32, error) {
	v, err := Unsigned32(a)
	return int32(v), err
}

// Integer64 decodes an Integer64 AVP.
func Integer64(a RawAVP) (int64, error) {
	v, err := Unsigned64(a)
	return int64(v), err
}

// Float32 decodes a Float32 AVP.
func Float32(a RawAVP) (float32, error) {
	v, err := Unsigned32(a)
	return math.Float32frombits(v), err
}

// Float64 decodes a Float64 AVP.
func Float64(a RawAVP) (float64, error) {
	v, err := Unsigned64(a)
	return math.Float64frombits(v), err
}

// String decodes a UTF8String, DiameterIdentity or DiameterURI AVP. The
// result does not alias the input.
func String(a RawAVP) (string, error) { return string(a.Data), nil }

// Bytes decodes an OctetString AVP into a copy that does not alias the input.
// The result is never nil.
func Bytes(a RawAVP) ([]byte, error) { return cloneBytes(a.Data), nil }

// Time decodes a Time AVP. Per RFC 6733 §4.3.1 a value with the high bit
// clear is in the NTP era that starts on 2036-02-07.
func Time(a RawAVP) (time.Time, error) {
	v, err := Unsigned32(a)
	if err != nil {
		return time.Time{}, err
	}
	s := int64(v) - ntpEpochOffset
	if v&0x80000000 == 0 {
		s += 1 << 32
	}
	return time.Unix(s, 0).UTC(), nil
}

// Address decodes an Address AVP. IPv4 and IPv6 are supported; another
// address family is reported as an invalid value (5004), a wrong length for
// a known family as an invalid length (5014).
func Address(a RawAVP) (netip.Addr, error) {
	if len(a.Data) < 2 {
		return netip.Addr{}, lenErr(a)
	}
	switch binary.BigEndian.Uint16(a.Data) {
	case addrFamilyIPv4:
		if len(a.Data) != 6 {
			return netip.Addr{}, lenErr(a)
		}
		return netip.AddrFrom4([4]byte(a.Data[2:6])), nil
	case addrFamilyIPv6:
		if len(a.Data) != 18 {
			return netip.Addr{}, lenErr(a)
		}
		return netip.AddrFrom16([16]byte(a.Data[2:18])), nil
	}
	return netip.Addr{}, &AVPError{ResultCode: ResultInvalidAVPValue, AVP: Clone(a), Msg: "unsupported address family"}
}
