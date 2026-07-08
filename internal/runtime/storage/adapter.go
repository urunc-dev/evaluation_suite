package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/oci"
	harnessruntime "github.com/urunc-dev/evaluation_suite/internal/runtime"
)

var processArgs = []string{
	"--name=test-runtime",
	"--directory=/bench",
	"--rw=randrw",
	"--bs=4k",
	"--size=512M",
	"--direct=1",
	"--time_based",
	"--runtime=30",
	"--group_reporting",
	"--output-format=json",
}

func getOrPullImage(
	ctx context.Context,
	client *containerd.Client,
	ref string,
	snapshotter string,
) (containerd.Image, error) {
	image, err := client.GetImage(ctx, ref)
	if err == nil {
		if err := image.Unpack(ctx, snapshotter); err != nil &&
			!errdefs.IsAlreadyExists(err) {
			return nil, fmt.Errorf(
				"unpack image %q using snapshotter %q: %w",
				ref,
				snapshotter,
				err,
			)
		}

		return image, nil
	}

	if !errdefs.IsNotFound(err) {
		return nil, fmt.Errorf("get image %q: %w", ref, err)
	}

	image, err = client.Pull(
		ctx,
		ref,
		containerd.WithPullUnpack,
		containerd.WithPullSnapshotter(snapshotter),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"pull image %q using snapshotter %q: %w",
			ref,
			snapshotter,
			err,
		)
	}

	return image, nil
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

