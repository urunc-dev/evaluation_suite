package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/urunc-dev/evaluation_suite/internal/plan"
	harnessruntime "github.com/urunc-dev/evaluation_suite/internal/runtime"
)

type Options struct {
	RunID string
}

type Orchestrator struct {
	adapterFactories []AdapterFactory
}

type AdapterFactory func(trial plan.Trial) (harnessruntime.Adapter, error)

func New(
	adapterFactories ...AdapterFactory,
) *Orchestrator {
	return &Orchestrator{
		adapterFactories: adapterFactories,
	}
}

func (o *Orchestrator) Run(
	ctx context.Context,
	p *plan.Plan,
	opts Options,
) (*RunResult, error) {
	startedAt := time.Now()

	runID := opts.RunID
	if runID == "" {
		runID = NewRunID(startedAt)
	}

	trials := p.Trials

	result := &RunResult{
		RunID:     runID,
		StartedAt: startedAt,
		Trials:    []TrialResult{},
	}

	for _, trial := range trials {
		trialResult := o.runTrial(ctx, trial, opts)
		result.Trials = append(result.Trials, trialResult)
	}

	result.EndedAt = time.Now()
	result.Duration = result.EndedAt.Sub(result.StartedAt)

	return result, nil
}

func (o *Orchestrator) runTrial(
	ctx context.Context,
	trial plan.Trial,
	opts Options,
) TrialResult {
	startedAt := time.Now()

	result := TrialResult{
		Trial:         trial,
		Status:        TrialStatusSuccess,
		StartedAt:     startedAt,
		RuntimeStages: []harnessruntime.StageResult{},
	}

	var adapters []harnessruntime.Adapter
	for _, factory := range o.adapterFactories {
		adapter, err := factory(trial)
		if err != nil {
			return failTrial(result, "", err)
		}
		adapters = append(adapters, adapter)
	}

	if len(adapters) == 0 {
		return failTrial(result, "", fmt.Errorf("no adapters available for trial %s", trial.ID))
	}

	runtimeTC := harnessruntime.TrialContext{
		Trial: trial,
	}

	var stages []struct {
		name       harnessruntime.Stage
		experiment string
		fn         func(context.Context, harnessruntime.TrialContext) (harnessruntime.StageResult, error)
	}

	for _, adapter := range adapters {
		stages = append(stages, []struct {
			name       harnessruntime.Stage
			experiment string
			fn         func(context.Context, harnessruntime.TrialContext) (harnessruntime.StageResult, error)
		}{
			{
				name:       harnessruntime.StagePrepare,
				experiment: adapter.ExperimentName(),
				fn:         adapter.Prepare,
			},
			{
				name:       harnessruntime.StageCreate,
				experiment: adapter.ExperimentName(),
				fn:         adapter.CreateTask,
			},
			{
				name:       harnessruntime.StageStart,
				experiment: adapter.ExperimentName(),
				fn:         adapter.StartTask,
			},
			{
				name:       harnessruntime.StageWaitReady,
				experiment: adapter.ExperimentName(),
				fn:         adapter.WaitReady,
			},
			{
				name:       harnessruntime.StageStop,
				experiment: adapter.ExperimentName(),
				fn:         adapter.Stop,
			},
			{
				name:       harnessruntime.StageDelete,
				experiment: adapter.ExperimentName(),
				fn:         adapter.DeleteTask,
			},
			{
				name:       harnessruntime.StageCleanup,
				experiment: adapter.ExperimentName(),
				fn:         adapter.Cleanup,
			},
		}...)
	}

	for _, stage := range stages {
		// if the stage's experiment does not match the trial's experiment, skip it
		if runtimeTC.Trial.ExperimentName != stage.experiment {
			continue
		}
		stageResult, err := stage.fn(ctx, runtimeTC)
		if err != nil {
			result.RuntimeStages = append(result.RuntimeStages, stageResult)
			return failTrial(result, stage.name, err)
		}

		result.RuntimeStages = append(result.RuntimeStages, stageResult)
	}

	result.EndedAt = time.Now()
	result.Duration = result.EndedAt.Sub(result.StartedAt)

	return result
}

func failTrial(
	result TrialResult,
	stage harnessruntime.Stage,
	err error,
) TrialResult {
	result.Status = TrialStatusFailed
	result.FailedStage = stage
	result.Error = err.Error()
	result.EndedAt = time.Now()
	result.Duration = result.EndedAt.Sub(result.StartedAt)

	return result
}
