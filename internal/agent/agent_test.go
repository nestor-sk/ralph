package agent

import (
	"testing"
)

func TestContainsComplete_WithSignal(t *testing.T) {
	output := "Some output\n<promise>COMPLETE</promise>\nMore output"
	if !ContainsComplete(output) {
		t.Error("expected ContainsComplete to return true")
	}
}

func TestContainsComplete_WithoutSignal(t *testing.T) {
	output := "Some output without the signal"
	if ContainsComplete(output) {
		t.Error("expected ContainsComplete to return false")
	}
}

func TestContainsComplete_Empty(t *testing.T) {
	if ContainsComplete("") {
		t.Error("expected ContainsComplete to return false for empty string")
	}
}

func TestNewInvoker_UnknownAgent(t *testing.T) {
	_, err := NewInvoker("unknown")
	if err == nil {
		t.Error("expected error for unknown agent")
	}
}

