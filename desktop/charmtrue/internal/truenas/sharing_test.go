package truenas

import (
	"context"
	"testing"
)

func TestSharingManifestComplete(t *testing.T) {
	if len(SharingMethods) != 35 {
		t.Fatalf("got %d", len(SharingMethods))
	}
	seen := map[string]bool{}
	for _, m := range SharingMethods {
		if seen[m.Name] {
			t.Fatal(m.Name)
		}
		seen[m.Name] = true
	}
	for _, n := range []string{"ftp.config", "nfs.config", "rsynctask.run", "sharing.nfs.query", "sharing.smb.setacl", "smb.update", "ssh.config"} {
		if !seen[n] {
			t.Error(n)
		}
	}
}
func TestUnknownSharingMethod(t *testing.T) {
	_, e := (SMBService{sharingCaller{&Client{}}}).Call(context.Background(), "bad", SharingCall{})
	if e == nil {
		t.Fatal("expected error")
	}
}
