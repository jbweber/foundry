package libvirt_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jbweber/foundry/internal/libvirt"
	"github.com/jbweber/foundry/internal/loader"
)

// updateGolden rewrites the expected domain XML: go test ./internal/libvirt -run GoldenDomain -update
var updateGolden = flag.Bool("update", false, "rewrite the golden domain XML files")

// goldenDir holds one VM file per case (<case>.yaml) and the domain XML
// foundry must render for it (<case>.xml). The hypervisor's architecture is
// the spec's arch, or the one a first line "# host-arch: <arch>" names, so a
// case renders the same on any machine: an aarch64 case is checked on an
// x86_64 CI runner, with no Arm hardware and no libvirt.
const goldenDir = "testdata/domains"

// renderCase renders a case's VM file for its hypervisor architecture.
func renderCase(t *testing.T, name string) (file, xml string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(goldenDir, name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	vm, err := loader.LoadFromYAML(data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	hostArch := vm.GetArch()
	if first, _, _ := strings.Cut(string(data), "\n"); strings.HasPrefix(first, "# host-arch: ") {
		hostArch = strings.TrimSpace(strings.TrimPrefix(first, "# host-arch: "))
	}
	xml, err = libvirt.GenerateDomainXML(vm, hostArch)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return name + ".xml", xml
}

// goldenCases returns the cases: the VM files in goldenDir, by name.
func goldenCases(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(goldenDir, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no cases in %s", goldenDir)
	}
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".yaml"))
	}
	sort.Strings(names)
	return names
}

// compareGolden returns an error naming the first line where got and want
// differ, or nil when they are the same.
func compareGolden(file, got, want string) error {
	if got == want {
		return nil
	}
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			return fmt.Errorf("%s differs at line %d:\n  got:  %q\n  want: %q\n(if the change is meant, run with -update and review the diff)", file, i+1, gl, wl)
		}
	}
	return fmt.Errorf("%s differs", file)
}

// TestGoldenDomainXML renders every case and compares it byte for byte with
// its expected XML, so any change to what foundry asks libvirt for, on either
// architecture, fails here until the expected file is rewritten and the change
// is reviewed in the diff.
func TestGoldenDomainXML(t *testing.T) {
	expected := map[string]bool{}
	for _, name := range goldenCases(t) {
		t.Run(name, func(t *testing.T) {
			file, got := renderCase(t, name)
			expected[file] = true
			path := filepath.Join(goldenDir, file)
			if *updateGolden {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run with -update to write it)", err)
			}
			if err := compareGolden(file, got, string(want)); err != nil {
				t.Error(err)
			}
		})
	}
	// An expected file whose case is gone would be checked by nothing.
	xmls, err := filepath.Glob(filepath.Join(goldenDir, "*.xml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range xmls {
		if !expected[filepath.Base(f)] {
			t.Errorf("%s has no case that renders it: delete it", f)
		}
	}
}

// TestGoldenDomainXMLCanFail shows the comparison is not vacuous: each case's
// XML with one attribute changed, and with a line dropped, is reported as
// different.
func TestGoldenDomainXMLCanFail(t *testing.T) {
	for _, name := range goldenCases(t) {
		file, got := renderCase(t, name)
		if err := compareGolden(file, got, got); err != nil {
			t.Fatalf("%s: equal XML reported as different: %v", file, err)
		}
		changed := strings.Replace(got, "type=", "type-changed=", 1)
		if changed == got {
			t.Fatalf("%s: nothing to change", file)
		}
		if compareGolden(file, changed, got) == nil {
			t.Errorf("%s: a changed attribute was not noticed", file)
		}
		lines := strings.Split(got, "\n")
		dropped := strings.Join(append(lines[:len(lines)/2:len(lines)/2], lines[len(lines)/2+1:]...), "\n")
		if compareGolden(file, dropped, got) == nil {
			t.Errorf("%s: a dropped line was not noticed", file)
		}
	}
}
