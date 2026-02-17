package execmodel_test

import (
	"testing"

	"github.com/metalagman/norma/internal/adk/execmodel"
	"github.com/stretchr/testify/assert"
)

func TestFactory_CreateLLMModel(t *testing.T) {
	config := execmodel.FactoryConfig{
		"m1": {
			Cmd: []string{"echo", "m1"},
		},
		"m2": {
			Cmd: []string{"echo", "m2"},
		},
	}

	f := execmodel.NewFactory(config)

	tests := []struct {
		name    string
		target  string
		wantErr string
	}{
		{
			name:   "found_m1",
			target: "m1",
		},
		{
			name:   "found_m2",
			target: "m2",
		},
		{
			name:    "not_found",
			target:  "unknown",
			wantErr: `exec model "unknown" not found`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := f.CreateLLMModel(tt.target)
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, m)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, m)
				assert.Equal(t, tt.target, m.Name())
			}
		})
	}
}
