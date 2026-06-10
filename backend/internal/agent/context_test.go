package agent

import (
	"testing"

	"github.com/devwithfarshi/painless-agent/internal/llm"
)

func mkMsg(role llm.Role, content string) llm.Message {
	return llm.Message{Role: role, Content: content}
}

func repeat(s string, n int) string {
	b := make([]byte, len(s)*n)
	for i := 0; i < n; i++ {
		copy(b[i*len(s):], s)
	}
	return string(b)
}

func filterNonSystem(msgs []llm.Message) []llm.Message {
	var out []llm.Message
	for _, m := range msgs {
		if m.Role != llm.RoleSystem {
			out = append(out, m)
		}
	}
	return out
}

func TestContextManager_UnderBudget(t *testing.T) {
	cm := NewContextManager(10000)
	cm.Add(mkMsg(llm.RoleSystem, "system"))
	cm.Add(mkMsg(llm.RoleUser, "hi"))
	cm.Add(mkMsg(llm.RoleAssistant, "hello"))

	w := cm.Window()
	if len(w) != 3 {
		t.Errorf("got %d messages, want 3", len(w))
	}
}

func TestContextManager_SystemMessagePreserved(t *testing.T) {
	cm := NewContextManager(2)
	cm.Add(mkMsg(llm.RoleSystem, "sys"))
	cm.Add(mkMsg(llm.RoleUser, repeat("a", 100)))
	cm.Add(mkMsg(llm.RoleAssistant, repeat("b", 100)))
	cm.Add(mkMsg(llm.RoleUser, repeat("c", 100)))

	w := cm.Window()
	if len(w) == 0 || w[0].Role != llm.RoleSystem {
		t.Errorf("system message not preserved; window: %v", w)
	}
}

func TestContextManager_TrimAtTurnBoundary(t *testing.T) {
	cm := NewContextManager(50) // ~200 chars budget
	cm.Add(mkMsg(llm.RoleSystem, "system prompt"))
	// Turn 1 — will be trimmed
	cm.Add(mkMsg(llm.RoleUser, repeat("q", 60)))
	cm.Add(mkMsg(llm.RoleAssistant, repeat("a", 60)))
	// Turn 2 — retained
	cm.Add(mkMsg(llm.RoleUser, "short"))
	cm.Add(mkMsg(llm.RoleAssistant, "reply"))

	w := cm.Window()
	nonSystem := filterNonSystem(w)
	if len(nonSystem) > 0 && nonSystem[0].Role != llm.RoleUser {
		t.Errorf("window starts with non-user non-system message: role=%s", nonSystem[0].Role)
	}
}

func TestContextManager_AddAndWindowIdempotent(t *testing.T) {
	cm := NewContextManager(10000)
	cm.Add(mkMsg(llm.RoleUser, "one"))
	cm.Add(mkMsg(llm.RoleAssistant, "two"))

	w1 := cm.Window()
	w2 := cm.Window()
	if len(w1) != len(w2) {
		t.Errorf("Window not idempotent: %d vs %d", len(w1), len(w2))
	}
}
