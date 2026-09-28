package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/containerd/containerd"
	harnessruntime "github.com/urunc-dev/evaluation_suite/internal/runtime"
)

var processArgs = []string{
	"--name=test-runtime",
	"--rw=write",
	"--bs=4k",
	"--size=256M",
	"--runtime=30",
	"--time_based",
	"--output-format=json",
}

type Adapter struct {
	ContainerdClient    *containerd.Client
	ContainerdNamespace *context.Context
	Container           containerd.Container
	Task                containerd.Task
	TaskExitCh          <-chan containerd.ExitStatus
	StdoutBuffer        bytes.Buffer
	StderrBuffer        bytes.Buffer
}

func NewAdapter(containerdClient *containerd.Client, containerdNamespace *context.Context) *Adapter {
	return &Adapter{
		ContainerdClient:    containerdClient,
		ContainerdNamespace: containerdNamespace,
	}
}

func (a *Adapter) ExperimentName() string {
	return "storage"
}

func (a *Adapter) Prepare(
	ctx context.Context,
	tc harnessruntime.TrialContext,
) (harnessruntime.StageResult, error) {

	return fakeStage(context.Background(), harnessruntime.StagePrepare, "would prepare the trial", tc)
}

func (a *Adapter) CreateTask(
	ctx context.Context,
	tc harnessruntime.TrialContext,
) (harnessruntime.StageResult, error) {

	return fakeStage(context.Background(), harnessruntime.StageCreate, "would create the task", tc)
}

func (a *Adapter) StartTask(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
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
		return harnessruntime.StageResult{}, fmt.Errorf(
			"start task with nerdctl: %w\nstdout: %s\nstderr: %s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}

	finishedAt := time.Now()

	fioJSON, err := extractFIOJSON(stdout.String())
	if err != nil {
		return harnessruntime.StageResult{}, fmt.Errorf(
			"extract fio output: %w; raw stdout=%q",
			err,
			stdout.String(),
		)
	}

	// Validate that fio actually returned valid JSON.
	var fioResult map[string]interface{}
	if err := json.Unmarshal([]byte(fioJSON), &fioResult); err != nil {
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

func (a *Adapter) Stop(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return fakeStage(context.Background(), harnessruntime.StageStop, "would stop the task", tc)
}

func (a *Adapter) DeleteTask(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return fakeStage(context.Background(), harnessruntime.StageDelete, "would delete the task", tc)
}

func (a *Adapter) Cleanup(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	// delete the bundle directory
	return fakeStage(context.Background(), harnessruntime.StageCleanup, "would audit and clean runtime leftovers", tc)
}

func (a *Adapter) WaitReady(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return fakeStage(ctx, harnessruntime.StageWaitReady, "would wait for READY event or health endpoint", tc)
}

func fakeStage(
	ctx context.Context,
	stage harnessruntime.Stage,
	description string,
	tc harnessruntime.TrialContext,
) (harnessruntime.StageResult, error) {
	startedAt := time.Now()

	select {
	case <-ctx.Done():
		return harnessruntime.StageResult{}, ctx.Err()
	case <-time.After(1 * time.Millisecond):
	}

	finishedAt := time.Now()

	return harnessruntime.StageResult{
		Stage:      stage,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(startedAt),
		Description: fmt.Sprintf(
			"%s: trial=%s runtime=%s handler=%s image=%s",
			description,
			tc.Trial.ID,
			tc.Trial.RuntimeName,
			tc.Trial.RuntimeHandler,
			tc.Trial.Image,
		),
	}, nil
}

func extractFIOJSON(stdout string) (string, error) {
	// SeaBIOS and boot output occur before the JSON.
	jsonStart := strings.IndexByte(stdout, '{')
	if jsonStart == -1 {
		return "", fmt.Errorf("fio JSON start not found in stdout")
	}

	decoder := json.NewDecoder(strings.NewReader(stdout[jsonStart:]))

	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return "", fmt.Errorf("decode fio JSON: %w", err)
	}

	// Confirm that what we extracted is actually fio output.
	var header struct {
		FIOVersion string `json:"fio version"`
		Jobs       []struct {
			JobName string `json:"jobname"`
			Error   int    `json:"error"`
		} `json:"jobs"`
	}

	if err := json.Unmarshal(raw, &header); err != nil {
		return "", fmt.Errorf("validate fio JSON: %w", err)
	}

	if header.FIOVersion == "" {
		return "", fmt.Errorf("extracted JSON is not fio output: missing fio version")
	}

	if len(header.Jobs) == 0 {
		return "", fmt.Errorf("fio JSON contains no jobs")
	}

	// Return a clean, consistently formatted JSON byte slice.
	var cleaned bytes.Buffer
	if err := json.Indent(&cleaned, raw, "", "  "); err != nil {
		return "", fmt.Errorf("format fio JSON: %w", err)
	}

	return cleaned.String(), nil
}

func (a *Adapter) GenerateResult(ctx context.Context, tc harnessruntime.TrialContext, results []harnessruntime.StageResult) (any, error) {
	// return the first Start stage result that contains the fio JSON output
	for _, result := range results {
		if result.Stage == harnessruntime.StageStart {
			return result.Data, nil
		}
	}

	return nil, fmt.Errorf("no Start stage result found for trial %s", tc.Trial.ID)
}
