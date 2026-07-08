package plan

import "github.com/urunc-dev/evaluation_suite/internal/manifest"

type Plan struct {
	Version string  `json:"version"`
	Trials  []Trial `json:"trials"`
}

type Trial struct {
	ID             string            `json:"id"`
	ExperimentName string            `json:"experimentName"`
	WorkloadName   string            `json:"workloadName"`
	RuntimeName    string            `json:"runtimeName"`
	RuntimeHandler string            `json:"runtimeHandler"`
	Image          string            `json:"image"`
	Ports          *manifest.Ports   `json:"ports,omitempty"`
	Volumes        []manifest.Volume `json:"volumes,omitempty"`
	Snapshotter    string            `yaml:"snapshotter,omitempty"`
}
