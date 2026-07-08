package manifest

type Manifest struct {
	Runtimes    []Runtime             `yaml:"runtimes"`
	Experiments map[string]Experiment `yaml:"experiments"`
}

type Runtime struct {
	Name    string `yaml:"name"`
	Handler string `yaml:"handler"`
}

type Experiment struct {
	Workloads Workloads `yaml:"workloads"`
}

type Workloads struct {
	Default Workload   `yaml:"default"`
	Other   []Workload `yaml:"other"`
}

type Workload struct {
	Image       string   `yaml:"image"`
	Runtime     string   `yaml:"runtime,omitempty"`
	Ports       *Ports   `yaml:"ports,omitempty"`
	Snapshotter string   `yaml:"snapshotter,omitempty"`
	Volumes     []Volume `yaml:"volumes,omitempty"`
}

type Ports struct {
	ContainerPort int `yaml:"containerPort"`
	HostPort      int `yaml:"hostPort"`
}

type Volume struct {
	Name      string `yaml:"name"`
	MountPath string `yaml:"mountPath"`
	Type      string `yaml:"type"`
	Source    string `yaml:"source,omitempty"`
}
