package runtime

import (
	"context"
	"time"

	"github.com/urunc-dev/evaluation_suite/internal/plan"
)

type Stage string

const (
	StagePrepare   Stage = "prepare"
	StageCreate    Stage = "create_task"
	StageStart     Stage = "start_task"
	StageWaitReady Stage = "wait_ready"
	StageStop      Stage = "stop"
	StageDelete    Stage = "delete_task"
	StageCleanup   Stage = "cleanup"
)

type StageResult struct {
	Stage       Stage         `json:"stage"`
	StartedAt   time.Time     `json:"startedAt"`
	FinishedAt  time.Time     `json:"finishedAt"`
	Duration    time.Duration `json:"duration"`
	Description string        `json:"description,omitempty"`
	Data        any           `json:"data,omitempty"`
}

type TrialContext struct {
	Trial plan.Trial
}

type Adapter interface {
	ExperimentName() string
	Prepare(ctx context.Context, tc TrialContext) (StageResult, error)
	CreateTask(ctx context.Context, tc TrialContext) (StageResult, error)
	StartTask(ctx context.Context, tc TrialContext) (StageResult, error)
	WaitReady(ctx context.Context, tc TrialContext) (StageResult, error)
	Stop(ctx context.Context, tc TrialContext) (StageResult, error)
	DeleteTask(ctx context.Context, tc TrialContext) (StageResult, error)
	Cleanup(ctx context.Context, tc TrialContext) (StageResult, error)
}
