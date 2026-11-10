// Package diam is the runtime for generated Diameter codecs (RFC 6733).
// It depends on the standard library only.
package diam

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Command flags (RFC 6733 §3).
const (
	FlagRequest    uint8 = 0x80
	FlagProxiable  uint8 = 0x40
	FlagError      uint8 = 0x20
	FlagRetransmit uint8 = 0x10
)

// HeaderLen is the fixed Diameter header size.
const HeaderLen = 20

// maxMessageLen is the largest value of the 24-bit Message Length field.
const maxMessageLen = 0xFFFFFF

// ErrMalformed is returned for wire data that cannot be framed.
var ErrMalformed = errors.New("diam: malformed message")

// Header is the Diameter message header. The message length is not stored;
// FinishMessage writes it.
type Header struct {
	Version     uint8
	Flags       uint8
	CommandCode uint32
	AppID       uint32
	HopByHop    uint32
	EndToEnd    uint32
}

// AppendHeader appends a header with a zero length placeholder. Version 0 is
// written as 1, so a zero Header is usable.
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

// FinishMessage writes the length of the message that starts at offset start.
// It fails when the message does not fit the 24-bit length field.
func FinishMessage(b []byte, start int) ([]byte, error) {
	n := len(b) - start
	if n > maxMessageLen {
		return nil, fmt.Errorf("diam: message too long (%d bytes)", n)
	}
	b[start+1], b[start+2], b[start+3] = byte(n>>16), byte(n>>8), byte(n)
	return b, nil
}

// ParseHeader parses the header at the start of b and returns it with the AVP
// section of that message. Bytes after the message length are ignored. The
// AVP section aliases b.
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

// ReadFrame reads exactly one Diameter message from r. It returns io.EOF when
// r is at a message boundary and io.ErrUnexpectedEOF when it ends inside a
// message.
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
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return b, nil
}

// AnswerHeader returns the header of an answer to req: same command,
// application and identifiers, with only the proxiable bit kept.
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
