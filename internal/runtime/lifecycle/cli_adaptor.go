package lifecycle

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	harnessruntime "github.com/urunc-dev/evaluation_suite/internal/runtime"
)

func (a *Adapter) CLIPrepare(tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	startedAt := time.Now()

	imageName := tc.Trial.Image

	cmdskopeo := exec.Command("skopeo", "copy",
		fmt.Sprintf("docker://%s", imageName),
		fmt.Sprintf("oci:%s:bench", tc.Trial.ID),
	)
	cmdskopeo.Stdout = os.Stdout
	cmdskopeo.Stderr = os.Stderr

	log.Printf("Running skopeo command: %s\n", cmdskopeo.String())

	if err := cmdskopeo.Run(); err != nil {
		return harnessruntime.StageResult{}, fmt.Errorf("save image %q to tarball: %w", imageName, err)
	}

	cmdumoci := exec.Command("umoci", "unpack",
		"--image", fmt.Sprintf("%s:bench", tc.Trial.ID),
		fmt.Sprintf("%s-bundle", tc.Trial.ID),
	)
	cmdumoci.Stdout = os.Stdout
	cmdumoci.Stderr = os.Stderr

	log.Printf("Running umoci command: %s\n", cmdumoci.String())

	if err := cmdumoci.Run(); err != nil {
		return harnessruntime.StageResult{}, fmt.Errorf("unpack image %q to bundle: %w", imageName, err)
	}

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

func (a *Adapter) CLICreateTask(tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {

	startedAt := time.Now()

	// BUNDLE PATH: pwd + <trial_id>-bundle
	dir, err := os.Getwd()
	if err != nil {
		return harnessruntime.StageResult{}, fmt.Errorf("get current working directory: %w", err)
	}
	bundlePath := fmt.Sprintf("%s/%s-bundle", dir, tc.Trial.ID)

	cmdrunsc := exec.Command("sudo", tc.Trial.RuntimeName, "create",
		"--bundle", bundlePath,
		fmt.Sprintf("%s-test", tc.Trial.ID),
	)
	cmdrunsc.Stdout = os.Stdout
	cmdrunsc.Stderr = os.Stderr

	log.Printf("Running runsc command: %s\n", cmdrunsc.String())

	if err := cmdrunsc.Run(); err != nil {
		return harnessruntime.StageResult{}, fmt.Errorf("create task %q: %w", tc.Trial.ID, err)
	}

	finishedAt := time.Now()

	return harnessruntime.StageResult{
		Stage:      harnessruntime.StageCreate,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(startedAt),
		Description: fmt.Sprintf(
			"%s: trial=%s runtime=%s handler=%s image=%s",
			"Create task",
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
		},
	}, nil
}

func (a *Adapter) CLIStartTask(tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	startedAt := time.Now()

	cmdrunsc := exec.Command("sudo", tc.Trial.RuntimeName, "start",
		fmt.Sprintf("%s-test", tc.Trial.ID),
	)
	cmdrunsc.Stdout = os.Stdout
	cmdrunsc.Stderr = os.Stderr

	if err := cmdrunsc.Run(); err != nil {
		return harnessruntime.StageResult{}, fmt.Errorf("start task %q: %w", tc.Trial.ID, err)
	}

	finishedAt := time.Now()

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
		},
	}, nil
}

func (a *Adapter) CLIStopTask(tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	startedAt := time.Now()

	cmdrunsc := exec.Command("sudo", tc.Trial.RuntimeName, "kill",
		fmt.Sprintf("%s-test", tc.Trial.ID),
	)
	cmdrunsc.Stdout = os.Stdout
	cmdrunsc.Stderr = os.Stderr

	if err := cmdrunsc.Run(); err != nil {
		log.Printf("failed to stop task %q: %v", tc.Trial.ID, err)
	}

	finishedAt := time.Now()

	return harnessruntime.StageResult{
		Stage:      harnessruntime.StageStop,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(startedAt),
		Description: fmt.Sprintf(
			"%s: trial=%s runtime=%s handler=%s image=%s",
			"Stop task",
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
		},
	}, nil
}

func (a *Adapter) CLIDeleteTask(tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	startedAt := time.Now()

	cmdrunsc := exec.Command("sudo", tc.Trial.RuntimeName, "delete",
		fmt.Sprintf("%s-test", tc.Trial.ID),
	)
	cmdrunsc.Stdout = os.Stdout
	cmdrunsc.Stderr = os.Stderr

	if err := cmdrunsc.Run(); err != nil {
		return harnessruntime.StageResult{}, fmt.Errorf("delete task %q: %w", tc.Trial.ID, err)
	}

	finishedAt := time.Now()

	return harnessruntime.StageResult{
		Stage:      harnessruntime.StageDelete,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(startedAt),
		Description: fmt.Sprintf(
			"%s: trial=%s runtime=%s handler=%s image=%s",
			"Stop task",
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
		},
	}, nil
}

func (a *Adapter) CLICleanupTask(tc harnessruntime.TrialContext) (harnessruntime.StageResult, error) {
	// delete the bundle directory
	startedAt := time.Now()

	bundleDir := fmt.Sprintf("%s-bundle", tc.Trial.ID)
	if err := os.RemoveAll(bundleDir); err != nil {
		return harnessruntime.StageResult{}, fmt.Errorf("remove bundle directory %q: %w", bundleDir, err)
	}

	if err := os.RemoveAll(tc.Trial.ID); err != nil {
		log.Printf("failed to remove bundle directory %q: %v", bundleDir, err)
	}

	finishedAt := time.Now()

	return harnessruntime.StageResult{
		Stage:      harnessruntime.StageCleanup,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(startedAt),
		Description: fmt.Sprintf(
			"%s: trial=%s runtime=%s handler=%s image=%s",
			"Cleanup task",
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
		},
	}, nil
}
