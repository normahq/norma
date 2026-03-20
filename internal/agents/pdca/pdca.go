package pdca

import (
	"sync"

	"github.com/metalagman/norma/internal/agents/pdca/roles"
	"github.com/metalagman/norma/internal/agents/roleagent"
)

const (
	RolePlan  = "plan"
	RoleDo    = "do"
	RoleCheck = "check"
	RoleAct   = "act"
)

var (
	roleMap  = make(map[string]roleagent.RoleContract)
	initOnce sync.Once
)

func initializeRoles() {
	initOnce.Do(func() {
		for name, role := range roles.DefaultRoles() {
			roleMap[name] = role
		}
	})
}

// Role returns the role implementation by name.
func Role(name string) roleagent.RoleContract {
	initializeRoles()
	return roleMap[name]
}
