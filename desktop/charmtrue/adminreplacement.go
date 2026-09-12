package main

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/ironpark/toolz/desktop/charmtrue/internal/truenas"
)

type adminReplacementPlan struct {
	Name   string
	Source truenas.UserEntry
	Groups []int
}

func prepareAdminReplacement(ctx context.Context, client apiCaller, input SetupPresetApply) (*adminReplacementPlan, error) {
	name := input.Preset.ReplaceAdminUser
	if name == "" || name != strings.TrimSpace(name) || name == "root" || name == "truenas_admin" || input.ReplacementPassword == "" {
		return nil, errors.New("새 관리자 이름과 비밀번호를 지정하세요. root·truenas_admin은 사용할 수 없습니다")
	}
	if input.Preset.AdminUser != "" {
		return nil, errors.New("관리자 교체와 기존 관리자 비밀번호 변경은 동시에 적용할 수 없습니다")
	}
	for _, u := range input.Preset.Users {
		if u.Name == name {
			return nil, errors.New("새 관리자는 일반 사용자 생성 목록에서 제외하세요")
		}
	}
	var users []truenas.UserEntry
	if err := client.Call(ctx, "user.query", []any{}, &users); err != nil {
		return nil, errors.New("관리자 교체 사전 사용자 조회 실패")
	}
	var source *truenas.UserEntry
	for _, u := range users {
		if u.Username == name {
			return nil, errors.New("새 관리자 이름이 이미 존재합니다. 기존 계정을 덮어쓰지 않습니다")
		}
		if u.Username == "truenas_admin" {
			copy := u
			source = &copy
		}
	}
	if source == nil || !source.Local || source.Locked || source.PasswordDisabled || source.Group.ID < 1 {
		return nil, errors.New("활성 로컬 truenas_admin 계정을 확인할 수 없습니다. 기존 관리자는 변경하지 않습니다")
	}
	var privileges []struct {
		BuiltinName string   `json:"builtin_name"`
		Roles       []string `json:"roles"`
		LocalGroups []struct {
			ID  int `json:"id"`
			GID int `json:"gid"`
		} `json:"local_groups"`
	}
	if err := client.Call(ctx, "privilege.query", []any{}, &privileges); err != nil {
		return nil, errors.New("관리자 권한 조회 실패")
	}
	groups := append([]int{}, source.Groups...)
	adminGroup := 0
	for _, p := range privileges {
		if p.BuiltinName != "LOCAL_ADMINISTRATOR" || !slices.Contains(p.Roles, "FULL_ADMIN") {
			continue
		}
		for _, g := range p.LocalGroups {
			if g.GID == 544 && g.ID > 0 {
				adminGroup = g.ID
			}
		}
	}
	if adminGroup == 0 {
		return nil, errors.New("FULL_ADMIN을 부여하는 기본 관리자 그룹을 검증하지 못했습니다")
	}
	if adminGroup != source.Group.ID && !slices.Contains(groups, adminGroup) {
		groups = append(groups, adminGroup)
	}
	groups = cleanGroupIDs(groups, source.Group.ID)
	return &adminReplacementPlan{Name: name, Source: *source, Groups: groups}, nil
}

type adminSessionDial func(context.Context, string, string) (apiCaller, func(), error)

func executeAdminReplacement(ctx context.Context, client apiCaller, plan *adminReplacementPlan, password string, result SetupPresetResult, dial adminSessionDial) SetupPresetResult {
	result.Complete = false
	fail := func(name, message string) SetupPresetResult {
		result.Steps = append(result.Steps, SetupPresetStep{name, "failed", message})
		return result
	}
	done := func(name string) { result.Steps = append(result.Steps, SetupPresetStep{name, "done", "적용 완료"}) }
	source := plan.Source
	data := map[string]any{"username": plan.Name, "full_name": plan.Name, "password": password, "group_create": false,
		"group": source.Group.ID, "groups": plan.Groups, "smb": source.SMB, "shell": source.Shell,
		"home": "/var/empty", "home_create": false, "locked": false, "password_disabled": false,
		"ssh_password_enabled": source.SSHPasswordEnabled, "sudo_commands": cleanStringList(source.SudoCommands), "sudo_commands_nopasswd": cleanStringList(source.SudoCommandsNoPassword)}
	if err := client.Call(ctx, "user.create", []any{data}, nil); err != nil {
		return fail("새 관리자 생성", "생성 요청 실패 또는 응답 유실. 기존 관리자는 유지합니다. 새 계정 존재 여부를 확인하세요.")
	}
	done("새 관리자 생성 및 그룹·sudo 권한 복제: " + plan.Name)
	verified, closeSession, err := dial(ctx, plan.Name, password)
	if err != nil {
		return fail("새 관리자 로그인 검증", "로그인을 검증하지 못했습니다. 새 계정은 남아 있고 truenas_admin은 유지됩니다. 2단계 인증·로그인 정책을 확인하세요.")
	}
	defer closeSession()
	var me struct {
		Username  string `json:"username"`
		Local     bool   `json:"local"`
		Privilege struct {
			Roles []string `json:"roles"`
		} `json:"privilege"`
	}
	if err := verified.Call(ctx, "auth.me", []any{}, &me); err != nil || me.Username != plan.Name || !me.Local || !slices.Contains(me.Privilege.Roles, "FULL_ADMIN") {
		return fail("새 관리자 권한 검증", "FULL_ADMIN 세션을 검증하지 못했습니다. truenas_admin은 유지됩니다.")
	}
	// Exercise a privileged read using the new session before any disable action.
	if err := verified.Call(ctx, "privilege.query", []any{}, nil); err != nil {
		return fail("관리자 API 접근 검증", "새 계정의 관리자 API 접근에 실패했습니다. truenas_admin은 유지됩니다.")
	}
	done("새 계정 로그인 · FULL_ADMIN · 관리자 API 검증")
	var keys []struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
	}
	if err := verified.Call(ctx, "api_key.query", []any{}, &keys); err != nil {
		return fail("기존 관리자 API 키 조회", "기존 API 키를 확인하지 못했습니다. truenas_admin은 유지됩니다.")
	}
	// Disable through the verified new session, so invalidating the original
	// account cannot cut off the remaining operations.
	if err := verified.Call(ctx, "user.update", []any{source.ID, map[string]any{"locked": true, "password_disabled": true, "ssh_password_enabled": false, "sshpubkey": "", "smb": false}}, nil); err != nil {
		return fail("truenas_admin 비활성화", "비활성화 요청 실패 또는 응답 유실. 새 관리자로 접속해 기존 계정의 잠금 상태를 확인하세요.")
	}
	var current []truenas.UserEntry
	if err := verified.Call(ctx, "user.query", []any{[]any{[]any{"id", "=", source.ID}}}, &current); err != nil || len(current) != 1 || current[0].ID != source.ID || !current[0].Locked || !current[0].PasswordDisabled {
		return fail("기존 관리자 비활성화 확인", "기존 계정의 잠금 상태 확인에 실패했습니다. 새 계정으로 확인하세요.")
	}
	done("truenas_admin 계정 잠금 · 비밀번호/SSH/SMB 로그인 비활성화")
	for _, key := range keys {
		if key.Username != "truenas_admin" {
			continue
		}
		if err := verified.Call(ctx, "api_key.delete", []any{key.ID}, nil); err != nil {
			return fail("기존 관리자 API 키 폐기", "일부 API 키 폐기에 실패했습니다. 새 계정으로 기존 키를 확인하세요.")
		}
	}
	done("truenas_admin API 키 폐기")
	result.Complete = true
	return result
}
