package agentfactory

// AgentConfig describes how to run an agent.
type AgentConfig struct {
	Type    string `json:"type"           mapstructure:"type"`
	Model   string `json:"model,omitempty" mapstructure:"model"`
	APIKey  string `json:"api_key,omitempty" mapstructure:"api_key"`
	BaseURL string `json:"base_url,omitempty" mapstructure:"base_url"`
	Timeout int    `json:"timeout,omitempty" mapstructure:"timeout"`
}

// FactoryConfig is a map of agent configurations.
type FactoryConfig map[string]AgentConfig

const (
	// AgentTypeGeminiAIStudio is the type for Gemini AI Studio agents.
	AgentTypeGeminiAIStudio = "gemini_aistudio"
	// AgentTypeOpenAI is the type for OpenAI agents.
	AgentTypeOpenAI = "openai"
)
