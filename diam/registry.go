package diam

import (
	"fmt"
	"sync"
)

type regKey struct {
	app, code uint32
	request   bool
}

var (
	regMu sync.RWMutex
	reg   = map[regKey]func() Message{}
)

// Register makes a generated command decodable by Decode. Generated packages
// call it from init.
func Register(appID, code uint32, request bool, newFn func() Message) {
	regMu.Lock()
	defer regMu.Unlock()
	reg[regKey{appID, code, request}] = newFn
}

// Decode picks the registered type for b's (app, command, R-bit) and unmarshals it.
func Decode(b []byte) (Message, error) {
	h, _, err := ParseHeader(b)
	if err != nil {
		return nil, err
	}
	regMu.RLock()
	newFn := reg[regKey{h.AppID, h.CommandCode, h.Flags&FlagRequest != 0}]
	regMu.RUnlock()
	if newFn == nil {
		return nil, fmt.Errorf("diam: no type registered for app %d command %d request=%v", h.AppID, h.CommandCode, h.Flags&FlagRequest != 0)
	}
	m := newFn()
	if err := m.Unmarshal(b); err != nil {
		return nil, err
	}
	return m, nil
}
