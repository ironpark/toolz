# Claude Code (`claude`) — 설정 파일과 설정 키

> [README](README.md) · [../](../README.md)

## 설정 파일과 우선순위

### 파일과 스코프

| 스코프 | 파일 | 영향 범위 |
| --- | --- | --- |
| User | `~/.claude/settings.json` | 이 머신의 모든 프로젝트 |
| Shared project | `<project>/.claude/settings.json` | 해당 폴더의 모두(커밋 대상) |
| Project local | `<project>/.claude/settings.local.json` | 이 프로젝트의 나만(자동 git 제외) |
| Managed | `managed-settings.json`, MDM, claude.ai 콘솔 | 조직이 배포한 모두 |

- Claude Code는 별도로 `~/.claude.json`을 스스로 관리합니다(로그인 세션, MCP 설정,
  프로젝트별 신뢰 결정, `/config`가 쓰는 global config 키).
- git 저장소 하위 디렉터리에서 시작해도 `settings.local.json`은 **저장소 루트**에 읽고 씁니다.
  worktree에서는 메인 체크아웃 루트의 파일을 사용합니다.
- 공유 `settings.json`은 세션의 **주 작업 디렉터리** 기준으로 읽습니다.

### 우선순위 (높은 순)

1. **Managed settings** — 무엇으로도 덮어쓸 수 없음. `--settings`도 못 이김
2. **커맨드라인** — `--settings <file-or-json>` 등, 한 세션 한정
3. **Project local** — `.claude/settings.local.json`
4. **Shared project** — `.claude/settings.json`
5. **User** — `~/.claude/settings.json`

리스트형 키(`permissions.allow` 등)는 **덮어쓰지 않고 병합**됩니다.
단, `fallbackModel`, `modelPicker`, `availableModels`, `modelSettings`는 별도 규칙을 따릅니다.

환경변수는 이 스택의 레벨이 **아닙니다**. 환경변수와 설정 키가 짝을 이루는 경우 어느 쪽이
이기는지는 쌍마다 다릅니다(§4 참고).

## 설정 키 (공식 settings-reference 요약)

spona 프리셋 스키마 설계에 직접 관계되는 그룹만 추립니다. 전체 목록은
<https://code.claude.com/docs/en/settings-reference> 참고.

| 그룹 | 대표 키 |
| --- | --- |
| 모델/응답 | `model`, `availableModels`, `enforceAvailableModels`, `fallbackModel`, `effortLevel`, `modelSettings`, `modelPicker`, `modelOverrides`, `outputStyle`, `promptCacheTtl`, `subagentPromptCacheTtl`, `alwaysThinkingEnabled`, `language` |
| 메모리/컨텍스트 | `claudeMd`, `claudeMdExcludes`, `autoCompactEnabled`, `autoCompactWindow`, `autoMemoryEnabled`, `autoMemoryDirectory`, `plansDirectory`, **`env`** |
| 권한 | `permissions.allow` / `.ask` / `.deny` / `.defaultMode` / `.additionalDirectories`, `permissions.disableBypassPermissionsMode`, `autoMode`, `disableAutoMode` |
| 샌드박스 | `sandbox.enabled`, `sandbox.filesystem.*`, `sandbox.network.*`, `sandbox.credentials.*`, `sandbox.excludedCommands`, `sandbox.failIfUnavailable` |
| MCP | `allowedMcpServers`, `deniedMcpServers`, `enableAllProjectMcpServers`, `enabledMcpjsonServers`, `disabledMcpjsonServers`, `allowManagedMcpServersOnly`, `disableClaudeAiConnectors` |
| 플러그인/스킬 | `enabledPlugins`, `extraKnownMarketplaces`, `strictKnownMarketplaces`, `blockedMarketplaces`, `disableBundledSkills`, `skillOverrides`, `strictPluginOnlyCustomization.*` |
| 훅/자동화 | `hooks`, `disableAllHooks`, `allowManagedHooksOnly`, `allowedHttpHookUrls`, `enableWorkflows`, `disableWorkflows` |
| 에이전트/세션 | `agent`, `disableAgentView`, `crossSessionInbound`, `isolatePeerMachines`, `processWrapper` |
| 인증/프로바이더 | `apiKeyHelper`, `awsAuthRefresh`, `awsCredentialExport`, `gcpAuthRefresh`, `forceLoginMethod`, `forceLoginOrgUUID`, `forceLoginGatewayUrl` |
| Git/기여 표시 | `attribution.commit` / `.pr` / `.sessionUrl`, `includeGitInstructions`, `prUrlTemplate` |
| 엔터프라이즈 | `disableSideloadFlags`, `policyHelper.*`, `managedSourcesBehavior`, `parentSettingsBehavior`, `requiredMinimumVersion` |

`~/.claude.json`에만 들어가는 **global config 키**: `autoConnectIde`,
`autoInstallIdeExtension`, `diffTool`, `externalEditorContext`.

> [!TIP]
> `env` 설정 키는 모든 세션과 그 하위 프로세스에 환경변수를 주입합니다. spona가 프리셋의
> 환경변수를 전달할 때, 셸 export 대신 `--settings '{"env":{...}}'` 한 덩어리로 넘기면
> 프리셋 하나가 곧 하나의 인자가 되어 관리가 단순해집니다.
