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
// System messages are always preserved; oldest non-system messages are dropped first.
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

	// Drop oldest non-system messages until under budget.
	for len(rest) > 1 && c.estimate(append(system, rest...))+0 > c.tokenBudget {
		rest = rest[1:]
	}
	return append(system, rest...)
}

func (c *ContextManager) Reset() { c.messages = nil }

func (c *ContextManager) estimate(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content)/charsPerToken + 4 // +4 overhead per message
	}
	return total
}
