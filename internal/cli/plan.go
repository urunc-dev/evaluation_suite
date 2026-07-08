package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urunc-dev/evaluation_suite/internal/manifest"
	"github.com/urunc-dev/evaluation_suite/internal/plan"
)

func NewPlanCommand() *cobra.Command {
	var file string
	var output string

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Generate a concrete trial plan from an experiment manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("manifest file is required: use -f or --file")
			}

			m, err := manifest.LoadFile(file)
			if err != nil {
				return err
			}

			if err := manifest.Validate(m); err != nil {
				return err
			}

			p, err := plan.Generate(m)
			if err != nil {
				return err
			}

			switch output {
			case "json":
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(p)

			case "summary":
				for _, trial := range p.Trials {
					fmt.Fprintf(
						cmd.OutOrStdout(),
						"%s: experiment=%s workload=%s runtime=%s handler=%s image=%s\n",
						trial.ID,
						trial.ExperimentName,
						trial.WorkloadName,
						trial.RuntimeName,
						trial.RuntimeHandler,
						trial.Image,
					)
				}
				return nil

			default:
				return fmt.Errorf("unsupported output format %q: expected json or summary", output)
			}
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to experiment manifest")
	cmd.Flags().StringVarP(&output, "output", "o", "json", "Output format: json or summary")

	_ = cmd.MarkFlagRequired("file")

	return cmd
}
