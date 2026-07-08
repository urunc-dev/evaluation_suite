package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/urunc-dev/evaluation_suite/internal/manifest"
)

type ContainerdConfig struct {
	Path     string                       `json:"path"`
	Runtimes map[string]ContainerdRuntime `json:"runtimes"`
}

type ContainerdRuntime struct {
	Name        string `json:"name"`
	RuntimeType string `json:"runtimeType"`
	ConfigPath  string `json:"configPath"`
}

func LoadContainerdConfig(path string) (*ContainerdConfig, error) {
	if path == "" {
		path = "/etc/containerd/config.toml"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	cfg := &ContainerdConfig{
		Path:     path,
		Runtimes: map[string]ContainerdRuntime{},
	}

	walkContainerdConfig(cfg, nil, root)

	return cfg, nil
}

func walkContainerdConfig(cfg *ContainerdConfig, path []string, node any) {
	table, ok := node.(map[string]any)
	if !ok {
		return
	}

	if runtimesRaw, ok := table["runtimes"]; ok {
		if runtimes, ok := runtimesRaw.(map[string]any); ok {
			for runtimeName, runtimeRaw := range runtimes {
				runtimeTable, ok := runtimeRaw.(map[string]any)
				if !ok {
					continue
				}

				runtimeType, _ := runtimeTable["runtime_type"].(string)
				if runtimeType == "" {
					runtimeType, _ = runtimeTable["runtimeType"].(string)
				}

				configPath := strings.Join(append(path, "runtimes", runtimeName), ".")

				cfg.Runtimes[runtimeName] = ContainerdRuntime{
					Name:        runtimeName,
					RuntimeType: runtimeType,
					ConfigPath:  configPath,
				}
			}
		}
	}

	keys := make([]string, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		walkContainerdConfig(cfg, append(path, key), table[key])
	}
}

func CheckContainerdConfig(report *Report, m *manifest.Manifest, configPath string) {
	if configPath == "" {
		configPath = "/etc/containerd/config.toml"
	}

	cfg, err := LoadContainerdConfig(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			report.Add(
				StatusWarn,
				"containerd-config",
				fmt.Sprintf("containerd config %q does not exist; only default runc can be assumed", configPath),
			)

			checkRuntimeHandlersWithMissingConfig(report, m)
			return
		}

		report.Add(
			StatusFail,
			"containerd-config",
			fmt.Sprintf("cannot read or parse containerd config %q: %v", configPath, err),
		)
		return
	}

	report.Add(
		StatusPass,
		"containerd-config",
		fmt.Sprintf("containerd config parsed from %s; discovered %d configured runtime(s)", cfg.Path, len(cfg.Runtimes)),
	)

	if len(cfg.Runtimes) == 0 {
		report.Add(
			StatusWarn,
			"containerd-runtimes",
			"no runtime entries were discovered in containerd config",
		)
	}

	checkRuntimeHandlersFromConfig(report, m, cfg)
}

func checkRuntimeHandlersWithMissingConfig(report *Report, m *manifest.Manifest) {
	for _, rt := range m.Runtimes {
		if rt.Name == "runc" && rt.Handler == "io.containerd.runc.v2" {
			report.Add(
				StatusWarn,
				"runtime:"+rt.Name,
				"assuming default runc runtime because containerd config is missing",
			)
			continue
		}

		report.Add(
			StatusFail,
			"runtime:"+rt.Name,
			fmt.Sprintf(
				"cannot verify runtime %q with handler %q because containerd config is missing",
				rt.Name,
				rt.Handler,
			),
		)
	}
}

func checkRuntimeHandlersFromConfig(report *Report, m *manifest.Manifest, cfg *ContainerdConfig) {
	for _, declared := range m.Runtimes {
		foundByName, nameExists := cfg.Runtimes[declared.Name]

		if nameExists {
			if foundByName.RuntimeType == declared.Handler {
				report.Add(
					StatusPass,
					"runtime:"+declared.Name,
					fmt.Sprintf(
						"runtime %q found in containerd config with handler %q",
						declared.Name,
						declared.Handler,
					),
				)
				continue
			}

			report.Add(
				StatusFail,
				"runtime:"+declared.Name,
				fmt.Sprintf(
					"runtime %q exists in containerd config but has runtime_type %q, expected %q",
					declared.Name,
					foundByName.RuntimeType,
					declared.Handler,
				),
			)
			continue
		}

		matches := findRuntimeTypeMatches(cfg, declared.Handler)
		if len(matches) > 0 {
			report.Add(
				StatusWarn,
				"runtime:"+declared.Name,
				fmt.Sprintf(
					"handler %q exists in containerd config as runtime name(s) %s, but manifest runtime name is %q",
					declared.Handler,
					strings.Join(matches, ", "),
					declared.Name,
				),
			)
			continue
		}

		if declared.Name == "runc" && declared.Handler == "io.containerd.runc.v2" {
			report.Add(
				StatusWarn,
				"runtime:"+declared.Name,
				"runc handler was not explicitly found in config; containerd may still use its default runc runtime",
			)
			continue
		}

		report.Add(
			StatusFail,
			"runtime:"+declared.Name,
			fmt.Sprintf(
				"runtime %q with handler %q was not found in containerd config %s",
				declared.Name,
				declared.Handler,
				filepath.Clean(cfg.Path),
			),
		)
	}
}

func findRuntimeTypeMatches(cfg *ContainerdConfig, runtimeType string) []string {
	var matches []string

	for name, rt := range cfg.Runtimes {
		if rt.RuntimeType == runtimeType {
			matches = append(matches, name)
		}
	}

	sort.Strings(matches)
	return matches
}
