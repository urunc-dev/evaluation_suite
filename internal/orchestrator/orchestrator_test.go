package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/urunc-dev/evaluation_suite/internal/plan"
	harnessruntime "github.com/urunc-dev/evaluation_suite/internal/runtime"
)

// fakeAdapter is a minimal harnessruntime.Adapter used to drive runTrial
// without needing a real containerd daemon. Each stage method records that
// it ran and can be configured to fail.
type fakeAdapter struct {
	experiment string

	failStage harnessruntime.Stage
	failErr   error

	cleanupErr error

	calls []harnessruntime.Stage
}

func (f *fakeAdapter) ExperimentName() string { return f.experiment }

func (f *fakeAdapter) stage(stage harnessruntime.Stage) (harnessruntime.StageResult, error) {
	f.calls = append(f.calls, stage)
	if f.failStage == stage {
		return harnessruntime.StageResult{Stage: stage}, f.failErr
	}
	return harnessruntime.StageResult{Stage: stage}, nil
}

func (f *fakeAdapter) Prepare(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return f.stage(harnessruntime.StagePrepare)
}

func (f *fakeAdapter) CreateTask(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return f.stage(harnessruntime.StageCreate)
}

func (f *fakeAdapter) StartTask(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return f.stage(harnessruntime.StageStart)
}

func (f *fakeAdapter) WaitReady(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return f.stage(harnessruntime.StageWaitReady)
}

func (f *fakeAdapter) Stop(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return f.stage(harnessruntime.StageStop)
}

func (f *fakeAdapter) DeleteTask(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return f.stage(harnessruntime.StageDelete)
}

func (f *fakeAdapter) Cleanup(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	f.calls = append(f.calls, harnessruntime.StageCleanup)
	if f.failStage == harnessruntime.StageCleanup {
		return harnessruntime.StageResult{Stage: harnessruntime.StageCleanup}, f.failErr
	}
	if f.cleanupErr != nil {
		return harnessruntime.StageResult{Stage: harnessruntime.StageCleanup}, f.cleanupErr
	}
	return harnessruntime.StageResult{Stage: harnessruntime.StageCleanup}, nil
}

func (f *fakeAdapter) GenerateResult(ctx context.Context, tc harnessruntime.TrialContext, results []harnessruntime.StageResult) (any, error) {
	return nil, nil
}

func countStage(calls []harnessruntime.Stage, stage harnessruntime.Stage) int {
	n := 0
	for _, c := range calls {
		if c == stage {
			n++
		}
	}
	return n
}

func trial(experiment string) plan.Trial {
	return plan.Trial{
		ID:             "trial-1",
		ExperimentName: experiment,
		RuntimeName:    "urunc",
	}
}

// TestRunTrialCleansUpAfterStageFailure verifies that when a stage fails
// after Prepare has already created resources, the orchestrator still calls
// Cleanup on the adapter before returning the (unmasked) original error.
// This is the regression test for issue #9: a failed trial used to leak
// containers/VMs/netns because Cleanup was only reachable via the normal
// (all-stages-succeed) path.
func TestRunTrialCleansUpAfterStageFailure(t *testing.T) {
	stagesToFail := []harnessruntime.Stage{
		harnessruntime.StageCreate,
		harnessruntime.StageStart,
		harnessruntime.StageWaitReady,
		harnessruntime.StageStop,
		harnessruntime.StageDelete,
	}

	for _, failStage := range stagesToFail {
		failStage := failStage
		t.Run(string(failStage), func(t *testing.T) {
			wantErr := errors.New("boom")
			adapter := &fakeAdapter{experiment: "lifecycle", failStage: failStage, failErr: wantErr}

			o := New(func(plan.Trial) (harnessruntime.Adapter, error) { return adapter, nil })

			result := o.runTrial(context.Background(), trial("lifecycle"), Options{})

			if result.Status != TrialStatusFailed {
				t.Fatalf("status = %s, want %s", result.Status, TrialStatusFailed)
			}
			if result.FailedStage != failStage {
				t.Fatalf("failedStage = %s, want %s", result.FailedStage, failStage)
			}
			if result.Error != wantErr.Error() {
				t.Fatalf("error = %q, want %q", result.Error, wantErr.Error())
			}
			if countStage(adapter.calls, harnessruntime.StageCleanup) != 1 {
				t.Fatalf("Cleanup calls = %d, want 1 (calls: %v)", countStage(adapter.calls, harnessruntime.StageCleanup), adapter.calls)
			}
		})
	}
}

