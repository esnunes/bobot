// tools/registry_test.go
package tools

import (
	"context"
	"testing"
)

type mockTool struct{}

func (m *mockTool) Name() string        { return "mock" }
func (m *mockTool) Description() string { return "A mock tool" }
func (m *mockTool) Schema() interface{} { return map[string]string{"type": "object"} }
func (m *mockTool) Execute(ctx context.Context, input map[string]interface{}) (string, error) {
	return "executed", nil
}

func TestRegistry_Register(t *testing.T) {
	reg := NewRegistry()
	mock := &mockTool{}

	reg.Register(mock)

	if len(reg.List()) != 1 {
		t.Errorf("expected 1 tool, got %d", len(reg.List()))
	}
}

func TestRegistry_Get(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockTool{})

	tool, ok := reg.Get("mock")
	if !ok {
		t.Fatal("expected to find mock tool")
	}
	if tool.Name() != "mock" {
		t.Errorf("expected name mock, got %s", tool.Name())
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	reg := NewRegistry()

	_, ok := reg.Get("nonexistent")
	if ok {
		t.Error("expected not to find tool")
	}
}

func TestRegistry_Execute(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockTool{})

	result, err := reg.Execute(context.Background(), "mock", map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "executed" {
		t.Errorf("expected 'executed', got '%s'", result)
	}
}
