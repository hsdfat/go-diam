package diam

import "encoding/binary"

// AVP flags (RFC 6733 §4.1).
const (
	AVPFlagVendor    uint8 = 0x80
	AVPFlagMandatory uint8 = 0x40
	AVPFlagProtected uint8 = 0x20
)

// RawAVP is an AVP kept verbatim: unknown AVPs, Failed-AVP contents, and
// occurrences beyond a grammar maximum.
type RawAVP struct {
	Code   uint32
	Flags  uint8
	Vendor uint32
	Data   []byte
}

// Mandatory reports whether the M-bit is set.
func (a RawAVP) Mandatory() bool { return a.Flags&AVPFlagMandatory != 0 }

// Clone detaches a RawAVP from the buffer it was parsed from.
func Clone(a RawAVP) RawAVP {
	a.Data = cloneBytes(a.Data)
	return a
}

// cloneBytes copies b. The result is never nil, so an empty value stays
// distinguishable from an absent one.
func cloneBytes(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

// BeginAVP appends an AVP header with a placeholder length and returns the
// offset to pass to EndAVP. The V-bit is set exactly when vendor is not 0.
func BeginAVP(b []byte, code uint32, flags uint8, vendor uint32) ([]byte, int) {
	start := len(b)
	b = binary.BigEndian.AppendUint32(b, code)
	flags &^= AVPFlagVendor
	if vendor != 0 {
		flags |= AVPFlagVendor
	}
	b = append(b, flags, 0, 0, 0)
	if vendor != 0 {
		b = binary.BigEndian.AppendUint32(b, vendor)
	}
	return b, start
}

// EndAVP writes the length of the AVP that starts at offset start and appends
// zero padding up to the next 32-bit boundary.
func EndAVP(b []byte, start int) []byte {
	n := len(b) - start
	b[start+5], b[start+6], b[start+7] = byte(n>>16), byte(n>>8), byte(n)
	for n%4 != 0 {
		b = append(b, 0)
		n++
	}
	return b
}

// AppendRaw re-encodes a RawAVP. The Vendor-Id field is written when the
// V-bit is set or the vendor is not 0, so an AVP parsed by NextAVP is
// reproduced exactly, including a V-bit with Vendor-Id 0.
func AppendRaw(b []byte, a RawAVP) []byte {
	s := len(b)
	flags := a.Flags
	if a.Vendor != 0 {
		flags |= AVPFlagVendor
	}
	b = binary.BigEndian.AppendUint32(b, a.Code)
	b = append(b, flags, 0, 0, 0)
	if flags&AVPFlagVendor != 0 {
		b = binary.BigEndian.AppendUint32(b, a.Vendor)
	}
	b = append(b, a.Data...)
	return EndAVP(b, s)
}

// NextAVP splits the first AVP off b. The returned Data aliases b. A missing
// padding after the last AVP is tolerated, and padding bytes are not checked.
func NextAVP(b []byte) (RawAVP, []byte, error) {
	if len(b) < 8 {
		return RawAVP{}, nil, &AVPError{ResultCode: ResultInvalidAVPLength, Msg: "truncated AVP header"}
	}
	a := RawAVP{Code: binary.BigEndian.Uint32(b), Flags: b[4]}
	n := int(b[5])<<16 | int(b[6])<<8 | int(b[7])
	hl := 8
	if a.Flags&AVPFlagVendor != 0 {
		hl = 12
		if len(b) < 12 {
			return RawAVP{}, nil, &AVPError{ResultCode: ResultInvalidAVPLength, AVP: a, Msg: "truncated vendor AVP header"}
		}
		a.Vendor = binary.BigEndian.Uint32(b[8:])
	}
	if n < hl || n > len(b) {
		return RawAVP{}, nil, &AVPError{ResultCode: ResultInvalidAVPLength, AVP: a, Msg: "bad AVP length"}
	}
	a.Data = b[hl:n]
	p := (n + 3) &^ 3
	if p > len(b) {
		p = len(b)
	}
	return a, b[p:], nil
}
