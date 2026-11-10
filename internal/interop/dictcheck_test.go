package interop

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fiorix/go-diameter/v4/diam/dict"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/hsdfat/go-diam/cmd/protoc-gen-go-diam/diampb"
)

// fiorixTypes maps go-diameter data type names to diam.Type. Derived types
// map to the base type the RFCs give them.
var fiorixTypes = map[string]diampb.Type{
	"OctetString":      diampb.Type_OCTET_STRING,
	"Integer32":        diampb.Type_INTEGER32,
	"Integer64":        diampb.Type_INTEGER64,
	"Unsigned32":       diampb.Type_UNSIGNED32,
	"Unsigned64":       diampb.Type_UNSIGNED64,
	"Float32":          diampb.Type_FLOAT32,
	"Float64":          diampb.Type_FLOAT64,
	"Grouped":          diampb.Type_GROUPED,
	"Address":          diampb.Type_ADDRESS,
	"Time":             diampb.Type_TIME,
	"UTF8String":       diampb.Type_UTF8_STRING,
	"DiameterIdentity": diampb.Type_DIAMETER_IDENTITY,
	"DiameterURI":      diampb.Type_DIAMETER_URI,
	"Enumerated":       diampb.Type_ENUMERATED,
	"IPFilterRule":     diampb.Type_OCTET_STRING,
	"QoSFilterRule":    diampb.Type_OCTET_STRING,
	"IPv4":             diampb.Type_OCTET_STRING,
	"IPv6":             diampb.Type_OCTET_STRING,
}

// schemaImage compiles the repository's proto files with buf.
func schemaImage(t *testing.T) *descriptorpb.FileDescriptorSet {
	t.Helper()
	buf, err := exec.LookPath("buf")
	if err != nil {
		t.Fatal("buf is not on PATH: the dictionary cross-check compiles the proto files with it (install buf, see the Makefile)")
	}
	out := filepath.Join(t.TempDir(), "image.binpb")
	cmd := exec.Command(buf, "build", "-o", out)
	cmd.Dir = filepath.Join("..", "..")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("buf build: %v\n%s", err, msg)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &set); err != nil {
		t.Fatal(err)
	}
	return &set
}

// allowList reads the reviewed disagreements, keyed "<code> <vendor> <field>".
func allowList(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "dictcheck-allow.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 4 {
			t.Fatalf("dictcheck-allow.txt:%d: want <code> <vendor> <field> <spec clause>, got %q", n, line)
		}
		out[strings.Join(parts[:3], " ")] = strings.Join(parts[3:], " ")
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// dictName turns a specification AVP name into the form dictionary entries
// use: "Origin-State-Id" -> "ORIGIN_STATE_ID".
func dictName(name string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", " ", "_").Replace(name))
}

// fiorixAVP finds an AVP by exact code and vendor in any application of
// go-diameter's default dictionary.
func fiorixAVP(code, vendor uint32) *dict.AVP {
	if a, err := dict.Default.FindAVPByCode(0, code, vendor); err == nil {
		return a
	}
	for _, app := range dict.Default.Apps() {
		if a, err := dict.Default.FindAVPByCode(app.ID, code, vendor); err == nil {
			return a
		}
	}
	return nil
}

// TestDictionaryAgreesWithGoDiameter compares every dictionary entry of the
// schema with go-diameter's dictionary: vendor, M-bit and data type. It is
// the guard against an AVP code, vendor or flag that is wrong but still
// passes go-diam's own round-trip tests.
func TestDictionaryAgreesWithGoDiameter(t *testing.T) {
	allow := allowList(t)
	used := map[string]bool{}
	checked, absent := 0, 0

	for _, file := range schemaImage(t).File {
		if strings.HasPrefix(file.GetPackage(), "gentest") {
			continue // generator fixtures under a private vendor id
		}
		for _, enum := range file.EnumType {
			if !proto.GetExtension(enum.GetOptions(), diampb.E_Dictionary).(bool) {
				continue
			}
			for _, v := range enum.Value {
				if !proto.HasExtension(v.GetOptions(), diampb.E_Def) {
					continue
				}
				def := proto.GetExtension(v.GetOptions(), diampb.E_Def).(*diampb.AVPDef)
				code, vendor := uint32(v.GetNumber()), def.GetVendor()
				theirs := fiorixAVP(code, vendor)
				if theirs == nil {
					absent++
					continue
				}
				checked++
				report := func(field, msg string) {
					key := fmt.Sprintf("%d %d %s", code, vendor, field)
					if _, ok := allow[key]; ok {
						used[key] = true
						return
					}
					t.Errorf("%s %s (code %d vendor %d): %s", file.GetName(), v.GetName(), code, vendor, msg)
				}
				// Looked up by code, so the name is the independent check of
				// the code itself: a wrong code lands on another AVP.
				if got := dictName(theirs.Name); got != v.GetName() {
					report("name", fmt.Sprintf("go-diameter calls this code %s (%s)", theirs.Name, got))
				}
				if theirs.VendorID != vendor {
					report("vendor", fmt.Sprintf("vendor %d, go-diameter has %d", vendor, theirs.VendorID))
				}
				if theirM := strings.Contains(theirs.Must, "M"); theirM != def.GetM() {
					report("m", fmt.Sprintf("m=%v, go-diameter has must=%q", def.GetM(), theirs.Must))
				}
				want, known := fiorixTypes[theirs.Data.TypeName]
				switch {
				case !known:
					report("type", fmt.Sprintf("go-diameter type %q is not mapped", theirs.Data.TypeName))
				case want != def.GetType():
					report("type", fmt.Sprintf("type %v, go-diameter has %s", def.GetType(), theirs.Data.TypeName))
				}
			}
		}
	}

	for key, clause := range allow {
		if !used[key] {
			t.Errorf("dictcheck-allow.txt: %q (%s) no longer matches a disagreement; remove it", key, clause)
		}
	}
	if checked == 0 {
		t.Fatal("no dictionary entry was compared")
	}
	t.Logf("compared %d dictionary entries; %d are not in go-diameter's dictionary", checked, absent)
}
