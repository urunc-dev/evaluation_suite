package plan_test

import (
	"strings"
	"testing"

	"github.com/urunc-dev/evaluation_suite/internal/manifest"
	"github.com/urunc-dev/evaluation_suite/internal/plan"
)

func TestGenerateDefaultTrialPerRuntime(t *testing.T) {
	m := &manifest.Manifest{
		Runtimes: []manifest.Runtime{
			{Name: "runc", Handler: "io.containerd.runc.v2"},
			{Name: "urunc", Handler: "io.containerd.urunc.v2"},
		},
		Experiments: map[string]manifest.Experiment{
			"lifecycle": {
				Workloads: manifest.Workloads{
					Default: manifest.Workload{Image: "docker.io/library/nginx:latest"},
				},
			},
		},
	}

	p, err := plan.Generate(m)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(p.Trials) != 2 {
		t.Fatalf("len(p.Trials) = %d; want 2", len(p.Trials))
	}

	if p.Trials[0].RuntimeName != "runc" || p.Trials[1].RuntimeName != "urunc" {
		t.Fatalf("trials not sorted by runtime name: %+v", p.Trials)
	}

	if p.Trials[0].ID != "lifecycle-default-runc" {
		t.Fatalf("Trials[0].ID = %q; want %q", p.Trials[0].ID, "lifecycle-default-runc")
	}
}

func TestGenerateOtherWorkloadOverridesDefaultForItsRuntime(t *testing.T) {
	m := &manifest.Manifest{
		Runtimes: []manifest.Runtime{
			{Name: "runc", Handler: "io.containerd.runc.v2"},
			{Name: "urunc", Handler: "io.containerd.urunc.v2"},
		},
		Experiments: map[string]manifest.Experiment{
			"lifecycle": {
				Workloads: manifest.Workloads{
					Default: manifest.Workload{Image: "docker.io/library/nginx:latest"},
					Other: []manifest.Workload{
						{Image: "docker.io/library/redis:latest", Runtime: "urunc"},
					},
				},
			},
		},
	}

	p, err := plan.Generate(m)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(p.Trials) != 2 {
		t.Fatalf("len(p.Trials) = %d; want 2, got %+v", len(p.Trials), p.Trials)
	}

	var sawDefaultRunc, sawOtherUrunc bool
	for _, trial := range p.Trials {
		if trial.RuntimeName == "runc" && trial.WorkloadName == "default" {
			sawDefaultRunc = true
		}
		if trial.RuntimeName == "urunc" && trial.WorkloadName == "other-0" {
			sawOtherUrunc = true
			if trial.Image != "docker.io/library/redis:latest" {
				t.Fatalf("other-0 trial image = %q; want redis image", trial.Image)
			}
		}
		if trial.RuntimeName == "urunc" && trial.WorkloadName == "default" {
			t.Fatalf("urunc should not get a default trial, it is overridden by other[0]")
		}
	}

	if !sawDefaultRunc || !sawOtherUrunc {
		t.Fatalf("missing expected trials: %+v", p.Trials)
	}
}

func TestGenerateOtherWorkloadWithoutRuntimeAppliesToAll(t *testing.T) {
	m := &manifest.Manifest{
		Runtimes: []manifest.Runtime{
			{Name: "runc", Handler: "io.containerd.runc.v2"},
			{Name: "urunc", Handler: "io.containerd.urunc.v2"},
		},
		Experiments: map[string]manifest.Experiment{
			"lifecycle": {
				Workloads: manifest.Workloads{
					Default: manifest.Workload{Image: "docker.io/library/nginx:latest"},
					Other: []manifest.Workload{
						{Image: "docker.io/library/redis:latest"},
					},
				},
			},
		},
	}

	p, err := plan.Generate(m)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(p.Trials) != 4 {
		t.Fatalf("len(p.Trials) = %d; want 4 (2 default + 2 other), got %+v", len(p.Trials), p.Trials)
	}
}

func TestGenerateOtherWorkloadUnknownRuntimeErrors(t *testing.T) {
	m := &manifest.Manifest{
		Runtimes: []manifest.Runtime{
			{Name: "runc", Handler: "io.containerd.runc.v2"},
		},
		Experiments: map[string]manifest.Experiment{
			"lifecycle": {
				Workloads: manifest.Workloads{
					Default: manifest.Workload{Image: "docker.io/library/nginx:latest"},
					Other: []manifest.Workload{
						{Image: "docker.io/library/redis:latest", Runtime: "kata"},
					},
				},
			},
		},
	}

	_, err := plan.Generate(m)
	if err == nil {
		t.Fatal("Generate() error = nil; want error for unknown runtime reference")
	}

	if !strings.Contains(err.Error(), `unknown runtime "kata"`) {
		t.Fatalf("Generate() error = %v; want it to mention unknown runtime kata", err)
	}
}

func TestGenerateTrialIDIsSlugified(t *testing.T) {
	m := &manifest.Manifest{
		Runtimes: []manifest.Runtime{
			{Name: "urunc", Handler: "io.containerd.urunc.v2"},
		},
		Experiments: map[string]manifest.Experiment{
			"HTTP Readiness!": {
				Workloads: manifest.Workloads{
					Default: manifest.Workload{Image: "docker.io/library/nginx:latest"},
				},
			},
		},
	}

	p, err := plan.Generate(m)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(p.Trials) != 1 {
		t.Fatalf("len(p.Trials) = %d; want 1", len(p.Trials))
	}

	want := "http-readiness-default-urunc"
	if p.Trials[0].ID != want {
		t.Fatalf("Trials[0].ID = %q; want %q", p.Trials[0].ID, want)
	}
}
