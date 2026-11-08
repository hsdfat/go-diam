// Package diam is the runtime for generated Diameter codecs (RFC 6733).
// It depends on the standard library only.
package diam

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/netip"
	"time"
)

// Command flags.
const (
	FlagRequest    uint8 = 0x80
	FlagProxiable  uint8 = 0x40
	FlagError      uint8 = 0x20
	FlagRetransmit uint8 = 0x10
)

// AVP flags.
const (
	AVPFlagVendor    uint8 = 0x80
	AVPFlagMandatory uint8 = 0x40
	AVPFlagProtected uint8 = 0x20
)

// Result-Code values used by Validate.
const (
	ResultAVPUnsupported     uint32 = 5001
	ResultInvalidAVPValue    uint32 = 5004
	ResultMissingAVP         uint32 = 5005
	ResultAVPOccursTooMany   uint32 = 5009
	ResultInvalidAVPLength   uint32 = 5014
	ResultInvalidMessageLen  uint32 = 5015
	ResultInvalidAVPBitCombo uint32 = 3009
)

// HeaderLen is the fixed Diameter header size.
const HeaderLen = 20

// Header is the Diameter message header. Length is filled in by Marshal.
type Header struct {
	Version     uint8
	Flags       uint8
	CommandCode uint32
	AppID       uint32
	HopByHop    uint32
	EndToEnd    uint32
}

// Message is implemented by every generated command.
type Message interface {
	DiamHeader() *Header
	AppendMarshal(b []byte) ([]byte, error)
	Unmarshal(b []byte) error
	Validate() error
}

// Opt is an optional scalar AVP: value plus presence, no allocation.
type Opt[T any] struct {
	V   T
	Set bool
}

// Some returns a present Opt.
func Some[T any](v T) Opt[T] { return Opt[T]{V: v, Set: true} }

// Get returns the value and whether it is present.
func (o Opt[T]) Get() (T, bool) { return o.V, o.Set }

// RawAVP is an AVP kept verbatim: unknown AVPs, Failed-AVP contents, excess occurrences.
type RawAVP struct {
	Code   uint32
	Flags  uint8
	Vendor uint32
	Data   []byte
}

// Mandatory reports whether the M-bit is set.
func (a RawAVP) Mandatory() bool { return a.Flags&AVPFlagMandatory != 0 }

// AVPError describes a protocol violation with the Result-Code to answer with.
// AVP carries the offending AVP for Failed-AVP when one exists.
type AVPError struct {
	ResultCode uint32
	AVP        RawAVP
	Msg        string
}

func (e *AVPError) Error() string {
	return fmt.Sprintf("diam: %s (result-code %d, avp %d vendor %d)", e.Msg, e.ResultCode, e.AVP.Code, e.AVP.Vendor)
}

// ErrMalformed is returned for wire data that cannot be framed.
var ErrMalformed = errors.New("diam: malformed message")

// ---------------------------------------------------------------------------
// Header

// AppendHeader appends a header with a zero length placeholder.
func AppendHeader(b []byte, h *Header) []byte {
	v := h.Version
	if v == 0 {
		v = 1
	}
	b = append(b, v, 0, 0, 0, h.Flags)
	b = append(b, byte(h.CommandCode>>16), byte(h.CommandCode>>8), byte(h.CommandCode))
	b = binary.BigEndian.AppendUint32(b, h.AppID)
	b = binary.BigEndian.AppendUint32(b, h.HopByHop)
	b = binary.BigEndian.AppendUint32(b, h.EndToEnd)
	return b
}

// FinishMessage patches the message length of the message starting at start.
func FinishMessage(b []byte, start int) ([]byte, error) {
	n := len(b) - start
	if n > 0xFFFFFF {
		return nil, fmt.Errorf("diam: message too long (%d bytes)", n)
	}
	b[start+1], b[start+2], b[start+3] = byte(n>>16), byte(n>>8), byte(n)
	return b, nil
}