// TestRunTrialSkipsCleanupWhenPrepareFails verifies that Cleanup is not
// invoked when Prepare itself fails, since no resources were created yet
// (calling Cleanup in that state can be unsafe for adapters that assume
// Prepare has already initialized adapter state).
func TestRunTrialSkipsCleanupWhenPrepareFails(t *testing.T) {
	wantErr := errors.New("prepare failed")
	adapter := &fakeAdapter{experiment: "lifecycle", failStage: harnessruntime.StagePrepare, failErr: wantErr}

	o := New(func(plan.Trial) (harnessruntime.Adapter, error) { return adapter, nil })

	result := o.runTrial(context.Background(), trial("lifecycle"), Options{})

	if result.Status != TrialStatusFailed {
		t.Fatalf("status = %s, want %s", result.Status, TrialStatusFailed)
	}
	if countStage(adapter.calls, harnessruntime.StageCleanup) != 0 {
		t.Fatalf("Cleanup calls = %d, want 0 (calls: %v)", countStage(adapter.calls, harnessruntime.StageCleanup), adapter.calls)
	}
}

// TestRunTrialCleanupFailureDoesNotMaskOriginalError verifies that a
// Cleanup failure is surfaced only via logging: the trial's reported error
// and failed stage must still reflect the original stage failure.
func TestRunTrialCleanupFailureDoesNotMaskOriginalError(t *testing.T) {
	wantErr := errors.New("create failed")
	adapter := &fakeAdapter{
		experiment: "lifecycle",
		failStage:  harnessruntime.StageCreate,
		failErr:    wantErr,
		cleanupErr: errors.New("cleanup also failed"),
	}

	o := New(func(plan.Trial) (harnessruntime.Adapter, error) { return adapter, nil })

	result := o.runTrial(context.Background(), trial("lifecycle"), Options{})

	if result.FailedStage != harnessruntime.StageCreate {
		t.Fatalf("failedStage = %s, want %s", result.FailedStage, harnessruntime.StageCreate)
	}
	if result.Error != wantErr.Error() {
		t.Fatalf("error = %q, want original stage error %q (must not be masked by cleanup failure)", result.Error, wantErr.Error())
	}
	if countStage(adapter.calls, harnessruntime.StageCleanup) != 1 {
		t.Fatalf("Cleanup calls = %d, want 1", countStage(adapter.calls, harnessruntime.StageCleanup))
	}
}

// TestRunTrialCallsCleanupExactlyOnceOnSuccess verifies the happy path is
// unchanged: Cleanup runs once, as the final regular stage.
func TestRunTrialCallsCleanupExactlyOnceOnSuccess(t *testing.T) {
	adapter := &fakeAdapter{experiment: "lifecycle"}

	o := New(func(plan.Trial) (harnessruntime.Adapter, error) { return adapter, nil })

	result := o.runTrial(context.Background(), trial("lifecycle"), Options{})

	if result.Status != TrialStatusSuccess {
		t.Fatalf("status = %s, want %s", result.Status, TrialStatusSuccess)
	}
	if countStage(adapter.calls, harnessruntime.StageCleanup) != 1 {
		t.Fatalf("Cleanup calls = %d, want 1 (calls: %v)", countStage(adapter.calls, harnessruntime.StageCleanup), adapter.calls)
	}
}

// TestRunTrialSkipsAdapterForOtherExperiments verifies adapters whose
// ExperimentName doesn't match the trial are never invoked.
func TestRunTrialSkipsAdapterForOtherExperiments(t *testing.T) {
	adapter := &fakeAdapter{experiment: "storage"}

	o := New(func(plan.Trial) (harnessruntime.Adapter, error) { return adapter, nil })

	result := o.runTrial(context.Background(), trial("lifecycle"), Options{})

	if result.Status != TrialStatusSuccess {
		t.Fatalf("status = %s, want %s", result.Status, TrialStatusSuccess)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("calls = %v, want none", adapter.calls)
	}
}
