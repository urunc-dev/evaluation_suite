package plan

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/urunc-dev/evaluation_suite/internal/manifest"
)

func Generate(m *manifest.Manifest) (*Plan, error) {
	runtimeByName := make(map[string]manifest.Runtime)

	for _, rt := range m.Runtimes {
		runtimeByName[rt.Name] = rt
	}

	runtimeNames := make([]string, 0, len(m.Runtimes))
	for _, rt := range m.Runtimes {
		runtimeNames = append(runtimeNames, rt.Name)
	}
	sort.Strings(runtimeNames)

	experimentNames := make([]string, 0, len(m.Experiments))
	for name := range m.Experiments {
		experimentNames = append(experimentNames, name)
	}
	sort.Strings(experimentNames)

	var trials []Trial

	for _, experimentName := range experimentNames {
		exp := m.Experiments[experimentName]

		defaultWorkload := exp.Workloads.Default

		for _, runtimeName := range runtimeNames {
			rt := runtimeByName[runtimeName]

			// first check if the runtime is metioned in exp.Workloads.Other, if it is just ignore it on default
			var inOther bool
			for _, workload := range exp.Workloads.Other {
				if workload.Runtime == runtimeName {
					inOther = true
					break
				}
			}

			if inOther {
				continue
			}

			trial := buildTrial(
				experimentName,
				"default",
				runtimeName,
				rt.Handler,
				defaultWorkload,
			)

			trials = append(trials, trial)
		}

		for i, workload := range exp.Workloads.Other {
			workloadName := fmt.Sprintf("other-%d", i)

			if workload.Runtime != "" {
				rt, ok := runtimeByName[workload.Runtime]
				if !ok {
					return nil, fmt.Errorf(
						"experiments.%s.workloads.other[%d] references unknown runtime %q",
						experimentName,
						i,
						workload.Runtime,
					)
				}

				trial := buildTrial(
					experimentName,
					workloadName,
					rt.Name,
					rt.Handler,
					workload,
				)

				trials = append(trials, trial)
				continue
			}

			for _, runtimeName := range runtimeNames {
				rt := runtimeByName[runtimeName]

				trial := buildTrial(
					experimentName,
					workloadName,
					runtimeName,
					rt.Handler,
					workload,
				)

				trials = append(trials, trial)
			}
		}
	}

	return &Plan{
		Version: "v1alpha1",
		Trials:  trials,
	}, nil
}

func buildTrial(
	experimentName string,
	workloadName string,
	runtimeName string,
	runtimeHandler string,
	workload manifest.Workload,
) Trial {
	id := makeTrialID(experimentName, workloadName, runtimeName)

	return Trial{
		ID:             id,
		ExperimentName: experimentName,
		WorkloadName:   workloadName,
		RuntimeName:    runtimeName,
		RuntimeHandler: runtimeHandler,
		Image:          workload.Image,
		Ports:          workload.Ports,
		Volumes:        workload.Volumes,
		Snapshotter:    workload.Snapshotter,
	}
}

func makeTrialID(experimentName, workloadName, runtimeName string) string {
	raw := fmt.Sprintf("%s-%s-%s", experimentName, workloadName, runtimeName)
	return slugify(raw)
}

func slugify(s string) string {
	s = strings.ToLower(s)

	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")

	s = strings.Trim(s, "-")

	if s == "" {
		return "trial"
	}

	return s
}
