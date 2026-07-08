package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	harnessruntime "github.com/urunc-dev/evaluation_suite/internal/runtime"
)

func (a *Adapter) CLIPrepare(
	tc harnessruntime.TrialContext,
) (harnessruntime.StageResult, error) {

	return fakeStage(context.Background(), harnessruntime.StagePrepare, "would prepare the trial", tc)
}

func (a *Adapter) CLICreateTask(
	tc harnessruntime.TrialContext,
) (harnessruntime.StageResult, error) {

	return fakeStage(context.Background(), harnessruntime.StageCreate, "would create the task", tc)
}

func (a *Adapter) CLIStartTask(tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	startedAt := time.Now()

	cmdArgs := []string{
		"run",
		"--rm",
		"-it",
		fmt.Sprintf("--runtime=%s", tc.Trial.RuntimeHandler),
		tc.Trial.Image,
	}
	cmdArgs = append(cmdArgs, processArgs...)
	cmdNerdctl := exec.Command("nerdctl", cmdArgs...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmdNerdctl.Stdin = os.Stdin
	cmdNerdctl.Stdout = &stdout
	cmdNerdctl.Stderr = &stderr

	log.Printf("Running nerdctl command: %s\n", cmdNerdctl.String())

	if err := cmdNerdctl.Run(); err != nil {
		log.Fatalf("failed to start task with nerdctl: %v", err)
	}

	finishedAt := time.Now()

	fioJSON := stdout.String()

	// Validate that fio actually returned valid JSON.
	var fioResult map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &fioResult); err != nil {
		return harnessruntime.StageResult{}, fmt.Errorf(
			"fio returned invalid JSON: %w\nstdout: %s\nstderr: %s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}

	return harnessruntime.StageResult{
		Stage:      harnessruntime.StageStart,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(startedAt),
		Description: fmt.Sprintf(
			"%s: trial=%s runtime=%s handler=%s image=%s",
			"Start task",
			tc.Trial.ID,
			tc.Trial.RuntimeName,
			tc.Trial.RuntimeHandler,
			tc.Trial.Image,
		),
		Data: map[string]interface{}{
			"start":      startedAt,
			"end":        finishedAt,
			"latency":    finishedAt.Sub(startedAt),
			"latency_ms": finishedAt.Sub(startedAt).Milliseconds(),
			"stdout":     fioJSON,
			"stderr":     stderr.String(),
		},
	}, nil
}

func (a *Adapter) CLIStopTask(tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return fakeStage(context.Background(), harnessruntime.StageStop, "would stop the task", tc)
}

func (a *Adapter) CLIDeleteTask(tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return fakeStage(context.Background(), harnessruntime.StageDelete, "would delete the task", tc)
}

func (a *Adapter) CLICleanupTask(tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	// delete the bundle directory
	return fakeStage(context.Background(), harnessruntime.StageCleanup, "would audit and clean runtime leftovers", tc)
}
