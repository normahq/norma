package config

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

//go:embed schema.json
var schemaJSON string

// ValidateSettings validates raw config settings against the JSON schema.
func ValidateSettings(settings map[string]any) error {
	if agentName, envVar, ok := openAIAPIKeyEnvUsage(settings); ok {
		return fmt.Errorf(
			"config validation failed: agents.%s.api_key_env is no longer supported for openai agents; migrate to api_key (for example, api_key: ${%s}) and remove api_key_env",
			agentName,
			envVar,
		)
	}

	schemaLoader := gojsonschema.NewStringLoader(schemaJSON)
	documentLoader := gojsonschema.NewGoLoader(settings)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("validate config schema: %w", err)
	}
	if result.Valid() {
		return nil
	}

	errs := make([]string, 0, len(result.Errors()))
	for _, schemaErr := range result.Errors() {
		errs = append(errs, schemaErr.String())
	}
	sort.Strings(errs)

	return fmt.Errorf("config schema validation failed: %s", strings.Join(errs, "; "))
}

func openAIAPIKeyEnvUsage(settings map[string]any) (agentName, envVar string, found bool) {
	agents, ok := asStringAnyMap(settings["agents"])
	if !ok {
		return "", "", false
	}

	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		agentCfg, ok := asStringAnyMap(agents[name])
		if !ok {
			continue
		}
		agentType, _ := agentCfg["type"].(string)
		if agentType != AgentTypeOpenAI {
			continue
		}

		rawEnvVar, exists := agentCfg["api_key_env"]
		if !exists {
			continue
		}

		env := "OPENAI_API_KEY"
		if value, ok := rawEnvVar.(string); ok {
			trimmed := strings.TrimSpace(value)
			if trimmed != "" {
				env = trimmed
			}
		}
		return name, env, true
	}

	return "", "", false
}

func asStringAnyMap(value any) (map[string]any, bool) {
	if typed, ok := value.(map[string]any); ok {
		return typed, true
	}
	typed, ok := value.(map[any]any)
	if !ok {
		return nil, false
	}

	converted := make(map[string]any, len(typed))
	for key, entry := range typed {
		name, ok := key.(string)
		if !ok {
			return nil, false
		}
		converted[name] = entry
	}

	return converted, true
}
