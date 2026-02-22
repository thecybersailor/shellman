package localapi

import (
	"errors"
	"strings"

	"shellman/cli/internal/projectstate"
)

var errInvalidTaskRole = errors.New("task_role must be one of full|planner|executor")
var errExecutorCannotDelegate = errors.New("executor task cannot delegate child tasks")
var errPlannerOnlySpawnExecutor = errors.New("planner task can only spawn executor child")

func normalizeTaskRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "", projectstate.TaskRoleFull:
		return projectstate.TaskRoleFull
	case projectstate.TaskRolePlanner:
		return projectstate.TaskRolePlanner
	case projectstate.TaskRoleExecutor:
		return projectstate.TaskRoleExecutor
	default:
		return ""
	}
}

func validTaskRole(role string) bool {
	return normalizeTaskRole(role) != ""
}
