package main

import (
	"context"
	"errors"
	"testing"

	"shellman/cli/internal/config"
)

func TestRunServe_LocalAlwaysRequired(t *testing.T) {
	cfg := config.Config{TurnEnabled: false}
	localCalled := false
	turnCalled := false

	err := runServe(
		context.Background(),
		cfg,
		func(context.Context) error {
			localCalled = true
			return nil
		},
		func(context.Context) error {
			turnCalled = true
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("runServe failed: %v", err)
	}
	if !localCalled || turnCalled {
		t.Fatalf("expected local only, turnCalled=%v localCalled=%v", turnCalled, localCalled)
	}
}

func TestRunServe_TurnFailureDoesNotFailLocal(t *testing.T) {
	cfg := config.Config{TurnEnabled: true}
	localCalled := false
	turnCalled := false

	err := runServe(
		context.Background(),
		cfg,
		func(context.Context) error {
			localCalled = true
			return nil
		},
		func(context.Context) error {
			turnCalled = true
			return errors.New("register failed")
		},
		testLogger(),
	)
	if err != nil {
		t.Fatalf("runServe should ignore turn failure, got %v", err)
	}
	if !turnCalled || !localCalled {
		t.Fatalf("expected both paths attempted, turnCalled=%v localCalled=%v", turnCalled, localCalled)
	}
}
