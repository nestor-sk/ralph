package cursor

import (
	"testing"

	"github.com/uesteibar/ralph/internal/agent"
)

func TestNewInvoker_Cursor(t *testing.T) {
	inv, err := agent.NewInvoker("cursor")
	if err != nil {
		t.Fatalf("NewInvoker(cursor): %v", err)
	}
	if inv == nil {
		t.Fatal("expected non-nil invoker")
	}
	if inv.ConfigDir() != ".cursor" {
		t.Errorf("ConfigDir() = %q, want .cursor", inv.ConfigDir())
	}
}
