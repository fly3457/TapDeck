// winresources builds the manifest and file-version resources used by the EXEs.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	release := flag.String("version", "", "release version from Android build metadata")
	flag.Parse()
	if err := generate(*release); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(release string) error {
	parts := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:[-+][0-9A-Za-z.-]+)?$`).FindStringSubmatch(release)
	if parts == nil {
		return fmt.Errorf("invalid release version %q", release)
	}
	var fixed [4]uint16
	for i := 0; i < 3; i++ {
		component, err := strconv.ParseUint(parts[i+1], 10, 16)
		if err != nil {
			return fmt.Errorf("Windows version component: %w", err)
		}
		fixed[i] = uint16(component)
	}
	manifest, err := os.ReadFile("cmd/tapdeck/app.manifest")
	if err != nil {
		return err
	}
	for _, target := range []struct{ path, description string }{
		{"cmd/tapdeck/rsrc.syso", "TapDeck Windows Receiver"},
		{"cmd/hidprobe/rsrc.syso", "TapDeck HID Diagnostic Tool"},
	} {
		rs := winres.ResourceSet{}
		// Preserve the existing Common Controls, DPI and execution-level manifest.
		if err := rs.Set(winres.RT_MANIFEST, winres.ID(1), winres.LCIDDefault, manifest); err != nil {
			return err
		}
		info := version.Info{FileVersion: fixed, ProductVersion: fixed, Type: version.App}
		for key, value := range map[string]string{
			version.FileVersion: release, version.ProductVersion: release,
			version.ProductName: "TapDeck", version.FileDescription: target.description,
		} {
			if err := info.Set(version.LangDefault, key, value); err != nil {
				return err
			}
		}
		rs.SetVersionInfo(info)
		out, err := os.Create(target.path)
		if err != nil {
			return err
		}
		writeErr := rs.WriteObject(out, winres.ArchAMD64)
		closeErr := out.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
