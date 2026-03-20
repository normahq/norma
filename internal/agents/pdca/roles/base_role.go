package roles

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"

	"github.com/metalagman/norma/internal/agents/roleagent"
)

//go:embed common.gotmpl
var commonPromptTemplate string

type baseRole struct {
	name     string
	schemas  roleagent.SchemaPair
	baseTmpl *template.Template
	roleTmpl *template.Template
}

func newBaseRole(name, inputSchema, outputSchema, roleTmplStr string) *baseRole {
	baseTmpl := template.Must(template.New(name + "-base").Parse(commonPromptTemplate))
	roleTmpl := template.Must(template.New(name).Parse(roleTmplStr))
	return &baseRole{
		name: name,
		schemas: roleagent.SchemaPair{
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
		},
		baseTmpl: baseTmpl,
		roleTmpl: roleTmpl,
	}
}

func (r *baseRole) Name() string                  { return r.name }
func (r *baseRole) Schemas() roleagent.SchemaPair { return r.schemas }

func (r *baseRole) Prompt(req roleagent.AgentRequest) (string, error) {
	var baseBuf bytes.Buffer
	if err := r.baseTmpl.Execute(&baseBuf, struct {
		Request roleagent.AgentRequest
	}{Request: req}); err != nil {
		return "", fmt.Errorf("execute base prompt template: %w", err)
	}

	data := struct {
		Request      roleagent.AgentRequest
		CommonPrompt string
	}{
		Request:      req,
		CommonPrompt: baseBuf.String(),
	}

	var buf bytes.Buffer
	if err := r.roleTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute prompt template: %w", err)
	}

	return buf.String(), nil
}
