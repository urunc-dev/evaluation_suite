package doctor

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/urunc-dev/evaluation_suite/internal/environment"
	"github.com/urunc-dev/evaluation_suite/internal/manifest"
)

func Run(
	ctx context.Context,
	m *manifest.Manifest,
	opts Options,
) (*Report, error) {
	report := NewReport()

	checkManifest(report, m)
	checkResultsDir(report, opts.ResultsDir)
	checkCgroupV2(report)
	checkContainerdSocket(report, opts.ContainerdSocket)
	CheckContainerdConfig(report, m, opts.ContainerdConfig)

	checkTool(report, "ctr", true)
	checkTool(report, "containerd", false)

	if hasStorageExperiment(m) {
		checkTool(report, "fio", false)
		report.Add(
			StatusWarn,
			"storage:fio",
			"host fio is optional if fio runs inside the benchmark image; image-level verification will happen during runtime execution",
		)
	}

	checkHostPorts(report, m)
	checkHostPathVolumes(report, m)
	checkEnvironmentVisibility(ctx, report)

	return report, nil
}

func checkManifest(report *Report, m *manifest.Manifest) {
	if err := manifest.Validate(m); err != nil {
		report.Add(StatusFail, "manifest", err.Error())
		return
	}

	report.Add(StatusPass, "manifest", "manifest parsed and validated")
}

func checkResultsDir(report *Report, resultsDir string) {
	if resultsDir == "" {
		resultsDir = "results"
	}

	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		report.Add(StatusFail, "results-dir", fmt.Sprintf("cannot create results directory %q: %v", resultsDir, err))
		return
	}

	tmp, err := os.CreateTemp(resultsDir, ".doctor-*")
	if err != nil {
		report.Add(StatusFail, "results-dir", fmt.Sprintf("results directory %q is not writable: %v", resultsDir, err))
		return
	}

	tmpName := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(tmpName)

	report.Add(StatusPass, "results-dir", fmt.Sprintf("results directory %q is writable", resultsDir))
}

func checkCgroupV2(report *Report) {
	const cgroup2SuperMagic = 0x63677270

	var stat syscall.Statfs_t
	if err := syscall.Statfs("/sys/fs/cgroup", &stat); err != nil {
		report.Add(StatusFail, "cgroup-v2", fmt.Sprintf("cannot stat /sys/fs/cgroup: %v", err))
		return
	}

	if stat.Type != cgroup2SuperMagic {
		report.Add(StatusFail, "cgroup-v2", "/sys/fs/cgroup is not mounted as cgroup v2")
		return
	}

	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err != nil {
		report.Add(StatusFail, "cgroup-v2", fmt.Sprintf("cgroup.controllers not readable: %v", err))
		return
	}

	report.Add(StatusPass, "cgroup-v2", "cgroup v2 is mounted")
}

func checkContainerdSocket(report *Report, socketPath string) {
	if socketPath == "" {
		socketPath = "/run/containerd/containerd.sock"
	}

	info, err := os.Stat(socketPath)
	if err != nil {
		report.Add(StatusFail, "containerd-socket", fmt.Sprintf("containerd socket %q not found: %v", socketPath, err))
		return
	}

	if info.Mode()&os.ModeSocket == 0 {
		report.Add(StatusFail, "containerd-socket", fmt.Sprintf("%q exists but is not a Unix socket", socketPath))
		return
	}

	report.Add(StatusPass, "containerd-socket", fmt.Sprintf("containerd socket found at %s", socketPath))
}

func checkTool(report *Report, name string, required bool) {
	path, err := exec.LookPath(name)
	if err != nil {
		if required {
			report.Add(StatusFail, "tool:"+name, fmt.Sprintf("%s not found in PATH", name))
			return
		}

		report.Add(StatusWarn, "tool:"+name, fmt.Sprintf("%s not found in PATH", name))
		return
	}

	report.Add(StatusPass, "tool:"+name, fmt.Sprintf("%s found at %s", name, path))
}

