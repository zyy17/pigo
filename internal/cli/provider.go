package cli

import (
	"fmt"
	"strings"

	"github.com/zyy17/pigo/internal/config"
)

const providerOpenAI = "openai"

func checkProvider(cfg config.Config) error {
	switch cfg.Provider {
	case providerOpenAI:
		if strings.TrimSpace(cfg.APIKey) == "" {
			return fmt.Errorf("OPENAI_API_KEY is not set (export it, add it to .env, or set api_key in pigo.yaml)")
		}
		return nil
	default:
		return fmt.Errorf("unknown PIGO_PROVIDER %q (supported: openai)", cfg.Provider)
	}
}
