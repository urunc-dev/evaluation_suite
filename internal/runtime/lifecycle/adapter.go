package lifecycle

import (
	"context"
	"fmt"
	"log"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	apievents "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/typeurl/v2"
	harnessruntime "github.com/urunc-dev/evaluation_suite/internal/runtime"
)

type Adapter struct {
	ContainerdClient    *containerd.Client
	ContainerdNamespace *context.Context
	Container           containerd.Container
	Task                containerd.Task
	TaskExitCh          <-chan containerd.ExitStatus
}

func NewAdapter(containerdClient *containerd.Client, containerdNamespace *context.Context) *Adapter {
	return &Adapter{
		ContainerdClient:    containerdClient,
		ContainerdNamespace: containerdNamespace,
	}
}

func (a *Adapter) ExperimentName() string {
	return "lifecycle"
}

func (a *Adapter) Prepare(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {

	log.Printf("LIFECYCLE: Preparing trial %s with runtime %s and handler %s and snapshotter %s and image %s", tc.Trial.ID, tc.Trial.RuntimeName, tc.Trial.RuntimeHandler, tc.Trial.Snapshotter, tc.Trial.Image)

	if tc.Trial.RuntimeName == "runsc" {
		return a.CLIPrepare(tc)
	}

	startedAt := time.Now()

	// image
	image, err := a.ContainerdClient.GetImage(*a.ContainerdNamespace, tc.Trial.Image)
	if err != nil {
		log.Fatal(err)
	}
	// create container metadata
	container, err := a.ContainerdClient.NewContainer(
		*a.ContainerdNamespace,
		tc.Trial.ID,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(tc.Trial.ID+"-snapshot", image),
		containerd.WithRuntime(tc.Trial.RuntimeHandler, nil),
		containerd.WithNewSpec(
			oci.WithImageConfig(image),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	a.Container = container

	finishedAt := time.Now()

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
	// subscribe to events

	eventCh, errCh := a.ContainerdClient.Subscribe(*a.ContainerdNamespace)

	var latency time.Duration

	var eventTime time.Time

	startedAt := time.Now()

	task, err := a.Container.NewTask(*a.ContainerdNamespace, cio.NullIO)
	if err != nil {
		log.Fatal(err)
	}
	a.Task = task

	exitCh, err := task.Wait(*a.ContainerdNamespace)
	if err != nil {
		log.Fatal(err)
	}

	a.TaskExitCh = exitCh

	for {
		select {
		case err := <-errCh:
			if err != nil {
				log.Fatal(err)
			}

		case envelope := <-eventCh:
			if envelope == nil {
				continue
			}

			if envelope.Topic != "/tasks/create" {
				continue
			}

			event, err := typeurl.UnmarshalAny(envelope.Event)
			if err != nil {
				log.Fatal(err)
			}

			taskCreate, ok := event.(*apievents.TaskCreate)
			if !ok {
				continue
			}

			if taskCreate.ContainerID != tc.Trial.ID {
				continue
			}

			eventTime = envelope.Timestamp

			latency = eventTime.Sub(startedAt)

			return harnessruntime.StageResult{
				Stage:      harnessruntime.StageCreate,
				StartedAt:  startedAt,
				FinishedAt: eventTime,
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
					"end":        eventTime,
					"latency":    latency,
					"latency_ms": float64(latency.Microseconds()) / 1000,
				},
			}, nil

		case <-time.After(60 * time.Second):
			log.Fatal("timed out waiting for /tasks/create event")
		}
	}

}

func (a *Adapter) StartTask(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {

	if tc.Trial.RuntimeName == "runsc" {
		return a.CLIStartTask(tc)
	}

	eventCh, errCh := a.ContainerdClient.Subscribe(*a.ContainerdNamespace)

	var latency time.Duration

	var eventTime time.Time

	var taskName string

	if tc.Trial.RuntimeName == "urunc" {
		// For urunc since it emits the start event earlier than the actual container starts, We measure the latency from the start of the task to the exit event instead of the start event.
		taskName = "/tasks/exit"
	} else {
		taskName = "/tasks/start"
	}

	startedAt := time.Now()

	if err := a.Task.Start(*a.ContainerdNamespace); err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case err := <-errCh:
			if err != nil {
				log.Fatal(err)
			}

		case envelope := <-eventCh:
			if envelope == nil {
				continue
			}

			if envelope.Topic != taskName {
				continue
			}

			event, err := typeurl.UnmarshalAny(envelope.Event)
			if err != nil {
				log.Fatal(err)
			}

			if taskName == "/tasks/start" {
				taskStart, ok := event.(*apievents.TaskStart)
				if !ok {
					continue
				}

				if taskStart.ContainerID != tc.Trial.ID {
					continue
				}
			} else if taskName == "/tasks/exit" {
				taskExit, ok := event.(*apievents.TaskExit)
				if !ok {
					continue
				}

				if taskExit.ContainerID != tc.Trial.ID {
					continue
				}
			}

			eventTime = envelope.Timestamp

			latency = eventTime.Sub(startedAt)

			return harnessruntime.StageResult{
				Stage:      harnessruntime.StageStart,
				StartedAt:  startedAt,
				FinishedAt: eventTime,
				Duration:   latency,
				Description: fmt.Sprintf(
					"%s: trial=%s runtime=%s handler=%s image=%s",
					"Start container task",
					tc.Trial.ID,
					tc.Trial.RuntimeName,
					tc.Trial.RuntimeHandler,
					tc.Trial.Image,
				),
				Data: map[string]interface{}{
					"start":      startedAt,
					"end":        eventTime,
					"latency":    latency,
					"latency_ms": float64(latency.Microseconds()) / 1000,
				},
			}, nil

		case <-time.After(60 * time.Second):
			log.Fatal("timed out waiting for task start event")
		}
	}

}

func (a *Adapter) WaitReady(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	return fakeStage(ctx, harnessruntime.StageWaitReady, "would wait for READY event or health endpoint", tc)
}

func (a *Adapter) Stop(
	ctx context.Context,
	tc harnessruntime.TrialContext,
) (harnessruntime.StageResult, error) {

	if tc.Trial.RuntimeName == "runsc" {
		return a.CLIStopTask(tc)
	}

	startedAt := time.Now()

	status, err := a.Task.Status(*a.ContainerdNamespace)
	if err != nil {
		log.Fatalf(
			"get task status: %w",
			err,
		)
	}

	if status.Status != containerd.Stopped {
		if err := a.Task.Kill(
			*a.ContainerdNamespace,
			syscall.SIGKILL,
		); err != nil {
			log.Fatalf(
				"kill task: %w",
				err,
			)
		}
	}

	if a.TaskExitCh == nil {
		log.Fatalf(
			"task exit waiter was not initialized",
		)
	}

	select {
	case exitStatus := <-a.TaskExitCh:
		exitCode, exitTime, err := exitStatus.Result()
		if err != nil {
			log.Fatalf(
				"read task exit status: %w",
				err,
			)
		}

		finishedAt := time.Now()

		return harnessruntime.StageResult{
			Stage:      harnessruntime.StageStop,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Duration:   finishedAt.Sub(startedAt),
			Description: fmt.Sprintf(
				"Stopped task: trial=%s runtime=%s",
				tc.Trial.ID,
				tc.Trial.RuntimeName,
			),
			Data: map[string]interface{}{
				"exit_code": exitCode,
				"exit_time": exitTime,
			},
		}, nil

	case <-ctx.Done():
		return harnessruntime.StageResult{}, ctx.Err()

	case <-time.After(60 * time.Second):
		log.Fatalf(
			"timed out waiting for task %q to stop",
			tc.Trial.ID,
		)
	}

	return harnessruntime.StageResult{}, nil
}

func (a *Adapter) DeleteTask(ctx context.Context, tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {

	if tc.Trial.RuntimeName == "runsc" {
		return a.CLIDeleteTask(tc)
	}

	eventsCh, errCh := a.ContainerdClient.Subscribe(*a.ContainerdNamespace)

	startedAt := time.Now()

	if _, err := a.Task.Delete(*a.ContainerdNamespace); err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case err := <-errCh:
			if err != nil {
				log.Fatal(err)
			}

		case envelope := <-eventsCh:
			if envelope == nil {
				continue
			}

			if envelope.Topic != "/tasks/delete" {
				continue
			}

			event, err := typeurl.UnmarshalAny(envelope.Event)
			if err != nil {
				log.Fatal(err)
			}

			taskDelete, ok := event.(*apievents.TaskDelete)
			if !ok {
				continue
			}

			if taskDelete.ContainerID != tc.Trial.ID {
				continue
			}

			eventTime := envelope.Timestamp

			latency := eventTime.Sub(startedAt)

			return harnessruntime.StageResult{
				Stage:      harnessruntime.StageDelete,
				StartedAt:  startedAt,
				FinishedAt: eventTime,
				Duration:   latency,
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
					"end":        eventTime,
					"latency":    latency,
					"latency_ms": float64(latency.Microseconds()) / 1000,
				},
			}, nil

		case <-time.After(60 * time.Second):
			log.Fatal("timed out waiting for task delete event")
		}
	}
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
