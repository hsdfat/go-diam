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

// sessionField returns the field declared as session_id, if any.
func sessionField(m *message) *field {
	for _, fd := range m.fields {
		if fd.f.Desc.Name() == "session_id" {
			return fd
		}
	}
	return nil
}

// genAnswerConstructors emits New<Answer>For for every request that has an
// answer with the same command code and application id in the same file.
func genAnswerConstructors(g *protogen.GeneratedFile, msgs []*message) {
	for _, req := range msgs {
		if req.cmd == nil || !req.cmd.GetRequest() {
			continue
		}
		for _, ans := range msgs {
			if ans.cmd == nil || ans.cmd.GetRequest() ||
				ans.cmd.GetCode() != req.cmd.GetCode() || ans.cmd.GetAppId() != req.cmd.GetAppId() {
				continue
			}
			copySession := sessionField(req) != nil && sessionField(ans) != nil
			g.P("// New", ans.name, "For returns the answer to req: the request's header")
			g.P("// identifiers with the R and T bits cleared", map[bool]string{true: ", and its Session-Id", false: ""}[copySession], ".")
			g.P("// Every other AVP is left for the caller to set.")
			g.P("func New", ans.name, "For(req *", req.name, ") *", ans.name, " {")
			g.P("ans := &", ans.name, "{Header: ", diamIdent(g, "AnswerHeader"), "(&req.Header)}")
			if copySession {
				g.P("ans.", sessionField(ans).f.GoName, " = req.", sessionField(req).f.GoName)
			}
			g.P("return ans")
			g.P("}")
			g.P()
		}
	}
}