// ParseHeader parses the header and returns it with the AVP section.
func ParseHeader(b []byte) (Header, []byte, error) {
	if len(b) < HeaderLen {
		return Header{}, nil, ErrMalformed
	}
	n := int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if n < HeaderLen || n > len(b) || n%4 != 0 {
		return Header{}, nil, ErrMalformed
	}
	h := Header{
		Version:     b[0],
		Flags:       b[4],
		CommandCode: uint32(b[5])<<16 | uint32(b[6])<<8 | uint32(b[7]),
		AppID:       binary.BigEndian.Uint32(b[8:]),
		HopByHop:    binary.BigEndian.Uint32(b[12:]),
		EndToEnd:    binary.BigEndian.Uint32(b[16:]),
	}
	return h, b[HeaderLen:n], nil
}

// ReadFrame reads exactly one Diameter message from r.
func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := int(hdr[1])<<16 | int(hdr[2])<<8 | int(hdr[3])
	if n < HeaderLen {
		return nil, ErrMalformed
	}
	b := make([]byte, n)
	copy(b, hdr[:])
	if _, err := io.ReadFull(r, b[4:]); err != nil {
		return nil, err
	}
	return b, nil
}

// AnswerHeader returns the header of an answer to req: same command, app and
// identifiers, request bit cleared, proxiable bit kept.
func AnswerHeader(req *Header) Header {
	return Header{
		Version:     1,
		Flags:       req.Flags & FlagProxiable,
		CommandCode: req.CommandCode,
		AppID:       req.AppID,
		HopByHop:    req.HopByHop,
		EndToEnd:    req.EndToEnd,
	}
}

// ---------------------------------------------------------------------------
// AVP framing

// BeginAVP appends an AVP header with a placeholder length and returns the
// offset to pass to EndAVP.
func BeginAVP(b []byte, code uint32, flags uint8, vendor uint32) ([]byte, int) {
	start := len(b)
	b = binary.BigEndian.AppendUint32(b, code)
	if vendor != 0 {
		flags |= AVPFlagVendor
	}
	b = append(b, flags, 0, 0, 0)
	if vendor != 0 {
		b = binary.BigEndian.AppendUint32(b, vendor)
	}
	return b, start
}

// EndAVP patches the AVP length and appends padding.
func EndAVP(b []byte, start int) []byte {
	n := len(b) - start
	b[start+5], b[start+6], b[start+7] = byte(n>>16), byte(n>>8), byte(n)
	for n%4 != 0 {
		b = append(b, 0)
		n++
	}
	return b
}

// AppendRaw re-encodes a RawAVP.
func AppendRaw(b []byte, a RawAVP) []byte {
	b, s := BeginAVP(b, a.Code, a.Flags&^AVPFlagVendor, a.Vendor)
	b = append(b, a.Data...)
	return EndAVP(b, s)
}

// NextAVP splits the first AVP off b. Data aliases b.
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
		p = len(b) // tolerate missing padding on the last AVP
	}
	return a, b[p:], nil
}

// ---------------------------------------------------------------------------
// Base data types: encode

func AppendUnsigned32(b []byte, v uint32) []byte { return binary.BigEndian.AppendUint32(b, v) }
func AppendUnsigned64(b []byte, v uint64) []byte { return binary.BigEndian.AppendUint64(b, v) }
func AppendInteger32(b []byte, v int32) []byte   { return binary.BigEndian.AppendUint32(b, uint32(v)) }
func AppendInteger64(b []byte, v int64) []byte   { return binary.BigEndian.AppendUint64(b, uint64(v)) }
func AppendFloat32(b []byte, v float32) []byte {
	return binary.BigEndian.AppendUint32(b, math.Float32bits(v))
}
func AppendFloat64(b []byte, v float64) []byte {
	return binary.BigEndian.AppendUint64(b, math.Float64bits(v))
}

// ntpEpochOffset is the seconds between 1900-01-01 and 1970-01-01.
const ntpEpochOffset = 2208988800

// AppendTime encodes RFC 6733 Time (NTP seconds, 32 bit).
func AppendTime(b []byte, t time.Time) []byte {
	return binary.BigEndian.AppendUint32(b, uint32(t.Unix()+ntpEpochOffset))
}

// AppendAddress encodes RFC 6733 Address (2-byte family + bytes).
func AppendAddress(b []byte, a netip.Addr) []byte {
	if a.Is4() {
		v := a.As4()
		return append(append(b, 0, 1), v[:]...)
	}
	v := a.As16()
	return append(append(b, 0, 2), v[:]...)
}

