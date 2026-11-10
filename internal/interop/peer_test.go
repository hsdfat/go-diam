//go:build peer

package interop

import (
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

var update = flag.Bool("update", false, "write the frames received from the peer to testdata/captures/base")

// relayAppID is the Relay application id (RFC 6733 §2.4): advertising it
// gives a common application with any peer.
const relayAppID = 0xffffffff

// peerConn is a TCP connection to the peer under test.
type peerConn struct {
	t    *testing.T
	conn net.Conn
}

func (p *peerConn) send(m diam.Message) {
	p.t.Helper()
	b, err := m.AppendMarshal(nil)
	if err != nil {
		p.t.Fatal(err)
	}
	if _, err := p.conn.Write(b); err != nil {
		p.t.Fatal(err)
	}
}

// answer reads frames until the answer to the given command arrives. The
// peer's own requests (it may send a DWR at any time) are skipped.
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
		p.t.Logf("skipping command %d flags %#x from the peer", h.CommandCode, h.Flags)
	}
}

// TestPeer exchanges CER/CEA, DWR/DWA and DPR/DPA with a real Diameter peer
// at DIAM_PEER_ADDR and checks that go-diam accepts every answer.
func TestPeer(t *testing.T) {
	addr := os.Getenv("DIAM_PEER_ADDR")
	if addr == "" {
		t.Skip("DIAM_PEER_ADDR is not set")
	}
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatal(err)
	}
	p := &peerConn{t: t, conn: conn}

	local, err := netip.ParseAddrPort(conn.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	const host, realm = "client.test", "test"
	frames := map[string][]byte{}

	p.send(&base.CapabilitiesExchangeRequest{
		Header:            diam.Header{HopByHop: 1, EndToEnd: 1},
		OriginHost:        host,
		OriginRealm:       realm,
		HostIpAddress:     []netip.Addr{local.Addr().Unmap()},
		VendorId:          0,
		ProductName:       "go-diam",
		AuthApplicationId: []uint32{relayAppID},
		InbandSecurityId:  []uint32{0}, // NO_INBAND_SECURITY
	})
	frames["cea"] = p.answer(257)
	var cea base.CapabilitiesExchangeAnswer
	if err := cea.Unmarshal(frames["cea"]); err != nil {
		t.Fatal(err)
	}
	if err := cea.Validate(); err != nil {
		t.Fatalf("CEA: Validate: %v", err)
	}
	if cea.ResultCode != 2001 {
		t.Fatalf("CEA: Result-Code %d (%s)", cea.ResultCode, cea.ErrorMessage.V)
	}
	t.Logf("peer %s, product %q, firmware %d", cea.OriginHost, cea.ProductName, cea.FirmwareRevision.V)

	p.send(&base.DeviceWatchdogRequest{Header: diam.Header{HopByHop: 2, EndToEnd: 2}, OriginHost: host, OriginRealm: realm})
	frames["dwa"] = p.answer(280)
	var dwa base.DeviceWatchdogAnswer
	if err := dwa.Unmarshal(frames["dwa"]); err != nil {
		t.Fatal(err)
	}
	if err := dwa.Validate(); err != nil {
		t.Fatalf("DWA: Validate: %v", err)
	}
	if dwa.ResultCode != 2001 {
		t.Fatalf("DWA: Result-Code %d", dwa.ResultCode)
	}

	p.send(&base.DisconnectPeerRequest{
		Header: diam.Header{HopByHop: 3, EndToEnd: 3}, OriginHost: host, OriginRealm: realm,
		DisconnectCause: base.DisconnectCauseRebooting,
	})
	frames["dpa"] = p.answer(282)
	var dpa base.DisconnectPeerAnswer
	if err := dpa.Unmarshal(frames["dpa"]); err != nil {
		t.Fatal(err)
	}
	if err := dpa.Validate(); err != nil {
		t.Fatalf("DPA: Validate: %v", err)
	}
	if dpa.ResultCode != 2001 {
		t.Fatalf("DPA: Result-Code %d", dpa.ResultCode)
	}

	if !*update {
		return
	}
	dir := filepath.Join(capturesDir, "base")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, frame := range frames {
		if err := os.WriteFile(filepath.Join(dir, name+".bin"), frame, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	source := fmt.Sprintf(`Frames received from a Diameter peer by internal/interop/peer_test.go.

Peer:     %s (Product-Name %q, Firmware-Revision %d)
Image:    ci/freediameter/Dockerfile (Debian bookworm package "freediameter")
Date:     %s
Command:  go test -tags peer -run TestPeer -update ./internal/interop/

Generated in CI against freeDiameter; no operator traffic.
`, cea.OriginHost, cea.ProductName, cea.FirmwareRevision.V, time.Now().UTC().Format("2006-01-02"))
	if err := os.WriteFile(filepath.Join(dir, "SOURCE"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}
