package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/toolz/desktop/charmtrue/internal/truenas"
)

type replacementCaller struct {
	methods  []string
	params   [][]any
	verified bool
	noRole   bool
	fail     string
}

func (c *replacementCaller) Call(_ context.Context, method string, params []any, output any) error {
	c.methods = append(c.methods, method)
	c.params = append(c.params, params)
	if method == c.fail {
		return errors.New("secret-password")
	}
	var value any
	switch method {
	case "user.query":
		if c.verified {
			value = []any{map[string]any{"id": 5, "username": "truenas_admin", "locked": true, "password_disabled": true}}
		} else {
			value = []any{map[string]any{"id": 5, "username": "truenas_admin", "local": true, "group": map[string]any{"id": 7, "gid": 950}, "groups": []int{8}, "shell": "/usr/bin/zsh", "sudo_commands_nopasswd": []string{"ALL"}}}
		}
	case "privilege.query":
		value = []any{map[string]any{"builtin_name": "LOCAL_ADMINISTRATOR", "roles": []string{"FULL_ADMIN"}, "local_groups": []any{map[string]any{"id": 9, "gid": 544}}}}
	case "auth.me":
		roles := []string{"FULL_ADMIN"}
		if c.noRole {
			roles = []string{"READONLY_ADMIN"}
		}
		value = map[string]any{"username": "new_admin", "local": true, "privilege": map[string]any{"roles": roles}}
	case "api_key.query":
		value = []any{map[string]any{"id": 50, "username": "truenas_admin"}, map[string]any{"id": 60, "username": "other"}}
	default:
		return nil
	}
	if output != nil {
		data, _ := json.Marshal(value)
		return json.Unmarshal(data, output)
	}
	return nil
}
func replacementPlan() *adminReplacementPlan {
	return &adminReplacementPlan{Name: "new_admin", Source: truenas.UserEntry{ID: 5, Username: "truenas_admin", Local: true, Group: truenas.UserPrimaryGroup{ID: 7}, Shell: "/usr/bin/zsh", SudoCommandsNoPassword: []string{"ALL"}}, Groups: []int{8, 9}}
}
func TestReplacementPreflightAddsVerifiedAdminGroupByDatabaseID(t *testing.T) {
	c := &replacementCaller{}
	p, err := prepareAdminReplacement(context.Background(), c, SetupPresetApply{Preset: SetupPreset{ReplaceAdminUser: "new_admin"}, ReplacementPassword: "secret-password"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Groups, []int{8, 9}) || p.Source.Group.ID != 7 {
		t.Fatalf("group mapping wrong: %+v", p)
	}
	if slices.Contains(c.methods, "user.create") {
		t.Fatal("preflight wrote to server")
	}
}
func TestReplacementLoginFailureNeverDisablesSource(t *testing.T) {
	c := &replacementCaller{}
	r := executeAdminReplacement(context.Background(), c, replacementPlan(), "secret-password", SetupPresetResult{}, func(context.Context, string, string) (apiCaller, func(), error) {
		return nil, nil, errors.New("secret-password")
	})
	if r.Complete || !reflect.DeepEqual(c.methods, []string{"user.create"}) {
		t.Fatalf("unsafe failure: %+v %v", r, c.methods)
	}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "secret-password") {
		t.Fatal("secret leaked")
	}
}
func TestReplacementReadonlySessionNeverDisablesSource(t *testing.T) {
	old, fresh := &replacementCaller{}, &replacementCaller{verified: true, noRole: true}
	closed := false
	r := executeAdminReplacement(context.Background(), old, replacementPlan(), "pw", SetupPresetResult{}, func(context.Context, string, string) (apiCaller, func(), error) {
		return fresh, func() { closed = true }, nil
	})
	if r.Complete || !closed || slices.Contains(fresh.methods, "user.update") {
		t.Fatal("unverified admin disabled original")
	}
}
func TestReplacementDisablesThroughNewSessionAndDeletesOnlyOldKeys(t *testing.T) {
	old, fresh := &replacementCaller{}, &replacementCaller{verified: true}
	r := executeAdminReplacement(context.Background(), old, replacementPlan(), "pw", SetupPresetResult{}, func(_ context.Context, name, password string) (apiCaller, func(), error) {
		if name != "new_admin" || password != "pw" {
			t.Fatal("wrong new credentials")
		}
		return fresh, func() {}, nil
	})
	if !r.Complete {
		t.Fatalf("not complete: %+v", r)
	}
	if !reflect.DeepEqual(old.methods, []string{"user.create"}) {
		t.Fatal("disable used old session")
	}
	want := []string{"auth.me", "privilege.query", "api_key.query", "user.update", "user.query", "api_key.delete"}
	if !reflect.DeepEqual(fresh.methods, want) {
		t.Fatalf("wrong order: %v", fresh.methods)
	}
	if !reflect.DeepEqual(fresh.params[5], []any{50}) {
		t.Fatal("unrelated API key deleted")
	}
	create := old.params[0][0].(map[string]any)
	if create["group"] != 7 || create["password"] != "pw" || create["shell"] != "/usr/bin/zsh" {
		t.Fatal("wrong cloning payload")
	}
	if _, exists := create["sshpubkey"]; exists {
		t.Fatal("personal SSH key copied")
	}
}
func TestReplacementAPIReadFailureKeepsOriginal(t *testing.T) {
	old, fresh := &replacementCaller{}, &replacementCaller{verified: true, fail: "privilege.query"}
	r := executeAdminReplacement(context.Background(), old, replacementPlan(), "pw", SetupPresetResult{}, func(context.Context, string, string) (apiCaller, func(), error) { return fresh, func() {}, nil })
	if r.Complete || slices.Contains(fresh.methods, "user.update") {
		t.Fatal("API verification failure disabled original")
	}
}