// ---------------------------------------------------------------------------
// Base data types: decode

func lenErr(a RawAVP) error {
	return &AVPError{ResultCode: ResultInvalidAVPLength, AVP: a, Msg: "invalid data length"}
}

func Unsigned32(a RawAVP) (uint32, error) {
	if len(a.Data) != 4 {
		return 0, lenErr(a)
	}
	return binary.BigEndian.Uint32(a.Data), nil
}

func Unsigned64(a RawAVP) (uint64, error) {
	if len(a.Data) != 8 {
		return 0, lenErr(a)
	}
	return binary.BigEndian.Uint64(a.Data), nil
}

func Integer32(a RawAVP) (int32, error) { v, err := Unsigned32(a); return int32(v), err }
func Integer64(a RawAVP) (int64, error) { v, err := Unsigned64(a); return int64(v), err }

func Float32(a RawAVP) (float32, error) {
	v, err := Unsigned32(a)
	return math.Float32frombits(v), err
}

func Float64(a RawAVP) (float64, error) {
	v, err := Unsigned64(a)
	return math.Float64frombits(v), err
}

// String copies the data of an OctetString-derived text AVP.
func String(a RawAVP) (string, error) { return string(a.Data), nil }

// Bytes copies OctetString data so the message does not alias the input buffer.
func Bytes(a RawAVP) ([]byte, error) { return append([]byte(nil), a.Data...), nil }

func Time(a RawAVP) (time.Time, error) {
	v, err := Unsigned32(a)
	if err != nil {
		return time.Time{}, err
	}
	// RFC 6733 §4.3.1: values with the high bit clear wrap past 2036.
	s := int64(v) - ntpEpochOffset
	if v&0x80000000 == 0 {
		s += 1 << 32
	}
	return time.Unix(s, 0).UTC(), nil
}

func Address(a RawAVP) (netip.Addr, error) {
	if len(a.Data) < 2 {
		return netip.Addr{}, lenErr(a)
	}
	fam := binary.BigEndian.Uint16(a.Data)
	switch {
	case fam == 1 && len(a.Data) == 6:
		return netip.AddrFrom4([4]byte(a.Data[2:6])), nil
	case fam == 2 && len(a.Data) == 18:
		return netip.AddrFrom16([16]byte(a.Data[2:18])), nil
	}
	return netip.Addr{}, &AVPError{ResultCode: ResultInvalidAVPValue, AVP: a, Msg: "unsupported address family"}
}

// Clone detaches a RawAVP from the input buffer.
func Clone(a RawAVP) RawAVP {
	a.Data = append([]byte(nil), a.Data...)
	return a
}

// ---------------------------------------------------------------------------
// Validation helpers used by generated code.

// Missing reports a required AVP that is absent.
func Missing(code, vendor uint32, name string) error {
	return &AVPError{ResultCode: ResultMissingAVP, AVP: RawAVP{Code: code, Vendor: vendor}, Msg: "missing required AVP " + name}
}

// TooMany reports an AVP that exceeds its maximum occurrence.
func TooMany(a RawAVP, name string) error {
	return &AVPError{ResultCode: ResultAVPOccursTooMany, AVP: a, Msg: "AVP occurs too many times: " + name}
}

// TooFew reports a repeated AVP under its minimum occurrence.
func TooFew(code, vendor uint32, name string, min int) error {
	return &AVPError{ResultCode: ResultMissingAVP, AVP: RawAVP{Code: code, Vendor: vendor}, Msg: fmt.Sprintf("AVP %s needs at least %d occurrences", name, min)}
}

// CheckUnknown returns a 5001 error for the first unknown AVP with the M-bit set.
// For a non-extensible group any unknown AVP is a 5001 when M is set; when M is
// clear it is ignored per RFC 6733 §4.1.
func CheckUnknown(unknown []RawAVP) error {
	for _, a := range unknown {
		if a.Mandatory() {
			return &AVPError{ResultCode: ResultAVPUnsupported, AVP: a, Msg: "unsupported mandatory AVP"}
		}
	}
	return nil
}