func (a *Adapter) Prepare(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {

	if tc.Trial.RuntimeName == "runsc" {
		return a.CLIPrepare(tc)
	}

	log.Printf("STORAGE: Preparing trial %s with runtime %s and handler %s and snapshotter %s and image %s", tc.Trial.ID, tc.Trial.RuntimeName, tc.Trial.RuntimeHandler, tc.Trial.Snapshotter, tc.Trial.Image)
	startedAt := time.Now()

	// image
	image, err := getOrPullImage(*a.ContainerdNamespace, a.ContainerdClient, tc.Trial.Image, tc.Trial.Snapshotter)
	if err != nil {
		log.Fatal(err)
	}
	// create container metadata
	container, err := a.ContainerdClient.NewContainer(
		*a.ContainerdNamespace,
		tc.Trial.ID,
		containerd.WithImage(image),
		containerd.WithSnapshotter(tc.Trial.Snapshotter),
		containerd.WithNewSnapshot(tc.Trial.ID+"-snapshot", image),
		containerd.WithRuntime(tc.Trial.RuntimeHandler, nil),
		containerd.WithNewSpec(
			oci.WithImageConfigArgs(image, processArgs),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	a.Container = container

	finishedAt := time.Now()

	log.Printf("STORAGE: Finished preparing trial %s with runtime %s and handler %s and snapshotter %s and image %s", tc.Trial.ID, tc.Trial.RuntimeName, tc.Trial.RuntimeHandler, tc.Trial.Snapshotter, tc.Trial.Image)

	return harnessruntime.StageResult{
		Stage:      harnessruntime.StagePrepare,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(startedAt),
		Description: fmt.Sprintf(
			"%s: trial=%s runtime=%s handler=%s image=%s",
			"Setup container Metadata",
			tc.Trial.ID,
			tc.Trial.RuntimeName,
			tc.Trial.RuntimeHandler,
			tc.Trial.Image,
		),
		Data: map[string]interface{}{
			"start":   startedAt,
			"end":     finishedAt,
			"latency": finishedAt.Sub(startedAt),
		},
	}, nil
}

func (a *Adapter) CreateTask(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {

	if tc.Trial.RuntimeName == "runsc" {
		return a.CLICreateTask(tc)
	}

	log.Printf("STORAGE: Creating task for trial %s with runtime %s and handler %s and snapshotter %s and image %s", tc.Trial.ID, tc.Trial.RuntimeName, tc.Trial.RuntimeHandler, tc.Trial.Snapshotter, tc.Trial.Image)
	var latency time.Duration

	startedAt := time.Now()

	task, err := a.Container.NewTask(*a.ContainerdNamespace, cio.NewCreator(
		cio.WithStreams(nil, &a.StdoutBuffer, &a.StderrBuffer),
	))
	if err != nil {
		log.Fatal(err)
	}
	a.Task = task

	exitCh, err := task.Wait(*a.ContainerdNamespace)
	if err != nil {
		log.Fatal(err)
	}

	a.TaskExitCh = exitCh

	endTime := time.Now()

	latency = endTime.Sub(startedAt)
	log.Printf("STORAGE: Finished creating task for trial %s with runtime %s and handler %s and snapshotter %s and image %s, latency: %v", tc.Trial.ID, tc.Trial.RuntimeName, tc.Trial.RuntimeHandler, tc.Trial.Snapshotter, tc.Trial.Image, latency)

	return harnessruntime.StageResult{
		Stage:      harnessruntime.StageCreate,
		StartedAt:  startedAt,
		FinishedAt: endTime,
		Duration:   latency,
		Description: fmt.Sprintf(
			"%s: trial=%s runtime=%s handler=%s image=%s",
			"Setup container Metadata",
			tc.Trial.ID,
			tc.Trial.RuntimeName,
			tc.Trial.RuntimeHandler,
			tc.Trial.Image,
		),
		Data: map[string]interface{}{
			"start":      startedAt,
			"end":        endTime,
			"latency":    latency,
			"latency_ms": float64(latency.Microseconds()) / 1000,
		},
	}, nil

}

func (a *Adapter) StartTask(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {

	if tc.Trial.RuntimeName == "runsc" {
		return a.CLIStartTask(tc)
	}

	startedAt := time.Now()

	if err := a.Task.Start(*a.ContainerdNamespace); err != nil {
		log.Fatal(err)
	}

	finishedAt := time.Now()

	var exitCode uint32

	// wait for the task to exit and also get exit code
	select {
	case <-ctx.Done():
		return harnessruntime.StageResult{}, ctx.Err()
	case status := <-a.TaskExitCh:
		exitCodee, _, err := status.Result()
		if err != nil {
			log.Fatal(err)
		}

		exitCode = exitCodee

	}

	fioJSON, err := extractFIOJSON(a.StdoutBuffer.String())
	if err != nil {
		return harnessruntime.StageResult{}, fmt.Errorf(
			"extract fio output: %w; raw stdout=%q",
			err,
			a.StdoutBuffer.String(),
		)
	}

	return harnessruntime.StageResult{
		Stage:      harnessruntime.StageStart,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(startedAt),
		Description: fmt.Sprintf(
			"%s: trial=%s runtime=%s handler=%s image=%s",
			"Start container task",
			tc.Trial.ID,
			tc.Trial.RuntimeName,
			tc.Trial.RuntimeHandler,
			tc.Trial.Image,
		),
		// since the task has exited, we can include the exit code in the data, also incluse the stdout and stderr buffers
		Data: map[string]interface{}{
			"start":      startedAt,
			"end":        finishedAt,
			"latency":    finishedAt.Sub(startedAt),
			"latency_ms": float64(finishedAt.Sub(startedAt).Microseconds()) / 1000,
			"stdout":     string(fioJSON),
			"stderr":     a.StderrBuffer.String(),
			"exit_code":  exitCode,
		},
	}, nil

}

func (a *Adapter) WaitReady(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return fakeStage(ctx, harnessruntime.StageWaitReady, "would wait for READY event or health endpoint", tc)
}

func (a *Adapter) Stop(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return fakeStage(ctx, harnessruntime.StageCleanup, "would audit and clean runtime leftovers", tc)
}

func (a *Adapter) DeleteTask(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	if tc.Trial.RuntimeName == "runsc" {
		return a.CLIDeleteTask(tc)
	}

	startedAt := time.Now()

	if _, err := a.Task.Delete(*a.ContainerdNamespace); err != nil {
		log.Fatal(err)
	}

	finishedAt := time.Now()

	return harnessruntime.StageResult{
		Stage:      harnessruntime.StageDelete,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(startedAt),
		Description: fmt.Sprintf(
			"%s: trial=%s runtime=%s handler=%s image=%s",
			"Delete container task",
			tc.Trial.ID,
			tc.Trial.RuntimeName,
			tc.Trial.RuntimeHandler,
			tc.Trial.Image,
		),
		Data: map[string]interface{}{
			"start":      startedAt,
			"end":        finishedAt,
			"latency":    finishedAt.Sub(startedAt),
			"latency_ms": float64(finishedAt.Sub(startedAt).Microseconds()) / 1000,
		},
	}, nil
}

func (a *Adapter) Cleanup(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	if tc.Trial.RuntimeName == "runsc" {
		return a.CLICleanupTask(tc)
	}
	a.Container.Delete(
		*a.ContainerdNamespace,
		containerd.WithSnapshotCleanup,
	)
	return fakeStage(ctx, harnessruntime.StageCleanup, "would audit and clean runtime leftovers", tc)
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

func extractFIOJSON(stdout string) ([]byte, error) {
	// SeaBIOS and boot output occur before the JSON.
	jsonStart := strings.IndexByte(stdout, '{')
	if jsonStart == -1 {
		return nil, fmt.Errorf("fio JSON start not found in stdout")
	}

	decoder := json.NewDecoder(strings.NewReader(stdout[jsonStart:]))

	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode fio JSON: %w", err)
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
		return nil, fmt.Errorf("validate fio JSON: %w", err)
	}

	if header.FIOVersion == "" {
		return nil, fmt.Errorf("extracted JSON is not fio output: missing fio version")
	}

	if len(header.Jobs) == 0 {
		return nil, fmt.Errorf("fio JSON contains no jobs")
	}

	// Return a clean, consistently formatted JSON byte slice.
	var cleaned bytes.Buffer
	if err := json.Indent(&cleaned, raw, "", "  "); err != nil {
		return nil, fmt.Errorf("format fio JSON: %w", err)
	}

	return cleaned.Bytes(), nil
}
