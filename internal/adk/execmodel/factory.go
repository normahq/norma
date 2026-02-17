package execmodel

import (
	"fmt"

	"google.golang.org/adk/model"
)

// Factory is a registry of executive models and their configurations.
type Factory struct {
	config FactoryConfig
}

// NewFactory creates a new Factory from a map of configurations.
func NewFactory(config FactoryConfig) *Factory {
	return &Factory{
		config: config,
	}
}

// CreateLLMModel creates an LLM instance by name.
func (f *Factory) CreateLLMModel(name string) (model.LLM, error) {
	cfg, ok := f.config[name]
	if !ok {
		return nil, fmt.Errorf("exec model %q not found", name)
	}

	cfg.Name = name
	return New(cfg)
}
