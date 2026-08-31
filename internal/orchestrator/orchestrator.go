package orchestrator

import (
	"context"
	"fmt"
	"log"
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

	var repetitions int
	if trial.Repetitions > 0 {
		repetitions = trial.Repetitions
	} else {
		repetitions = 1
	}

	for i := 0; i < repetitions; i++ {
		// prepared tracks whether the Prepare stage has succeeded, i.e.
		// whether the adapter may own real resources (containers, tasks,
		// netns, etc.) that Cleanup needs to tear down. cleanedUp tracks
		// whether Cleanup has already run (as the last regular stage) so we
		// don't invoke it a second time on the happy path.
		prepared := false
		cleanedUp := false

		for _, stage := range stages {
			// if the stage's experiment does not match the trial's experiment, skip it
			if runtimeTC.Trial.ExperimentName != stage.experiment {
				continue
			}
			stageResult, err := stage.fn(ctx, runtimeTC)
			result.RuntimeStages = append(result.RuntimeStages, stageResult)

			if stage.name == harnessruntime.StageCleanup {
				cleanedUp = err == nil
			}

			if err != nil {
				// A later stage failed after resources were created in
				// Prepare. Run cleanup on a best-effort basis so a failed
				// trial doesn't leak containers/VMs/netns on the host. A
				// cleanup failure here is logged, not fatal: it must not
				// mask the original stage error being returned below.
				if prepared && !cleanedUp && stage.name != harnessruntime.StageCleanup {
					for _, adapter := range adapters {
						if adapter.ExperimentName() != stage.experiment {
							continue
						}
						if _, cleanupErr := adapter.Cleanup(ctx, runtimeTC); cleanupErr != nil {
							log.Printf(
								"trial %s: cleanup after %s stage failure also failed: %v",
								trial.ID, stage.name, cleanupErr,
							)
						}
						break
					}
				}
				return failTrial(result, stage.name, err)
			}

			if stage.name == harnessruntime.StagePrepare {
				prepared = true
			}
		}
	}

	// get adapter for the trial's experiment to generate the result
	var resultAdapter harnessruntime.Adapter
	for _, adapter := range adapters {
		if adapter.ExperimentName() == trial.ExperimentName {
			resultAdapter = adapter
			break
		}
	}

	if resultAdapter != nil {
		generatedResult, err := resultAdapter.GenerateResult(ctx, runtimeTC, result.RuntimeStages)
		if err != nil {
			return failTrial(result, "", fmt.Errorf("generate result: %w", err))
		}
		result.Results = generatedResult
	}

	result.RuntimeStages = nil // Clear runtime stages to save space in the final result

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
