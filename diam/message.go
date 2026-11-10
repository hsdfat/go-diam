package diam

import (
	"fmt"
	"sync"
)

// Message is implemented by every generated command.
type Message interface {
	// DiamHeader returns the message header for reading and writing.
	DiamHeader() *Header
	// AppendMarshal appends the encoded message to b.
	AppendMarshal(b []byte) ([]byte, error)
	// Unmarshal decodes a whole message. It fails only when the message
	// cannot be framed; grammar violations are reported by Validate.
	Unmarshal(b []byte) error
	// Validate checks the message against its grammar and returns the first
	// violation as an *AVPError.
	Validate() error
}

// Group is implemented by every generated grouped AVP, and by commands, so
// that types from different packages can be nested.
type Group interface {
	AppendAVPs(b []byte) []byte
	DecodeAVPs(data []byte) error
	Validate() error
}

type regKey struct {
	app, code uint32
	request   bool
}

var (
	regMu sync.RWMutex
	reg   = map[regKey]func() Message{}
)

func kindName(request bool) string {
	if request {
		return "request"
	}
	return "answer"
}

// Register makes a command decodable by Decode. Generated packages call it
// from init. Registering the same (application, command, R-bit) twice panics.
func Register(appID, code uint32, request bool, newFn func() Message) {
	k := regKey{appID, code, request}
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := reg[k]; dup {
		panic(fmt.Sprintf("diam: duplicate registration for app %d command %d %s", appID, code, kindName(request)))
	}
	reg[k] = newFn
}

// Decode unmarshals b into the type registered for its application, command
// code and R-bit. The package that defines the command must be imported.
func Decode(b []byte) (Message, error) {
	h, _, err := ParseHeader(b)
	if err != nil {
		return nil, err
	}
	request := h.Flags&FlagRequest != 0
	regMu.RLock()
	newFn := reg[regKey{h.AppID, h.CommandCode, request}]
	regMu.RUnlock()
	if newFn == nil {
		return nil, fmt.Errorf("diam: no type registered for app %d command %d %s", h.AppID, h.CommandCode, kindName(request))
	}
	m := newFn()
	if err := m.Unmarshal(b); err != nil {
		return nil, err
	}
	return m, nil
}
