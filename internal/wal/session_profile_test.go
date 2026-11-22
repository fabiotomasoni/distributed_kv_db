package wal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionIdentityProfilesNeverConvertOrRepairEachOther(t *testing.T) {
	for _, sessions := range []bool{false, true} {
		root := t.TempDir()
		if e := os.Chmod(root, 0700); e != nil {
			t.Fatal(e)
		}
		var s *Store
		var e error
		if sessions {
			s, e = BootstrapSessions(root, testIdentity)
		} else {
			s, e = Bootstrap(root, testIdentity)
		}
		if e != nil {
			t.Fatal(e)
		}
		_ = s.Close()
		identity, _ := os.ReadFile(filepath.Join(root, "IDENTITY"))
		walPath := filepath.Join(root, "gen-1", "wal")
		original, _ := os.ReadFile(walPath)
		// A torn tail must not be repaired when the application profile mismatches.
		image := append(bytes.Clone(original), []byte("QW")...)
		if e = os.WriteFile(walPath, image, 0600); e != nil {
			t.Fatal(e)
		}
		if sessions {
			s, e = Open(root, testIdentity)
		} else {
			s, e = OpenSessions(root, testIdentity)
		}
		if e == nil || s != nil {
			t.Fatal("profile silently converted")
		}
		after, _ := os.ReadFile(walPath)
		idAfter, _ := os.ReadFile(filepath.Join(root, "IDENTITY"))
		if !bytes.Equal(after, image) || !bytes.Equal(identity, idAfter) {
			t.Fatal("wrong-profile open changed data")
		}
		if sessions {
			s, e = OpenSessions(root, testIdentity)
		} else {
			s, e = Open(root, testIdentity)
		}
		if e != nil {
			t.Fatal(e)
		}
		want := 1
		if sessions {
			want = 2
		}
		if s.ApplicationVersion() != want {
			t.Fatal("profile")
		}
		_ = s.Close()
	}
}
