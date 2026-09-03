package process

import (
	"maps"
	"os"
	"slices"
)

// Env puts extra on top of this process's environment. The parent's is
// inherited because an agent CLI needs its own credentials and PATH, and
// stripping them would only mean measuring an agent that cannot log in.
//
// extra comes last and in sorted order, so a configuration's setting wins and
// two runs of it differ in the agent's behaviour rather than in their inputs.
func Env(extra map[string]string) []string {
	environ := slices.Grow(os.Environ(), len(extra))
	for _, key := range slices.Sorted(maps.Keys(extra)) {
		environ = append(environ, key+"="+extra[key])
	}
	return environ
}
