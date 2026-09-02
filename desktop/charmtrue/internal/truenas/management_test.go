package truenas

import "testing"

func TestManagementMethodManifests(t *testing.T) {
	if got := len(SystemMethods); got != 102 {
		t.Fatalf("system methods = %d, want 102", got)
	}
	if got := len(IdentityMethods); got != 49 {
		t.Fatalf("identity methods = %d, want 49", got)
	}
	seen := map[string]bool{}
	for _, m := range SystemMethods {
		if seen[m.Name] {
			t.Fatalf("duplicate method %q", m.Name)
		}
		seen[m.Name] = true
	}
	for _, m := range IdentityMethods {
		if seen[m.Name] {
			t.Fatalf("duplicate method %q", m.Name)
		}
		seen[m.Name] = true
	}
	for _, name := range []string{"system.info", "service.restart", "update.run", "user.query", "group.create", "api_key.delete", "auth.me"} {
		if !seen[name] {
			t.Errorf("missing method %q", name)
		}
	}
	if _, ok := SystemMethodByName("system.reboot"); ok {
		t.Error("namespace-only system.reboot must not be callable wrapper")
	}
}

func TestManagementDestructiveMetadata(t *testing.T) {
	for _, name := range []string{"config.reset", "system.shutdown", "user.delete", "group.delete", "api_key.delete"} {
		var destructive bool
		if m, ok := SystemMethodByName(name); ok {
			destructive = m.Destructive
		} else if m, ok := IdentityMethodByName(name); ok {
			destructive = m.Destructive
		}
		if !destructive {
			t.Errorf("%s should be destructive", name)
		}
	}
}
