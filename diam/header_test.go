package diam

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestHeaderRoundTrip(t *testing.T) {
	in := Header{Version: 1, Flags: FlagRequest | FlagProxiable, CommandCode: 0x010203, AppID: 16777251, HopByHop: 0xdeadbeef, EndToEnd: 0x01020304}
	b := AppendHeader(nil, &in)
	b = append(b, 0, 0, 0, 0) // four bytes of body
	b, err := FinishMessage(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != HeaderLen+4 {
		t.Fatalf("len = %d", len(b))
	}
	got, body, err := ParseHeader(b)
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Fatalf("header = %+v, want %+v", got, in)
	}
	if len(body) != 4 {
		t.Fatalf("body len = %d", len(body))
	}

	zero := Header{CommandCode: 257}
	b, _ = FinishMessage(AppendHeader(nil, &zero), 0)
	if b[0] != 1 {
		t.Fatalf("version 0 encoded as %d, want 1", b[0])
	}
}

func TestFinishMessageAtOffset(t *testing.T) {
	prefix := []byte{0xaa, 0xbb, 0xcc}
	b := AppendHeader(append([]byte(nil), prefix...), &Header{CommandCode: 280})
	b, err := FinishMessage(b, len(prefix))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b[:3], prefix) {
		t.Fatal("prefix overwritten")
	}
	if _, _, err := ParseHeader(b[3:]); err != nil {
		t.Fatal(err)
	}
}

func TestFinishMessageTooLong(t *testing.T) {
	b := AppendHeader(nil, &Header{})
	b = append(b, make([]byte, 0x1000000)...)
	if _, err := FinishMessage(b, 0); err == nil {
		t.Fatal("want an error for a message over 0xFFFFFF bytes")
	}
}

func TestParseHeaderRejects(t *testing.T) {
	good, _ := FinishMessage(AppendHeader(nil, &Header{CommandCode: 257}), 0)

	short := good[:19]
	tooLong := append([]byte(nil), good...)
	tooLong[3] = 24 // claims 24 bytes, buffer has 20
	unaligned := append(append([]byte(nil), good...), 0, 0)
	unaligned[3] = 22
	underHeader := append([]byte(nil), good...)
	underHeader[3] = 16

	for name, b := range map[string][]byte{
		"under 20 bytes":         short,
		"length over the buffer": tooLong,
		"length not aligned":     unaligned,
		"length under header":    underHeader,
		"empty":                  nil,
	} {
		if _, _, err := ParseHeader(b); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: err = %v, want ErrMalformed", name, err)
		}
	}
}

func TestParseHeaderIgnoresTrailingBytes(t *testing.T) {
	good, _ := FinishMessage(AppendHeader(nil, &Header{CommandCode: 257}), 0)
	_, body, err := ParseHeader(append(good, 1, 2, 3, 4))
	if err != nil || len(body) != 0 {
		t.Fatalf("body = %x, err = %v", body, err)
	}
}

func TestReadFrame(t *testing.T) {
	m1, _ := FinishMessage(append(AppendHeader(nil, &Header{CommandCode: 257}), 1, 2, 3, 4), 0)
	m2, _ := FinishMessage(AppendHeader(nil, &Header{CommandCode: 280}), 0)
	r := bytes.NewReader(append(append([]byte(nil), m1...), m2...))

	got1, err := ReadFrame(r)
	if err != nil || !bytes.Equal(got1, m1) {
		t.Fatalf("first frame = %x, %v", got1, err)
	}
	got2, err := ReadFrame(r)
	if err != nil || !bytes.Equal(got2, m2) {
		t.Fatalf("second frame = %x, %v", got2, err)
	}
	if _, err := ReadFrame(r); err != io.EOF {
		t.Fatalf("at end: err = %v, want io.EOF", err)
	}

	for name, b := range map[string][]byte{
		"short header": m1[:2],
		"no body":      m1[:4],
		"short body":   m1[:22],
	} {
		if _, err := ReadFrame(bytes.NewReader(b)); err != io.ErrUnexpectedEOF {
			t.Errorf("%s: err = %v, want io.ErrUnexpectedEOF", name, err)
		}
	}

	bad := append([]byte(nil), m2...)
	bad[3] = 8 // length under the header size
	if _, err := ReadFrame(bytes.NewReader(bad)); !errors.Is(err, ErrMalformed) {
		t.Fatalf("length under header: err = %v, want ErrMalformed", err)
	}
}

func TestAnswerHeader(t *testing.T) {
	req := Header{Version: 1, Flags: FlagRequest | FlagProxiable | FlagRetransmit, CommandCode: 316, AppID: 16777251, HopByHop: 7, EndToEnd: 9}
	got := AnswerHeader(&req)
	want := Header{Version: 1, Flags: FlagProxiable, CommandCode: 316, AppID: 16777251, HopByHop: 7, EndToEnd: 9}
	if got != want {
		t.Fatalf("answer header = %+v, want %+v", got, want)
	}
}
