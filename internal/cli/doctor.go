package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urunc-dev/evaluation_suite/internal/doctor"
	"github.com/urunc-dev/evaluation_suite/internal/manifest"
)

func NewDoctorCommand() *cobra.Command {
	var file string
	var output string
	var resultsDir string
	var containerdSocket string
	var containerdConfig string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check whether the host is ready to run benchmark experiments",
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("manifest file is required: use -f or --file")
			}

			m, err := manifest.LoadFile(file)
			if err != nil {
				return err
			}

			report, err := doctor.Run(cmd.Context(), m, doctor.Options{
				ManifestPath:     file,
				ResultsDir:       resultsDir,
				ContainerdSocket: containerdSocket,
				ContainerdConfig: containerdConfig,
			})
			if err != nil {
				return err
			}

			switch output {
			case "json":
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(report); err != nil {
					return err
				}

			case "summary":
				fmt.Fprintf(cmd.OutOrStdout(), "doctor status: %s\n", report.Status)

				for _, check := range report.Checks {
					fmt.Fprintf(
						cmd.OutOrStdout(),
						"[%s] %s: %s\n",
						check.Status,
						check.Name,
						check.Message,
					)
				}

			default:
				return fmt.Errorf("unsupported output format %q: expected summary or json", output)
			}

			if report.Failed() {
				return fmt.Errorf("doctor found failing checks")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to experiment manifest")
	cmd.Flags().StringVarP(&output, "output", "o", "summary", "Output format: summary or json")
	cmd.Flags().StringVar(&resultsDir, "results-dir", "results", "Directory where run artifacts will be written")
	cmd.Flags().StringVar(&containerdSocket, "containerd-socket", "/run/containerd/containerd.sock", "Path to containerd socket")
	cmd.Flags().StringVar(&containerdConfig, "containerd-config", "/etc/containerd/config.toml", "Path to containerd config.toml")

	_ = cmd.MarkFlagRequired("file")

	return cmd
}
