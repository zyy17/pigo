package cli

import (
	"fmt"
	"os"
	"strings"
)

const providerOpenAI = "openai"

// endpoint is the chat backend selected by PIGO_PROVIDER.
// openai uses the OPENAI_* variables and speaks OpenAI-compatible chat completions.
type endpoint struct {
	apiKey  string
	baseURL string
	model   string
}

func endpointFromEnv(baseURL, model string) (endpoint, error) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("PIGO_PROVIDER")))
	if provider == "" {
		provider = providerOpenAI
	}
	switch provider {
	case providerOpenAI:
		return openAIEndpoint(baseURL, model)
	default:
		return endpoint{}, fmt.Errorf("unknown PIGO_PROVIDER %q (supported: openai)", provider)
	}
}

func openAIEndpoint(baseURL, model string) (endpoint, error) {
	key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if key == "" {
		return endpoint{}, fmt.Errorf("OPENAI_API_KEY is not set (export it or add it to .env)")
	}
	return endpoint{
		apiKey:  key,
		baseURL: firstNonEmpty(baseURL, os.Getenv("OPENAI_BASE_URL")),
		model:   firstNonEmpty(model, os.Getenv("OPENAI_MODEL")),
	}, nil
}
