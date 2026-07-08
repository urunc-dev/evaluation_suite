package environment

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"
)

type Environment struct {
	Schema     string            `json:"schema"`
	CapturedAt time.Time         `json:"capturedAt"`
	Hostname   string            `json:"hostname"`
	Go         GoInfo            `json:"go"`
	OS         OSInfo            `json:"os"`
	Kernel     KernelInfo        `json:"kernel"`
	CPU        CPUInfo           `json:"cpu"`
	Memory     MemoryInfo        `json:"memory"`
	NUMA       NUMAInfo          `json:"numa"`
	Swap       SwapInfo          `json:"swap"`
	Tools      map[string]string `json:"tools,omitempty"`
	Warnings   []string          `json:"warnings,omitempty"`
}

type GoInfo struct {
	Version string `json:"version"`
	GOOS    string `json:"goos"`
	GOARCH  string `json:"goarch"`
}

type OSInfo struct {
	Release map[string]string `json:"release,omitempty"`
}

type KernelInfo struct {
	Uname      string            `json:"uname,omitempty"`
	Cmdline    string            `json:"cmdline,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

type CPUInfo struct {
	ModelName    string            `json:"modelName,omitempty"`
	Architecture string            `json:"architecture,omitempty"`
	OnlineCPUs   string            `json:"onlineCpus,omitempty"`
	PossibleCPUs string            `json:"possibleCpus,omitempty"`
	PresentCPUs  string            `json:"presentCpus,omitempty"`
	Governors    map[string]string `json:"governors,omitempty"`
	Turbo        TurboInfo         `json:"turbo"`
	SMT          SMTInfo           `json:"smt"`
	IsolatedCPUs string            `json:"isolatedCpus,omitempty"`
	NoHzFullCPUs string            `json:"noHzFullCpus,omitempty"`
	RcuNoCbsCPUs string            `json:"rcuNoCbsCpus,omitempty"`
}

type TurboInfo struct {
	IntelNoTurbo string `json:"intelNoTurbo,omitempty"`
	AMDCPB       string `json:"amdCpb,omitempty"`
	Status       string `json:"status,omitempty"`
}

type SMTInfo struct {
	Active  string `json:"active,omitempty"`
	Control string `json:"control,omitempty"`
}

type MemoryInfo struct {
	MemTotalKB     uint64 `json:"memTotalKb,omitempty"`
	MemAvailableKB uint64 `json:"memAvailableKb,omitempty"`
	SwapTotalKB    uint64 `json:"swapTotalKb,omitempty"`
	SwapFreeKB     uint64 `json:"swapFreeKb,omitempty"`
}

type NUMAInfo struct {
	OnlineNodes   string            `json:"onlineNodes,omitempty"`
	PossibleNodes string            `json:"possibleNodes,omitempty"`
	NodeCPUs      map[string]string `json:"nodeCpus,omitempty"`
}

type SwapInfo struct {
	Enabled bool         `json:"enabled"`
	Devices []SwapDevice `json:"devices,omitempty"`
}

type SwapDevice struct {
	Filename string `json:"filename"`
	Type     string `json:"type"`
	SizeKB   uint64 `json:"sizeKb"`
	UsedKB   uint64 `json:"usedKb"`
	Priority int    `json:"priority"`
}

func Capture(ctx context.Context) (*Environment, error) {
	var warnings []string

	hostname, err := os.Hostname()
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("read hostname: %v", err))
	}

	env := &Environment{
		Schema:     "environment.v1alpha1",
		CapturedAt: time.Now().UTC(),
		Hostname:   hostname,
		Go: GoInfo{
			Version: goruntime.Version(),
			GOOS:    goruntime.GOOS,
			GOARCH:  goruntime.GOARCH,
		},
		Tools:    map[string]string{},
		Warnings: warnings,
	}

	env.OS = captureOS(&env.Warnings)
	env.Kernel = captureKernel(ctx, &env.Warnings)
	env.CPU = captureCPU(ctx, &env.Warnings)
	env.Memory = captureMemory(&env.Warnings)
	env.NUMA = captureNUMA(&env.Warnings)
	env.Swap = captureSwap(&env.Warnings)
	env.Tools = captureTools(ctx, &env.Warnings)

	return env, nil
}

func captureOS(warnings *[]string) OSInfo {
	release, err := parseKeyValueFile("/etc/os-release")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("read /etc/os-release: %v", err))
	}

	return OSInfo{
		Release: release,
	}
}

func captureKernel(ctx context.Context, warnings *[]string) KernelInfo {
	uname, err := commandOutput(ctx, "uname", "-a")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("run uname -a: %v", err))
	}

	cmdline, err := readTrim("/proc/cmdline")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("read /proc/cmdline: %v", err))
	}

	return KernelInfo{
		Uname:      uname,
		Cmdline:    cmdline,
		Parameters: parseKernelCmdline(cmdline),
	}
}

func captureCPU(ctx context.Context, warnings *[]string) CPUInfo {
	modelName, architecture := parseCPUInfo(warnings)

	online, err := readTrim("/sys/devices/system/cpu/online")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("read online CPUs: %v", err))
	}

	possible, err := readTrim("/sys/devices/system/cpu/possible")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("read possible CPUs: %v", err))
	}

	present, err := readTrim("/sys/devices/system/cpu/present")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("read present CPUs: %v", err))
	}

	isolated, _ := readTrim("/sys/devices/system/cpu/isolated")
	nohzFull := kernelParamValue("/proc/cmdline", "nohz_full")
	rcuNoCbs := kernelParamValue("/proc/cmdline", "rcu_nocbs")

	return CPUInfo{
		ModelName:    modelName,
		Architecture: architecture,
		OnlineCPUs:   online,
		PossibleCPUs: possible,
		PresentCPUs:  present,
		Governors:    captureCPUGovernors(warnings),
		Turbo:        captureTurbo(warnings),
		SMT:          captureSMT(warnings),
		IsolatedCPUs: isolated,
		NoHzFullCPUs: nohzFull,
		RcuNoCbsCPUs: rcuNoCbs,
	}
}

func captureCPUGovernors(warnings *[]string) map[string]string {
	paths, err := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_governor")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("glob CPU governors: %v", err))
		return nil
	}

	out := map[string]string{}

	for _, path := range paths {
		value, err := readTrim(path)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("read %s: %v", path, err))
			continue
		}

		cpu := filepath.Base(filepath.Dir(filepath.Dir(path)))
		out[cpu] = value
	}

	if len(out) == 0 {
		*warnings = append(*warnings, "no CPU governor files found; CPU frequency scaling may be unavailable or hidden")
	}

	return out
}

func captureTurbo(warnings *[]string) TurboInfo {
	intelNoTurbo, intelErr := readTrim("/sys/devices/system/cpu/intel_pstate/no_turbo")
	amdCPB, amdErr := readTrim("/sys/devices/system/cpu/cpufreq/boost")

	info := TurboInfo{
		IntelNoTurbo: intelNoTurbo,
		AMDCPB:       amdCPB,
	}

	switch {
	case intelErr == nil:
		if intelNoTurbo == "1" {
			info.Status = "disabled_or_noturbo"
		} else if intelNoTurbo == "0" {
			info.Status = "enabled"
		}
	case amdErr == nil:
		if amdCPB == "1" {
			info.Status = "enabled"
		} else if amdCPB == "0" {
			info.Status = "disabled"
		}
	default:
		info.Status = "unknown"
		*warnings = append(*warnings, "turbo/boost status unavailable; neither intel_pstate/no_turbo nor cpufreq/boost was readable")
	}

	return info
}

func captureSMT(warnings *[]string) SMTInfo {
	active, err := readTrim("/sys/devices/system/cpu/smt/active")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("read SMT active: %v", err))
	}

	control, err := readTrim("/sys/devices/system/cpu/smt/control")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("read SMT control: %v", err))
	}

	return SMTInfo{
		Active:  active,
		Control: control,
	}
}

func captureMemory(warnings *[]string) MemoryInfo {
	values, err := parseMemInfo("/proc/meminfo")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("read /proc/meminfo: %v", err))
	}

	return MemoryInfo{
		MemTotalKB:     values["MemTotal"],
		MemAvailableKB: values["MemAvailable"],
		SwapTotalKB:    values["SwapTotal"],
		SwapFreeKB:     values["SwapFree"],
	}
}

func captureNUMA(warnings *[]string) NUMAInfo {
	online, err := readTrim("/sys/devices/system/node/online")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("read NUMA online nodes: %v", err))
	}

	possible, err := readTrim("/sys/devices/system/node/possible")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("read NUMA possible nodes: %v", err))
	}

	nodeCPUs := map[string]string{}

	nodes, err := filepath.Glob("/sys/devices/system/node/node[0-9]*")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("glob NUMA nodes: %v", err))
	}

	for _, nodePath := range nodes {
		cpulistPath := filepath.Join(nodePath, "cpulist")
		cpulist, err := readTrim(cpulistPath)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("read %s: %v", cpulistPath, err))
			continue
		}

		nodeCPUs[filepath.Base(nodePath)] = cpulist
	}

	return NUMAInfo{
		OnlineNodes:   online,
		PossibleNodes: possible,
		NodeCPUs:      nodeCPUs,
	}
}

func captureSwap(warnings *[]string) SwapInfo {
	devices, err := parseProcSwaps("/proc/swaps")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("read /proc/swaps: %v", err))
	}

	return SwapInfo{
		Enabled: len(devices) > 0,
		Devices: devices,
	}
}

func captureTools(ctx context.Context, warnings *[]string) map[string]string {
	tools := []string{
		"containerd",
		"ctr",
		"nerdctl",
		"runc",
		"runsc",
		"kata-runtime",
		"urunc",
		"qemu-system-x86_64",
		"firecracker",
	}

	out := map[string]string{}

	for _, tool := range tools {
		path, err := exec.LookPath(tool)
		if err != nil {
			continue
		}

		version, err := commandOutput(ctx, tool, "--version")
		if err != nil {
			version = fmt.Sprintf("found at %s, but --version failed: %v", path, err)
		}

		out[tool] = version
	}

	if len(out) == 0 {
		*warnings = append(*warnings, "no known runtime/container tools found in PATH")
	}

	return out
}

func parseCPUInfo(warnings *[]string) (modelName string, architecture string) {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("read /proc/cpuinfo: %v", err))
		return "", ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "model name":
			if modelName == "" {
				modelName = value
			}
		case "Processor":
			if modelName == "" {
				modelName = value
			}
		case "CPU architecture":
			if architecture == "" {
				architecture = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		*warnings = append(*warnings, fmt.Sprintf("scan /proc/cpuinfo: %v", err))
	}

	return modelName, architecture
}

func parseMemInfo(path string) (map[string]uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	values := map[string]uint64{}

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}

		value, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}

		values[key] = value
	}

	return values, scanner.Err()
}

func parseProcSwaps(path string) ([]SwapDevice, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var devices []SwapDevice

	scanner := bufio.NewScanner(f)
	first := true

	for scanner.Scan() {
		line := scanner.Text()

		if first {
			first = false
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		size, _ := strconv.ParseUint(fields[2], 10, 64)
		used, _ := strconv.ParseUint(fields[3], 10, 64)
		priority, _ := strconv.Atoi(fields[4])

		devices = append(devices, SwapDevice{
			Filename: fields[0],
			Type:     fields[1],
			SizeKB:   size,
			UsedKB:   used,
			Priority: priority,
		})
	}

	return devices, scanner.Err()
}

func parseKeyValueFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		value = strings.Trim(value, `"`)

		out[key] = value
	}

	return out, scanner.Err()
}

func parseKernelCmdline(cmdline string) map[string]string {
	out := map[string]string{}

	for _, field := range strings.Fields(cmdline) {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			out[field] = "true"
			continue
		}

		out[key] = value
	}

	return out
}

func kernelParamValue(cmdlinePath string, key string) string {
	cmdline, err := readTrim(cmdlinePath)
	if err != nil {
		return ""
	}

	params := parseKernelCmdline(cmdline)
	return params[key]
}

func readTrim(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

func commandOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := strings.TrimSpace(stdout.String())
	errOutput := strings.TrimSpace(stderr.String())

	if err != nil {
		if errOutput != "" {
			return output, fmt.Errorf("%w: %s", err, errOutput)
		}

		return output, err
	}

	return output, nil
}
