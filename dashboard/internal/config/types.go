package config

type Instance struct {
	Name     string `json:"name"`
	Status   string `json:"status"` // Running, Pending, CrashLoopBackOff, etc.
	Model    string `json:"model"`
	Provider string `json:"provider"`
	Ready    bool   `json:"ready"`
	Age      string `json:"age"`
	CPU      string `json:"cpu_limit"`
	Memory   string `json:"memory_limit"`
}

type DeployRequest struct {
	Name          string            `json:"name"`
	Provider      string            `json:"provider"`
	Model         string            `json:"model"`
	APIKey        string            `json:"api_key"`
	CPULimit      string            `json:"cpu_limit,omitempty"`
	MemoryLimit   string            `json:"memory_limit,omitempty"`
	StorageSize   string            `json:"storage_size,omitempty"`
	CustomEnv     map[string]string `json:"custom_env,omitempty"`
	LinearAPIKey  string            `json:"linear_api_key,omitempty"`
	SlackBotToken string            `json:"slack_bot_token,omitempty"`
}

type InstanceStatus struct {
	Instance
	GRPCStatus *GRPCStatus `json:"grpc_status,omitempty"`
	PodName    string      `json:"pod_name"`
	PodIP      string      `json:"pod_ip"`
}

type GRPCStatus struct {
	Ready         bool   `json:"ready"`
	Model         string `json:"model"`
	Provider      string `json:"provider"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

type SecretUpdate struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ChatMessage struct {
	Message    string `json:"message"`
	SessionKey string `json:"session_key,omitempty"`
}

type ChatResponse struct {
	Text  string `json:"text"`
	Done  bool   `json:"done"`
	Error string `json:"error,omitempty"`
}

// Provider info
type Provider struct {
	Name    string `json:"name"`
	APIBase string `json:"api_base"`
}

var SupportedProviders = []Provider{
	{Name: "anthropic", APIBase: "https://api.anthropic.com/v1"},
	{Name: "openai", APIBase: "https://api.openai.com/v1"},
	{Name: "gemini", APIBase: "https://generativelanguage.googleapis.com/v1beta"},
	{Name: "deepseek", APIBase: "https://api.deepseek.com/v1"},
	{Name: "mistral", APIBase: "https://api.mistral.ai/v1"},
	{Name: "xai", APIBase: "https://api.x.ai/v1"},
	{Name: "kimi", APIBase: "https://api.moonshot.cn/v1"},
	{Name: "groq", APIBase: "https://api.groq.com/openai/v1"},
	{Name: "openrouter", APIBase: "https://openrouter.ai/api/v1"},
	{Name: "copilot", APIBase: "https://api.githubcopilot.com"},
}

func ProviderAPIBase(provider string) string {
	for _, p := range SupportedProviders {
		if p.Name == provider {
			return p.APIBase
		}
	}
	return ""
}

func ProviderModelPrefix(provider string) string {
	switch provider {
	case "gemini":
		return "gemini/"
	case "anthropic":
		return "anthropic/"
	case "openai":
		return "openai/"
	case "groq":
		return "groq/"
	case "deepseek":
		return "deepseek/"
	case "openrouter":
		return "openrouter/"
	case "copilot":
		return "copilot/"
	case "mistral":
		return "mistral/"
	case "xai":
		return "xai/"
	case "kimi":
		return "kimi/"
	default:
		return provider + "/"
	}
}
