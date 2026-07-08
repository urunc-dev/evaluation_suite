// internal/manifest/validate.go
package manifest

import (
	"errors"
	"fmt"
	"strings"
)

func Validate(m *Manifest) error {
	var errs []string

	if len(m.Runtimes) == 0 {
		errs = append(errs, "runtimes must contain at least one runtime")
	}

	runtimeNames := map[string]bool{}
	for i, rt := range m.Runtimes {
		if rt.Name == "" {
			errs = append(errs, fmt.Sprintf("runtimes[%d].name is required", i))
		}
		if rt.Handler == "" {
			errs = append(errs, fmt.Sprintf("runtimes[%d].handler is required", i))
		}
		if runtimeNames[rt.Name] {
			errs = append(errs, fmt.Sprintf("runtime %q is duplicated", rt.Name))
		}
		runtimeNames[rt.Name] = true
	}

	if len(m.Experiments) == 0 {
		errs = append(errs, "experiments must contain at least one experiment")
	}

	for name, exp := range m.Experiments {
		if exp.Workloads.Default.Image == "" {
			errs = append(errs, fmt.Sprintf("experiments.%s.workloads.default.image is required", name))
		}

		validateWorkload := func(path string, w Workload) {
			if w.Image == "" {
				errs = append(errs, path+".image is required")
			}

			if w.Runtime != "" && !runtimeNames[w.Runtime] {
				errs = append(errs, fmt.Sprintf("%s.runtime references unknown runtime %q", path, w.Runtime))
			}

			if w.Ports != nil {
				if w.Ports.ContainerPort <= 0 || w.Ports.ContainerPort > 65535 {
					errs = append(errs, path+".ports.containerPort must be between 1 and 65535")
				}
				if w.Ports.HostPort <= 0 || w.Ports.HostPort > 65535 {
					errs = append(errs, path+".ports.hostPort must be between 1 and 65535")
				}
			}

			for i, v := range w.Volumes {
				vpath := fmt.Sprintf("%s.volumes[%d]", path, i)
				if v.Name == "" {
					errs = append(errs, vpath+".name is required")
				}
				if v.MountPath == "" {
					errs = append(errs, vpath+".mountPath is required")
				}
				if v.Type == "" {
					errs = append(errs, vpath+".type is required")
				}
			}
		}

		validateWorkload("experiments."+name+".workloads.default", exp.Workloads.Default)

		for i, w := range exp.Workloads.Other {
			validateWorkload(fmt.Sprintf("experiments.%s.workloads.other[%d]", name, i), w)
		}
	}

	if len(errs) > 0 {
		return errors.New("manifest validation failed:\n- " + strings.Join(errs, "\n- "))
	}

	return nil
}
