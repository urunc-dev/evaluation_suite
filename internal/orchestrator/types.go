package orchestrator

import (
	"time"

	"github.com/urunc-dev/evaluation_suite/internal/plan"
	harnessruntime "github.com/urunc-dev/evaluation_suite/internal/runtime"
)

type TrialStatus string

const (
	TrialStatusSuccess TrialStatus = "success"
	TrialStatusFailed  TrialStatus = "failed"
	TrialStatusSkipped TrialStatus = "skipped"
)

type RunResult struct {
	RunID     string        `json:"runId"`
	DryRun    bool          `json:"dryRun"`
	StartedAt time.Time     `json:"startedAt"`
	EndedAt   time.Time     `json:"endedAt"`
	Duration  time.Duration `json:"duration"`
	Trials    []TrialResult `json:"trials"`
}

type TrialResult struct {
	Trial         plan.Trial                   `json:"trial"`
	Status        TrialStatus                  `json:"status"`
	FailedStage   harnessruntime.Stage         `json:"failedStage,omitempty"`
	Error         string                       `json:"error,omitempty"`
	StartedAt     time.Time                    `json:"startedAt"`
	EndedAt       time.Time                    `json:"endedAt"`
	Duration      time.Duration                `json:"duration"`
	RuntimeStages []harnessruntime.StageResult `json:"runtimeStages"`
}
