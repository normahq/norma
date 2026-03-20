package roleagent

import (
	"bytes"
	"fmt"
	"text/template"
)

const DefaultBasePromptTemplate = `You are a %s agent. Follow the instructions strictly.
- IMPORTANT: ACCESS RESTRICTION: You MUST ONLY read files within the assigned 'run_dir' and 'workspace_dir' as specified in the input JSON.
- IMPORTANT: DO NOT attempt to read, list, or index the project root directory or any directory outside of your assigned workspace.
- IMPORTANT: DO NOT use recursive tools (like 'grep -r', 'find', or 'ls -R') on the project root.
- IMPORTANT: Accessing files outside of your assigned directories will cause a PERMISSION ERROR and failure of the run.
- IMPORTANT: Do NOT read or modify any files in the 'logs' directory. This directory is reserved for the orchestrator to capture your output.
- Follow the norma-loop: plan -> do -> check -> act.
- Workspace exists before any agent runs.
- Agents never modify workspace or git directly. Git commands are forbidden, except read-only 'git diff' in the check step.
- Agents never modify task state, labels, or metadata directly; this is handled by the orchestrator.
- All agents operate in read-only mode with respect to the project codebase (except Do, which may only perform file writes).
- IMPORTANT: In 'do' step, the orchestrator will commit your changes. You MUST NOT run 'git add' or 'git commit'.
- Use status='ok' if you successfully completed your task, even if tests failed or results are not perfect.
- Use status='stop' or 'error' only for technical failures or when budgets are exceeded.
`

type BasePromptBuilder struct {
	baseTmpl *template.Template
	roleTmpl *template.Template
	roleName string
}

func NewBasePromptBuilder(roleName, rolePrompt string) (*BasePromptBuilder, error) {
	baseStr := fmt.Sprintf(DefaultBasePromptTemplate, roleName)
	baseTmpl, err := template.New(roleName + "-base").Parse(baseStr)
	if err != nil {
		return nil, fmt.Errorf("parse base template: %w", err)
	}

	roleTmpl, err := template.New(roleName).Parse(rolePrompt)
	if err != nil {
		return nil, fmt.Errorf("parse role template: %w", err)
	}

	return &BasePromptBuilder{
		baseTmpl: baseTmpl,
		roleTmpl: roleTmpl,
		roleName: roleName,
	}, nil
}

func (b *BasePromptBuilder) Build(commonData, roleData any) (string, error) {
	var baseBuf bytes.Buffer
	if err := b.baseTmpl.Execute(&baseBuf, commonData); err != nil {
		return "", fmt.Errorf("execute base template: %w", err)
	}

	data := struct {
		Common any
		Role   any
	}{
		Common: commonData,
		Role:   roleData,
	}

	var buf bytes.Buffer
	if err := b.roleTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute role template: %w", err)
	}

	var combined bytes.Buffer
	combined.WriteString(baseBuf.String())
	combined.WriteString("\n\n")
	combined.WriteString(buf.String())

	return combined.String(), nil
}

func (b *BasePromptBuilder) BuildFromRequest(req AgentRequest, roleData any) (string, error) {
	return b.Build(req, roleData)
}

func (b *BasePromptBuilder) RoleName() string {
	return b.roleName
}
