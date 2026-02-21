package main

import (
	"context"
	"testing"

	"termteam/cli/internal/config"
)

func TestRunMode_DefaultLocal(t *testing.T) {
	cfg := config.Config{Mode: ""}
	turnCalled := false
	localCalled := false

	err := runByMode(context.Background(), cfg, func(context.Context) error {
		turnCalled = true
		return nil
	}, func(context.Context) error {
		localCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("runByMode failed: %v", err)
	}
	if !localCalled || turnCalled {
		t.Fatalf("expected default local mode, turnCalled=%v localCalled=%v", turnCalled, localCalled)
	}
}

func TestRunMode_ExplicitTurn(t *testing.T) {
	cfg := config.Config{Mode: "turn"}
	turnCalled := false
	localCalled := false

	err := runByMode(context.Background(), cfg, func(context.Context) error {
		turnCalled = true
		return nil
	}, func(context.Context) error {
		localCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("runByMode failed: %v", err)
	}
	if !turnCalled || localCalled {
		t.Fatalf("expected explicit turn mode, turnCalled=%v localCalled=%v", turnCalled, localCalled)
	}
}
