package progdetector

import (
	"reflect"
	"testing"

	"shellman/cli/internal/programadapter"
)

func TestDetectorTypeMatchesProgramAdapterContract(t *testing.T) {
	detectorType := reflect.TypeOf((*Detector)(nil)).Elem()
	contractType := reflect.TypeOf((*programadapter.ProgramAdapter)(nil)).Elem()
	if detectorType != contractType {
		t.Fatalf("detector type must equal shared contract type: detector=%v contract=%v", detectorType, contractType)
	}
}

func TestRuntimeStateTypeMatchesProgramAdapterContract(t *testing.T) {
	stateType := reflect.TypeOf(RuntimeState{})
	contractType := reflect.TypeOf(programadapter.RuntimeState{})
	if stateType != contractType {
		t.Fatalf("runtime state type must equal shared contract type: state=%v contract=%v", stateType, contractType)
	}
}

func TestPromptStepTypeMatchesProgramAdapterContract(t *testing.T) {
	stepType := reflect.TypeOf(PromptStep{})
	contractType := reflect.TypeOf(programadapter.PromptStep{})
	if stepType != contractType {
		t.Fatalf("prompt step type must equal shared contract type: step=%v contract=%v", stepType, contractType)
	}
}
