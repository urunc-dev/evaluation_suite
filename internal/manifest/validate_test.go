package manifest_test

import (
	"strings"
	"testing"

	"github.com/urunc-dev/evaluation_suite/internal/manifest"
)

func validManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Runtimes: []manifest.Runtime{
			{Name: "runc", Handler: "io.containerd.runc.v2"},
		},
		Experiments: map[string]manifest.Experiment{
			"lifecycle": {
				Workloads: manifest.Workloads{
					Default: manifest.Workload{Image: "docker.io/library/nginx:latest"},
				},
			},
		},
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(m *manifest.Manifest)
		wantError string
	}{
		{
			name:      "valid manifest",
			mutate:    func(m *manifest.Manifest) {},
			wantError: "",
		},
		{
			name:      "no runtimes",
			mutate:    func(m *manifest.Manifest) { m.Runtimes = nil },
			wantError: "runtimes must contain at least one runtime",
		},
		{
			name:      "runtime missing name",
			mutate:    func(m *manifest.Manifest) { m.Runtimes[0].Name = "" },
			wantError: "runtimes[0].name is required",
		},
		{
			name:      "runtime missing handler",
			mutate:    func(m *manifest.Manifest) { m.Runtimes[0].Handler = "" },
			wantError: "runtimes[0].handler is required",
		},
		{
			name: "duplicate runtime name",
			mutate: func(m *manifest.Manifest) {
				m.Runtimes = append(m.Runtimes, manifest.Runtime{Name: "runc", Handler: "io.containerd.runsc.v1"})
			},
			wantError: `runtime "runc" is duplicated`,
		},
		{
			name:      "no experiments",
			mutate:    func(m *manifest.Manifest) { m.Experiments = nil },
			wantError: "experiments must contain at least one experiment",
		},
		{
			name: "default workload missing image",
			mutate: func(m *manifest.Manifest) {
				exp := m.Experiments["lifecycle"]
				exp.Workloads.Default.Image = ""
				m.Experiments["lifecycle"] = exp
			},
			wantError: "experiments.lifecycle.workloads.default.image is required",
		},
		{
			name: "other workload references unknown runtime",
			mutate: func(m *manifest.Manifest) {
				exp := m.Experiments["lifecycle"]
				exp.Workloads.Other = []manifest.Workload{
					{Image: "docker.io/library/redis:latest", Runtime: "kata"},
				}
				m.Experiments["lifecycle"] = exp
			},
			wantError: `experiments.lifecycle.workloads.other[0].runtime references unknown runtime "kata"`,
		},
		{
			name: "container port out of range",
			mutate: func(m *manifest.Manifest) {
				exp := m.Experiments["lifecycle"]
				exp.Workloads.Default.Ports = &manifest.Ports{ContainerPort: 70000, HostPort: 8080}
				m.Experiments["lifecycle"] = exp
			},
			wantError: "experiments.lifecycle.workloads.default.ports.containerPort must be between 1 and 65535",
		},
		{
			name: "host port out of range",
			mutate: func(m *manifest.Manifest) {
				exp := m.Experiments["lifecycle"]
				exp.Workloads.Default.Ports = &manifest.Ports{ContainerPort: 80, HostPort: 0}
				m.Experiments["lifecycle"] = exp
			},
			wantError: "experiments.lifecycle.workloads.default.ports.hostPort must be between 1 and 65535",
		},
		{
			name: "volume missing required fields",
			mutate: func(m *manifest.Manifest) {
				exp := m.Experiments["lifecycle"]
				exp.Workloads.Default.Volumes = []manifest.Volume{{}}
				m.Experiments["lifecycle"] = exp
			},
			wantError: "experiments.lifecycle.workloads.default.volumes[0].name is required",
		},
		{
			name: "cpu experiment missing cpu and timeout",
			mutate: func(m *manifest.Manifest) {
				m.Experiments["cpu"] = manifest.Experiment{
					Workloads: manifest.Workloads{
						Default: manifest.Workload{Image: "docker.io/library/stress-ng:latest"},
					},
				}
			},
			wantError: "experiments.cpu.workloads.default.cpu must be greater than zero",
		},
		{
			name: "cpu experiment invalid timeout duration",
			mutate: func(m *manifest.Manifest) {
				m.Experiments["cpu"] = manifest.Experiment{
					Workloads: manifest.Workloads{
						Default: manifest.Workload{
							Image:     "docker.io/library/stress-ng:latest",
							CPU:       1,
							CPUMethod: "stress-ng",
							Timeout:   "not-a-duration",
						},
					},
				}
			},
			wantError: "experiments.cpu.workloads.default.timeout must be a positive Go duration such as 30s",
		},
		{
			name: "http-readiness experiment missing ports",
			mutate: func(m *manifest.Manifest) {
				m.Experiments["http-readiness"] = manifest.Experiment{
					Workloads: manifest.Workloads{
						Default: manifest.Workload{Image: "docker.io/library/nginx:latest"},
					},
				}
			},
			wantError: "experiments.http-readiness.workloads.default.ports is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validManifest()
			tt.mutate(m)

			err := manifest.Validate(m)

			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() = %v; want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("Validate() = nil; want error containing %q", tt.wantError)
			}

			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Validate() = %v; want error containing %q", err, tt.wantError)
			}
		})
	}
}
