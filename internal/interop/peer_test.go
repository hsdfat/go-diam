//go:build peer

package interop

import (
	"bytes"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hsdfat/go-diam/base"
	"github.com/hsdfat/go-diam/diam"
)

var update = flag.Bool("update", false, "write the frames received from the peer to testdata/captures/<dir>")

// relayAppID is the Relay application id (RFC 6733 §2.4): advertising it
// gives a common application with any peer.
const relayAppID = 0xffffffff

// peerConn is a TCP connection to the peer under test.
type peerConn struct {
	t           *testing.T
	conn        net.Conn
	host, realm string // our identity
	id          uint32 // last Hop-by-Hop and End-to-End identifier used
	dir         string // capture directory written with -update
}

// dialPeer connects to the peer at the address in the environment variable
// env, or skips the test when it is not set.
func dialPeer(t *testing.T, env, host, realm, dir string) *peerConn {
	t.Helper()
	addr := os.Getenv(env)
	if addr == "" {
		t.Skipf("%s is not set", env)
	}
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(60 * time.Second)); err != nil {
		t.Fatal(err)
	}
	p := &peerConn{t: t, conn: conn, host: host, realm: realm, dir: filepath.Join(capturesDir, dir)}
	if *update {
		if err := os.MkdirAll(p.dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// localAddr is our end of the connection, for Host-IP-Address.
func (p *peerConn) localAddr() netip.Addr {
	local, err := netip.ParseAddrPort(p.conn.LocalAddr().String())
	if err != nil {
		p.t.Fatal(err)
	}
	return local.Addr().Unmap()
}

// next returns a header with fresh identifiers.
func (p *peerConn) next() diam.Header {
	p.id++
	return diam.Header{HopByHop: p.id, EndToEnd: p.id}
}

// sessionID returns a Session-Id of ours (RFC 6733 §8.8).
func (p *peerConn) sessionID() string {
	return fmt.Sprintf("%s;%d;%d", p.host, time.Now().Unix(), p.id+1)
}

// send writes m and returns its command code.
func (p *peerConn) send(m diam.Message) uint32 {
	p.t.Helper()
	b, err := m.AppendMarshal(nil)
	if err != nil {
		p.t.Fatal(err)
	}
	if _, err := p.conn.Write(b); err != nil {
		p.t.Fatal(err)
	}
	h, _, err := diam.ParseHeader(b)
	if err != nil {
		p.t.Fatal(err)
	}
	return h.CommandCode
}

// answer reads frames until the answer to the given command arrives. A DWR
// from the peer is answered; other requests from it are logged and skipped.
func (p *peerConn) answer(code uint32) []byte {
	p.t.Helper()
	for {
		frame, err := diam.ReadFrame(p.conn)
		if err != nil {
			p.t.Fatalf("reading the answer to command %d: %v", code, err)
		}
		h, _, err := diam.ParseHeader(frame)
		if err != nil {
			p.t.Fatal(err)
		}
		if h.CommandCode == code && h.Flags&diam.FlagRequest == 0 {
			return frame
		}
		if h.CommandCode == 280 && h.Flags&diam.FlagRequest != 0 {
			var dwr base.DeviceWatchdogRequest
			if err := dwr.Unmarshal(frame); err != nil {
				p.t.Fatal(err)
			}
			dwa := base.NewDeviceWatchdogAnswerFor(&dwr)
			dwa.ResultCode, dwa.OriginHost, dwa.OriginRealm = 2001, p.host, p.realm
			p.send(dwa)
			continue
		}
		p.t.Logf("skipping command %d flags %#x from the peer: %x", h.CommandCode, h.Flags, frame)
	}
}

// exchange sends req, reads the answer with the same command code, saves it
// as name.bin with -update, and replays it as TestCaptures will. It returns
// the decoded answer, or nil after reporting an error.
func (p *peerConn) exchange(name string, req diam.Message) diam.Message {
	p.t.Helper()
	frame := p.answer(p.send(req))
	p.t.Logf("%s: %d bytes: %x", name, len(frame), frame)
	if *update {
		if err := os.WriteFile(filepath.Join(p.dir, name+".bin"), frame, 0o644); err != nil {
			p.t.Fatal(err)
		}
	}
	m, err := replay(frame)
	if err != nil {
		p.t.Errorf("%s: %v", name, err)
		return nil
	}
	out, _ := m.AppendMarshal(nil)
	p.t.Logf("%s: decoded as %T; re-encoding byte-identical: %v", name, m, bytes.Equal(out, frame))
	return m
}

// probe sends req and reports on the answer like exchange, but only in the
// log: the answer is not saved and a failed check is not a test failure. It
// is for answers that are not committed as captures.
func (p *peerConn) probe(name string, req diam.Message) diam.Message {
	p.t.Helper()
	frame := p.answer(p.send(req))
	p.t.Logf("%s (probe): %d bytes: %x", name, len(frame), frame)
	m, err := replay(frame)
	if err != nil {
		p.t.Logf("%s (probe): %v", name, err)
	}
	return m
}

// writeSource writes the SOURCE file of the capture directory.
func (p *peerConn) writeSource(text string) {
	p.t.Helper()
	if !*update {
		return
	}
	if err := os.WriteFile(filepath.Join(p.dir, "SOURCE"), []byte(text), 0o644); err != nil {
		p.t.Fatal(err)
	}
}

// capabilities exchanges CER/CEA and returns the CEA.
func (p *peerConn) capabilities(name string, cer *base.CapabilitiesExchangeRequest) *base.CapabilitiesExchangeAnswer {
	p.t.Helper()
	cer.Header = p.next()
	cea, ok := p.exchange(name, cer).(*base.CapabilitiesExchangeAnswer)
	if !ok {
		p.t.Fatal("no valid CEA")
	}
	if cea.ResultCode != 2001 {
		p.t.Fatalf("CEA: Result-Code %d (%s)", cea.ResultCode, cea.ErrorMessage.V)
	}
	p.t.Logf("peer %s, product %q, vendor %d, firmware %d, auth apps %v, vendor apps %+v",
		cea.OriginHost, cea.ProductName, cea.VendorId, cea.FirmwareRevision.V, cea.AuthApplicationId, cea.VendorSpecificApplicationId)
	return cea
}

// TestPeer exchanges the base protocol commands with a real Diameter peer at
// DIAM_PEER_ADDR and checks that go-diam accepts every answer.
func TestPeer(t *testing.T) {
	const host, realm = "client.test", "test"
	p := dialPeer(t, "DIAM_PEER_ADDR", host, realm, "base")

	cea := p.capabilities("cea", &base.CapabilitiesExchangeRequest{
		OriginHost:        host,
		OriginRealm:       realm,
		HostIpAddress:     []netip.Addr{p.localAddr()},
		VendorId:          0,
		ProductName:       "go-diam",
		AuthApplicationId: []uint32{relayAppID},
		InbandSecurityId:  []uint32{0}, // NO_INBAND_SECURITY
	})

	if dwa, ok := p.exchange("dwa", &base.DeviceWatchdogRequest{Header: p.next(), OriginHost: host, OriginRealm: realm}).(*base.DeviceWatchdogAnswer); !ok || dwa.ResultCode != 2001 {
		t.Fatalf("DWA: %+v", dwa)
	}

	// freeDiameter has no application behind RAR, STR and ASR, so it answers
	// each with a protocol error (E bit, 3007). It clears the P bit in those
	// answers, where RFC 6733 §6.2 and §7.2 say the P bit is the request's,
	// so they are probed here but not committed: see docs/design.md.
	for _, req := range []struct {
		name string
		m    diam.Message
	}{
		{"sta", &base.SessionTerminationRequest{
			Header: p.next(), SessionId: p.sessionID(), OriginHost: host, OriginRealm: realm,
			DestinationRealm: realm, DestinationHost: diam.Some(string(cea.OriginHost)),
			AuthApplicationId: 0, TerminationCause: base.TerminationCauseLogout,
		}},
		{"asa", &base.AbortSessionRequest{
			Header: p.next(), SessionId: p.sessionID(), OriginHost: host, OriginRealm: realm,
			DestinationRealm: realm, DestinationHost: string(cea.OriginHost), AuthApplicationId: 0,
		}},
		{"raa", &base.ReAuthRequest{
			Header: p.next(), SessionId: p.sessionID(), OriginHost: host, OriginRealm: realm,
			DestinationRealm: realm, DestinationHost: string(cea.OriginHost), AuthApplicationId: 0,
			ReAuthRequestType: base.ReAuthRequestTypeAuthorizeOnly,
		}},
	} {
		p.probe(req.name, req.m)
	}

	if dpa, ok := p.exchange("dpa", &base.DisconnectPeerRequest{
		Header: p.next(), OriginHost: host, OriginRealm: realm,
		DisconnectCause: base.DisconnectCauseRebooting,
	}).(*base.DisconnectPeerAnswer); !ok || dpa.ResultCode != 2001 {
		t.Fatalf("DPA: %+v", dpa)
	}

	p.writeSource(fmt.Sprintf(`Frames received from a Diameter peer by internal/interop/peer_test.go.

Peer:     %s (Product-Name %q, Firmware-Revision %d)
Image:    ci/freediameter/Dockerfile (Debian bookworm package "freediameter")
Date:     %s
Command:  go test -tags peer -run 'TestPeer$' -update ./internal/interop/

Generated in CI against freeDiameter; no operator traffic.
`, cea.OriginHost, cea.ProductName, cea.FirmwareRevision.V, time.Now().UTC().Format("2006-01-02")))
}
