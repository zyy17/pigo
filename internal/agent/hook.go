package agent

import (
	"context"

	"github.com/zyy17/pigo/internal/ai"
)

// Decision is the pre-tool hook result.
// Block skips Execute. Reason becomes the tool result.
// Args, when non-nil, replaces the arguments passed to Execute.
type Decision struct {
	Block  bool
	Reason string
	Args   []byte
}

// BeforeToolCall can allow, block, or rewrite a call.
// A nil hook allows every call. The agent has no safety policy of its own.
type BeforeToolCall func(ctx context.Context, call ai.ToolCall) (Decision, error)
