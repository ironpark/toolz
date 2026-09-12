package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SetupPresetUser struct {
	Name     string `json:"name"`
	FullName string `json:"fullName"`
	Group    string `json:"group"`
	SMB      bool   `json:"smb"`
}
type SetupPreset struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Groups           []string          `json:"groups"`
	Users            []SetupPresetUser `json:"users"`
	EnableSSH        bool              `json:"enableSSH"`
	AdminUser        string            `json:"adminUser"`
	ReplaceAdminUser string            `json:"replaceAdminUser"`
}
type SetupPresetApply struct {
	Preset              SetupPreset       `json:"preset"`
	ExpectedEndpoint    string            `json:"expectedEndpoint"`
	Passwords           map[string]string `json:"passwords"`
	AdminPassword       string            `json:"adminPassword"`
	ReplacementPassword string            `json:"replacementPassword"`
}
type SetupPresetStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
type SetupPresetResult struct {
	Steps    []SetupPresetStep `json:"steps"`
	Complete bool              `json:"complete"`
}
type presetIdentity struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"group"`
	Local    bool   `json:"local"`
	Builtin  bool   `json:"builtin"`
}

// ApplySetupPreset captures one connection; it never switches servers mid-run.
func (s *TrueNASService) ApplySetupPreset(input SetupPresetApply) (SetupPresetResult, error) {
	if !s.setupMu.TryLock() {
		return SetupPresetResult{}, errors.New("이미 프리셋 적용 작업이 진행 중입니다")
	}
	defer s.setupMu.Unlock()
	s.mu.RLock()
	client, endpoint := s.client, s.endpoint
	s.mu.RUnlock()
	if client == nil || input.ExpectedEndpoint == "" || endpoint != input.ExpectedEndpoint {
		return SetupPresetResult{}, errors.New("대상 서버 연결이 변경되었습니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var replacement *adminReplacementPlan
	if input.Preset.ReplaceAdminUser != "" {
		var err error
		replacement, err = prepareAdminReplacement(ctx, client, input)
		if err != nil {
			return SetupPresetResult{}, err
		}
	}
	result, err := applySetupPreset(ctx, client, input)
	if err != nil || !result.Complete || replacement == nil {
		return result, err
	}
	return executeAdminReplacement(ctx, client, replacement, input.ReplacementPassword, result, func(ctx context.Context, username, password string) (apiCaller, func(), error) {
		verified, err := client.DialAs(ctx, username, password)
		if err != nil {
			return nil, nil, err
		}
		return verified, func() { _ = verified.Close() }, nil
	}), nil
}

func applySetupPreset(ctx context.Context, client apiCaller, input SetupPresetApply) (SetupPresetResult, error) {
	result := SetupPresetResult{Steps: []SetupPresetStep{}}
	p := input.Preset
	if len(p.Users) > 50 || len(p.Groups) > 50 {
		return result, errors.New("사용자·그룹은 각각 최대 50개까지 가능합니다")
	}
	if len(p.Users)+len(p.Groups) == 0 && !p.EnableSSH && p.AdminUser == "" && p.ReplaceAdminUser == "" {
		return result, errors.New("적용할 설정이 없습니다")
	}
	var users, groups []presetIdentity
	if err := client.Call(ctx, "user.query", []any{}, &users); err != nil {
		return result, errors.New("사전 사용자 조회 실패: 조회 권한과 연결을 확인하세요")
	}
	if err := client.Call(ctx, "group.query", []any{}, &groups); err != nil {
		return result, errors.New("사전 그룹 조회 실패: 조회 권한과 연결을 확인하세요")
	}
	userMap, groupMap := map[string]presetIdentity{}, map[string]presetIdentity{}
	for _, u := range users {
		userMap[u.Username] = u
	}
	for _, g := range groups {
		groupMap[g.Name] = g
	}
	seenGroups, seenUsers := map[string]bool{}, map[string]bool{}
	for _, name := range p.Groups {
		if name == "" || name != strings.TrimSpace(name) || seenGroups[name] {
			return result, errors.New("그룹 이름의 공백·중복을 확인하세요")
		}
		seenGroups[name] = true
		if g, ok := groupMap[name]; ok && (!g.Local || g.Builtin) {
			return result, fmt.Errorf("기본·외부 그룹은 프리셋 대상으로 사용할 수 없습니다: %s", name)
		}
	}
	for _, u := range p.Users {
		if u.Name == "" || u.Name != strings.TrimSpace(u.Name) || seenUsers[u.Name] {
			return result, errors.New("사용자 이름의 공백·중복을 확인하세요")
		}
		seenUsers[u.Name] = true
		if old, exists := userMap[u.Name]; exists {
			if !old.Local || old.Builtin {
				return result, fmt.Errorf("기본·외부 사용자는 생성 대상으로 사용할 수 없습니다: %s", u.Name)
			}
		} else if input.Passwords[u.Name] == "" {
			return result, fmt.Errorf("새 사용자 %s의 비밀번호가 필요합니다", u.Name)
		}
		g, exists := groupMap[u.Group]
		if !seenGroups[u.Group] && (!exists || !g.Local || g.Builtin) {
			return result, fmt.Errorf("사용자 %s의 일반 로컬 기본 그룹을 지정하세요", u.Name)
		}
	}
	admin, adminExists := userMap[p.AdminUser]
	if p.AdminUser != "" && (!adminExists || !admin.Local || input.AdminPassword == "") {
		return result, errors.New("비밀번호를 변경할 기존 로컬 관리자 계정과 새 비밀번호를 확인하세요")
	}
	// Only password-free summaries are returned; middleware errors can echo payloads.
	run := func(label, method string, params []any, output any) bool {
		if err := client.Call(ctx, method, params, output); err != nil {
			result.Steps = append(result.Steps, SetupPresetStep{label, "failed", "요청 실패 또는 응답 유실. 서버 상태·권한·입력 정책을 확인하세요. 이미 완료된 변경은 유지됩니다."})
			return false
		}
		result.Steps = append(result.Steps, SetupPresetStep{label, "done", "적용 완료"})
		return true
	}
	for _, name := range p.Groups {
		if _, exists := groupMap[name]; exists {
			result.Steps = append(result.Steps, SetupPresetStep{"그룹 " + name, "skipped", "이미 존재 · 기존 설정 유지"})
			continue
		}
		var id int
		if !run("그룹 "+name, "group.create", []any{map[string]any{"name": name, "smb": true}}, &id) {
			return result, nil
		}
		groupMap[name] = presetIdentity{ID: id, Name: name, Local: true}
	}
	for _, u := range p.Users {
		if _, exists := userMap[u.Name]; exists {
			result.Steps = append(result.Steps, SetupPresetStep{"사용자 " + u.Name, "skipped", "이미 존재 · 비밀번호·소속·권한 유지"})
			continue
		}
		fullName := strings.TrimSpace(u.FullName)
		if fullName == "" {
			fullName = u.Name
		}
		data := map[string]any{"username": u.Name, "full_name": fullName, "group_create": false, "group": groupMap[u.Group].ID, "groups": []int{}, "password": input.Passwords[u.Name], "smb": u.SMB, "home": "/var/empty", "home_create": false, "shell": "/usr/sbin/nologin", "ssh_password_enabled": false, "sudo_commands": []string{}, "sudo_commands_nopasswd": []string{}}
		if !run("사용자 "+u.Name, "user.create", []any{data}, nil) {
			return result, nil
		}
	}
	if p.EnableSSH {
		if !run("SSH 자동 시작", "service.update", []any{"ssh", map[string]any{"enable": true}}, nil) {
			return result, nil
		}
		var ok bool
		if !run("SSH 활성화", "service.start", []any{"ssh", map[string]any{}}, &ok) {
			return result, nil
		}
		if !ok {
			result.Steps[len(result.Steps)-1].Status = "failed"
			result.Steps[len(result.Steps)-1].Message = "서버가 SSH 시작을 완료하지 못했습니다"
			return result, nil
		}
	}
	if p.AdminUser != "" {
		if !run("관리자 비밀번호 변경: "+p.AdminUser, "user.update", []any{admin.ID, map[string]any{"password": input.AdminPassword}}, nil) {
			return result, nil
		}
	}
	result.Complete = true
	return result, nil
}
