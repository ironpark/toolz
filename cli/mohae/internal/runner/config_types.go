package runner

import "github.com/ironpark/toolz/cli/mohae/internal/config"

// Configuration types are owned by config. These aliases keep runner APIs
// readable while preserving that single source of truth.
type Config = config.Config
type Prompt = config.Prompt
type HookCommand = config.HookCommand

const (
	HookScopeWorkspace = config.HookScopeWorkspace
	HookScopeOutside   = config.HookScopeOutside
)
