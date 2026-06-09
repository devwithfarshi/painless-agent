package agent

import "github.com/devwithfarshi/painless-agent/internal/llm"

const charsPerToken = 4

// ContextManager maintains a rolling message window that stays within a token budget.
// When the window is over budget, the oldest non-system messages are dropped.
type ContextManager struct {
	messages    []llm.Message
	tokenBudget int
}

func NewContextManager(tokenBudget int) *ContextManager {
	return &ContextManager{tokenBudget: tokenBudget}
}

func (c *ContextManager) Add(msg llm.Message) {
	c.messages = append(c.messages, msg)
}

// Window returns a message slice that fits within the token budget.
// System messages are always preserved. When trimming, messages are always dropped
// at turn boundaries (starting at a user message) to avoid orphaned tool-result
// messages, which cause HTTP 400 errors from providers that require every
// role=tool message to be preceded by role=assistant with matching tool_calls.
func (c *ContextManager) Window() []llm.Message {
	if c.estimate(c.messages) <= c.tokenBudget {
		return c.messages
	}

	var system []llm.Message
	var rest []llm.Message
	for _, m := range c.messages {
		if m.Role == llm.RoleSystem {
			system = append(system, m)
		} else {
			rest = append(rest, m)
		}
	}

	// Drop whole turns (user message → next user message) until under budget.
	// This ensures we never start the window in the middle of a tool-call exchange.
	for len(rest) > 0 && c.estimate(append(system, rest...)) > c.tokenBudget {
		// Find the next user message after position 0. Drop everything before it.
		nextUser := -1
		for i := 1; i < len(rest); i++ {
			if rest[i].Role == llm.RoleUser {
				nextUser = i
				break
			}
		}
		if nextUser < 0 {
			// Only one turn left — keep it even if over budget.
			break
		}
		rest = rest[nextUser:]
	}

	// Safety net: if the window still starts with a non-user message (e.g. we only
	// have one turn of tool calls with no user preamble), skip any leading
	// orphaned tool or assistant messages so providers never see a bare tool message.
	for len(rest) > 0 && rest[0].Role == llm.RoleTool {
		rest = rest[1:]
	}

	// If trimming left nothing, preserve at least the most recent user message.
	if len(rest) == 0 {
		for i := len(c.messages) - 1; i >= 0; i-- {
			if c.messages[i].Role == llm.RoleUser {
				rest = c.messages[i:]
				break
			}
		}
	}

	return append(system, rest...)
}

func (c *ContextManager) Reset() { c.messages = nil }

func (c *ContextManager) estimate(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content)/charsPerToken + 4 // +4 overhead per message
		for _, tc := range m.ToolCalls {
			total += len(tc.Name)/charsPerToken + 8
		}
	}
	return total
}
