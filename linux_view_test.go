package isobox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// linuxRuntimePathCandidates feeds appendLinuxRuntimeMounts; missing paths are
// dropped by the os.Lstat filter there, so the candidate list itself only has
// to enumerate the union of paths needed across distros and architectures.
func TestLinuxRuntimePathCandidatesCoversDynamicELFClosure(t *testing.T) {
	got := linuxRuntimePathCandidates(Spec{Args: []string{"/bin/ls"}})

	required := []string{
		"/etc/ld.so.cache",
		"/usr/lib",
	}
	for _, p := range required {
		if !containsString(got, p) {
			t.Errorf("linuxRuntimePathCandidates missing %q: %v", p, got)
		}
	}

	// At least one of /lib or /lib64 must be present so the dynamic linker
	// (/lib64/ld-linux-x86-64.so.2 on x86_64 Debian/Fedora, /lib/ld-linux-* on
	// some embedded layouts) is exposed.
	if !containsString(got, "/lib") && !containsString(got, "/lib64") {
		t.Errorf("linuxRuntimePathCandidates must include /lib or /lib64 so the dynamic linker is mounted: %v", got)
	}

	// The executable itself must be in the list — otherwise the OCI bundle
	// would not have an image to exec under the scoped rootfs.
	if !containsString(got, "/bin/ls") {
		t.Errorf("linuxRuntimePathCandidates missing the executable path /bin/ls: %v", got)
	}
}

func TestLinuxRuntimePathCandidatesIncludesMultiarchAndLdConfig(t *testing.T) {
	// Multiarch dirs (Debian/Ubuntu) and ld.so.conf.d must be candidates so
	// glibc can locate libraries under /lib/<triple>-linux-gnu via ld.so.cache.
	got := linuxRuntimePathCandidates(Spec{})

	for _, p := range []string{
		"/etc/ld.so.cache",
		"/etc/ld.so.conf",
		"/etc/ld.so.conf.d",
	} {
		if !containsString(got, p) {
			t.Errorf("linuxRuntimePathCandidates missing %q: %v", p, got)
		}
	}

	multiarch := []string{
		"/lib/x86_64-linux-gnu",
		"/usr/lib/x86_64-linux-gnu",
		"/lib/aarch64-linux-gnu",
		"/usr/lib/aarch64-linux-gnu",
	}
	for _, p := range multiarch {
		if !containsString(got, p) {
			t.Errorf("linuxRuntimePathCandidates missing multiarch dir %q: %v", p, got)
		}
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestLinkLinuxViewTopLevelSymlinks(t *testing.T) {
	host := t.TempDir()
	if err := os.MkdirAll(filepath.Join(host, "usr", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bin", "sbin", "lib"} {
		if err := os.Symlink("usr/"+name, filepath.Join(host, name)); err != nil {
			t.Fatal(err)
		}
	}
	old := linuxTopLevelSymlinkCandidates
	t.Cleanup(func() { linuxTopLevelSymlinkCandidates = old })
	linuxTopLevelSymlinkCandidates = []string{filepath.Join(host, "bin"), filepath.Join(host, "sbin"), filepath.Join(host, "lib")}

	usr, _ := filepath.EvalSymlinks(filepath.Join(host, "usr"))
	rootfs := t.TempDir()
	parent := filepath.Join(rootfs, strings.TrimPrefix(host, "/"))
	if err := os.MkdirAll(filepath.Join(parent, "lib"), 0o755); err != nil { // already in the view
		t.Fatal(err)
	}
	mounts := []linuxViewMount{{Source: filepath.Join(usr, "bin"), Target: filepath.Join(usr, "bin"), Mode: linuxViewMountReadOnly}}
	if err := linkLinuxViewTopLevelSymlinks(rootfs, mounts); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(filepath.Join(parent, "bin")); err != nil || got != "usr/bin" {
		t.Errorf("bin: %q %v, want a usr/bin link (its target is mounted)", got, err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "sbin")); err == nil {
		t.Error("sbin linked although usr/sbin is not in the view")
	}
	if fi, err := os.Lstat(filepath.Join(parent, "lib")); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("lib: an existing view path must be kept: %v %v", fi, err)
	}
}
