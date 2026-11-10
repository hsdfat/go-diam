package main

import (
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

// headerFlags is the Go expression for the R and P bits the grammar fixes.
func headerFlags(g *protogen.GeneratedFile, m *message) string {
	var fl []string
	if m.cmd.GetRequest() {
		fl = append(fl, diamIdent(g, "FlagRequest"))
	}
	if m.cmd.GetProxiable() {
		fl = append(fl, diamIdent(g, "FlagProxiable"))
	}
	if len(fl) == 0 {
		return "0"
	}
	return strings.Join(fl, "|")
}

// genCommand emits the diam.Message methods of a command.
func genCommand(g *protogen.GeneratedFile, m *message) {
	g.P("// DiamHeader implements diam.Message.")
	g.P("func (m *", m.name, ") DiamHeader() *", diamIdent(g, "Header"), " { return &m.Header }")
	g.P()
	g.P("// AppendMarshal appends the encoded message to b. The command code, the")
	g.P("// application id and the R and P bits come from the grammar; the E and T")
	g.P("// bits and the identifiers come from m.Header, which is not modified.")
	g.P("func (m *", m.name, ") AppendMarshal(b []byte) ([]byte, error) {")
	g.P("h := m.Header")
	g.P("h.CommandCode, h.AppID = ", m.cmd.GetCode(), ", ", m.cmd.GetAppId())
	g.P("h.Flags = h.Flags&^(", diamIdent(g, "FlagRequest"), "|", diamIdent(g, "FlagProxiable"), ") | ", headerFlags(g, m))
	g.P("start := len(b)")
	g.P("b = ", diamIdent(g, "AppendHeader"), "(b, &h)")
	g.P("b = m.AppendAVPs(b)")
	g.P("return ", diamIdent(g, "FinishMessage"), "(b, start)")
	g.P("}")
	g.P()
	g.P("// Marshal encodes the message.")
	g.P("func (m *", m.name, ") Marshal() ([]byte, error) { return m.AppendMarshal(nil) }")
	g.P()
	g.P("// Unmarshal decodes a whole message into m, replacing its contents. It")
	g.P("// fails only when the message cannot be framed; grammar violations are")
	g.P("// reported by Validate.")
	g.P("func (m *", m.name, ") Unmarshal(b []byte) error {")
	g.P("h, avps, err := ", diamIdent(g, "ParseHeader"), "(b)")
	g.P("if err != nil {")
	g.P("return err")
	g.P("}")
	g.P("*m = ", m.name, "{Header: h}")
	g.P("return m.DecodeAVPs(avps)")
	g.P("}")
	g.P()
}

// genRegister emits the init function that makes the commands of a file
// decodable by diam.Decode.
func genRegister(g *protogen.GeneratedFile, msgs []*message) {
	var cmds []*message
	for _, m := range msgs {
		if m.cmd != nil {
			cmds = append(cmds, m)
		}
	}
	if len(cmds) == 0 {
		return
	}
	g.P("func init() {")
	for _, m := range cmds {
		g.P(diamIdent(g, "Register"), "(", m.cmd.GetAppId(), ", ", m.cmd.GetCode(), ", ", m.cmd.GetRequest(),
			", func() ", diamIdent(g, "Message"), " { return new(", m.name, ") })")
	}
	g.P("}")
}
