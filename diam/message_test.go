package diam

import (
	"errors"
	"strings"
	"testing"
)

// stubMessage is a minimal Message for registry tests.
type stubMessage struct {
	Header     Header
	unmarshals int
	body       []byte
}

func (m *stubMessage) DiamHeader() *Header { return &m.Header }

func (m *stubMessage) AppendMarshal(b []byte) ([]byte, error) {
	start := len(b)
	b = AppendHeader(b, &m.Header)
	b = append(b, m.body...)
	return FinishMessage(b, start)
}

func (m *stubMessage) Unmarshal(b []byte) error {
	h, body, err := ParseHeader(b)
	if err != nil {
		return err
	}
	m.Header, m.body = h, body
	m.unmarshals++
	return nil
}

func (m *stubMessage) Validate() error { return nil }

func TestRegisterDuplicatePanics(t *testing.T) {
	newFn := func() Message { return new(stubMessage) }
	Register(98, 1, true, newFn)
	Register(98, 1, false, newFn) // same command, answer: not a duplicate
	Register(98, 2, true, newFn)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("duplicate Register did not panic")
		}
		if msg, _ := r.(string); !strings.Contains(msg, "98") {
			t.Fatalf("panic = %v, want it to name the application", r)
		}
	}()
	Register(98, 1, true, newFn)
}

func TestDecodeUnregistered(t *testing.T) {
	m := &stubMessage{Header: Header{Flags: FlagRequest, CommandCode: 4242, AppID: 97}}
	b, _ := m.AppendMarshal(nil)
	_, err := Decode(b)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"97", "4242", "request"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

func TestDecodeDispatch(t *testing.T) {
	Register(99, 1, true, func() Message { return new(stubMessage) })

	in := &stubMessage{Header: Header{Version: 1, Flags: FlagRequest, CommandCode: 1, AppID: 99, HopByHop: 5}, body: []byte{0, 0, 0, 1, 0, 0, 0, 8}}
	b, _ := in.AppendMarshal(nil)
	m, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := m.(*stubMessage)
	if !ok {
		t.Fatalf("Decode returned %T", m)
	}
	if got.unmarshals != 1 || got.Header.HopByHop != 5 || len(got.body) != 8 {
		t.Fatalf("decoded = %+v", got)
	}

	// The answer of the same command is a different registration.
	in.Header.Flags = 0
	b, _ = in.AppendMarshal(nil)
	if _, err := Decode(b); err == nil {
		t.Fatal("answer decoded although only the request is registered")
	}

	if _, err := Decode(b[:10]); !errors.Is(err, ErrMalformed) {
		t.Fatalf("truncated input: err = %v, want ErrMalformed", err)
	}
}
