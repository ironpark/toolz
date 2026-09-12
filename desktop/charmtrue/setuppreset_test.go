package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type presetCaller struct {
	users, groups []presetIdentity
	methods       []string
	params        [][]any
	fail          string
	sshOK         bool
}

func (c *presetCaller) Call(_ context.Context, method string, params []any, result any) error {
	c.methods = append(c.methods, method)
	c.params = append(c.params, params)
	if method == c.fail {
		return errors.New("secret-password echoed by server")
	}
	var value any
	switch method {
	case "user.query":
		value = c.users
	case "group.query":
		value = c.groups
	case "group.create":
		value = 42
	case "service.start":
		value = c.sshOK
	default:
		return nil
	}
	if result != nil {
		data, _ := json.Marshal(value)
		return json.Unmarshal(data, result)
	}
	return nil
}
func presetInput() SetupPresetApply {
	return SetupPresetApply{Preset: SetupPreset{Name: "Initial", Groups: []string{"family"}, Users: []SetupPresetUser{{Name: "alex", Group: "family", SMB: true}}, EnableSSH: true, AdminUser: "admin"}, Passwords: map[string]string{"alex": "secret-password"}, AdminPassword: "secret-password"}
}
func TestPresetApplyOrderAndNarrowPayload(t *testing.T) {
	c := &presetCaller{users: []presetIdentity{{ID: 1, Username: "admin", Local: true}}, sshOK: true}
	result, err := applySetupPreset(context.Background(), c, presetInput())
	if err != nil || !result.Complete {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	want := []string{"user.query", "group.query", "group.create", "user.create", "service.update", "service.start", "user.update"}
	if !reflect.DeepEqual(c.methods, want) {
		t.Fatalf("methods: %v", c.methods)
	}
	user := c.params[3][0].(map[string]any)
	if user["group"] != 42 || user["shell"] != "/usr/sbin/nologin" || user["ssh_password_enabled"] != false {
		t.Fatalf("unsafe user defaults")
	}
	if !reflect.DeepEqual(c.params[6], []any{1, map[string]any{"password": "secret-password"}}) {
		t.Fatal("admin update must only set password")
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), "secret-password") {
		t.Fatal("secret in result")
	}
}
func TestPresetPreflightBeforeAnyWrite(t *testing.T) {
	input := presetInput()
	delete(input.Passwords, "alex")
	c := &presetCaller{users: []presetIdentity{{ID: 1, Username: "admin", Local: true}}}
	if _, err := applySetupPreset(context.Background(), c, input); err == nil {
		t.Fatal("missing password accepted")
	}
	if len(c.methods) != 2 {
		t.Fatal("write before validation")
	}
}

func TestPresetGroupQueryWireField(t *testing.T) {
	var g presetIdentity
	if err := json.Unmarshal([]byte(`{"id":42,"group":"family","local":true,"builtin":false}`), &g); err != nil || g.Name != "family" {
		t.Fatalf("group query decode failed: %+v %v", g, err)
	}
}
func TestPresetExistingIdentitiesPreserved(t *testing.T) {
	input := presetInput()
	input.Preset.EnableSSH = false
	input.Preset.AdminUser = ""
	input.Passwords = nil
	c := &presetCaller{users: []presetIdentity{{ID: 2, Username: "alex", Local: true}}, groups: []presetIdentity{{ID: 3, Name: "family", Local: true}}}
	result, err := applySetupPreset(context.Background(), c, input)
	if err != nil || !result.Complete || len(c.methods) != 2 {
		t.Fatalf("existing identities changed: %+v %v", result, err)
	}
	for _, step := range result.Steps {
		if step.Status != "skipped" {
			t.Fatal("expected skip")
		}
	}
}
func TestPresetStopsOnFailureAndRedactsAPIError(t *testing.T) {
	c := &presetCaller{users: []presetIdentity{{ID: 1, Username: "admin", Local: true}}, fail: "user.create"}
	result, err := applySetupPreset(context.Background(), c, presetInput())
	if err != nil || result.Complete || len(c.methods) != 4 || result.Steps[0].Status != "done" || result.Steps[1].Status != "failed" {
		t.Fatalf("bad partial result: %+v %v", result, err)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), "secret-password") {
		t.Fatal("API error leaked password")
	}
}
func TestPresetFalseSSHResultStopsBeforePasswordChange(t *testing.T) {
	c := &presetCaller{users: []presetIdentity{{ID: 1, Username: "admin", Local: true}}}
	result, err := applySetupPreset(context.Background(), c, presetInput())
	if err != nil || result.Complete || c.methods[len(c.methods)-1] != "service.start" {
		t.Fatalf("did not stop: %+v %v", result, err)
	}
}
func TestPresetRejectsDisconnectedAndConcurrentApply(t *testing.T) {
	s := &TrueNASService{}
	if _, err := s.ApplySetupPreset(presetInput()); err == nil {
		t.Fatal("disconnected apply accepted")
	}
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if _, err := s.ApplySetupPreset(presetInput()); err == nil {
		t.Fatal("concurrent apply accepted")
	}
}