func checkHostPorts(report *Report, m *manifest.Manifest) {
	seen := map[int]string{}

	for expName, exp := range m.Experiments {
		checkWorkloadPort := func(workloadPath string, w manifest.Workload) {
			if w.Ports == nil {
				return
			}

			hostPort := w.Ports.HostPort
			if hostPort == 0 {
				return
			}

			if previous, ok := seen[hostPort]; ok {
				report.Add(
					StatusFail,
					"port:"+fmt.Sprint(hostPort),
					fmt.Sprintf("host port %d is used more than once: %s and %s", hostPort, previous, workloadPath),
				)
				return
			}

			seen[hostPort] = workloadPath

			address := fmt.Sprintf("127.0.0.1:%d", hostPort)
			listener, err := net.Listen("tcp", address)
			if err != nil {
				report.Add(
					StatusFail,
					"port:"+fmt.Sprint(hostPort),
					fmt.Sprintf("host port %d is not available: %v", hostPort, err),
				)
				return
			}

			_ = listener.Close()

			report.Add(
				StatusPass,
				"port:"+fmt.Sprint(hostPort),
				fmt.Sprintf("host port %d is available for %s", hostPort, workloadPath),
			)
		}

		checkWorkloadPort("experiments."+expName+".workloads.default", exp.Workloads.Default)

		for i, workload := range exp.Workloads.Other {
			checkWorkloadPort(
				fmt.Sprintf("experiments.%s.workloads.other[%d]", expName, i),
				workload,
			)
		}
	}
}

func checkHostPathVolumes(report *Report, m *manifest.Manifest) {
	for expName, exp := range m.Experiments {
		checkWorkloadVolumes := func(workloadPath string, w manifest.Workload) {
			for i, volume := range w.Volumes {
				volumePath := fmt.Sprintf("%s.volumes[%d]", workloadPath, i)

				if volume.Type != "hostPath" {
					continue
				}

				if volume.Source == "" {
					report.Add(
						StatusFail,
						"volume:"+volumePath,
						"hostPath volume requires source",
					)
					continue
				}

				source := volume.Source

				if err := os.MkdirAll(source, 0o755); err != nil {
					report.Add(
						StatusFail,
						"volume:"+volumePath,
						fmt.Sprintf("cannot create hostPath source %q: %v", source, err),
					)
					continue
				}

				info, err := os.Stat(source)
				if err != nil {
					report.Add(
						StatusFail,
						"volume:"+volumePath,
						fmt.Sprintf("cannot stat hostPath source %q: %v", source, err),
					)
					continue
				}

				if !info.IsDir() {
					report.Add(
						StatusFail,
						"volume:"+volumePath,
						fmt.Sprintf("hostPath source %q is not a directory", source),
					)
					continue
				}

				report.Add(
					StatusPass,
					"volume:"+volumePath,
					fmt.Sprintf("hostPath source %q exists and is a directory", source),
				)
			}
		}

		checkWorkloadVolumes("experiments."+expName+".workloads.default", exp.Workloads.Default)

		for i, workload := range exp.Workloads.Other {
			checkWorkloadVolumes(
				fmt.Sprintf("experiments.%s.workloads.other[%d]", expName, i),
				workload,
			)
		}
	}
}

func checkEnvironmentVisibility(ctx context.Context, report *Report) {
	env, err := environment.Capture(ctx)
	if err != nil {
		report.Add(StatusWarn, "environment", fmt.Sprintf("environment capture failed: %v", err))
		return
	}

	if len(env.CPU.Governors) == 0 {
		report.Add(StatusWarn, "cpu-governor", "CPU governor files were not visible")
	} else {
		report.Add(StatusPass, "cpu-governor", fmt.Sprintf("captured governors for %d CPUs", len(env.CPU.Governors)))
	}

	if env.CPU.Turbo.Status == "" || env.CPU.Turbo.Status == "unknown" {
		report.Add(StatusWarn, "turbo", "turbo/boost status is unknown")
	} else {
		report.Add(StatusPass, "turbo", "turbo/boost status captured: "+env.CPU.Turbo.Status)
	}

	if env.CPU.SMT.Active == "" && env.CPU.SMT.Control == "" {
		report.Add(StatusWarn, "smt", "SMT status is not visible")
	} else {
		report.Add(StatusPass, "smt", fmt.Sprintf("SMT active=%s control=%s", env.CPU.SMT.Active, env.CPU.SMT.Control))
	}

	if env.NUMA.OnlineNodes == "" {
		report.Add(StatusWarn, "numa", "NUMA online nodes are not visible")
	} else {
		report.Add(StatusPass, "numa", "NUMA online nodes: "+env.NUMA.OnlineNodes)
	}

	if env.Swap.Enabled {
		report.Add(StatusWarn, "swap", "swap is enabled")
	} else {
		report.Add(StatusPass, "swap", "swap is disabled")
	}
}

func hasStorageExperiment(m *manifest.Manifest) bool {
	for name := range m.Experiments {
		if strings.EqualFold(name, "storage") {
			return true
		}
	}

	return false
}
