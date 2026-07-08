package doctor

type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

type Report struct {
	Status Status  `json:"status"`
	Checks []Check `json:"checks"`
}

type Check struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Message string `json:"message"`
}

type Options struct {
	ManifestPath      string
	ResultsDir        string
	ContainerdSocket  string
	ContainerdConfig  string
	RequireContainerd bool
}

func NewReport() *Report {
	return &Report{
		Status: StatusPass,
		Checks: []Check{},
	}
}

func (r *Report) Add(status Status, name string, message string) {
	r.Checks = append(r.Checks, Check{
		Name:    name,
		Status:  status,
		Message: message,
	})

	if status == StatusFail {
		r.Status = StatusFail
		return
	}

	if status == StatusWarn && r.Status != StatusFail {
		r.Status = StatusWarn
	}
}

func (r *Report) Failed() bool {
	return r.Status == StatusFail
}
