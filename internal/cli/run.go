package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/containerd/containerd"

	"github.com/containerd/containerd/namespaces"
	"github.com/urunc-dev/evaluation_suite/internal/artifact"
	"github.com/urunc-dev/evaluation_suite/internal/environment"
	"github.com/urunc-dev/evaluation_suite/internal/manifest"
	"github.com/urunc-dev/evaluation_suite/internal/orchestrator"
	"github.com/urunc-dev/evaluation_suite/internal/plan"
	harnessruntime "github.com/urunc-dev/evaluation_suite/internal/runtime"
	runtimeLifecycle "github.com/urunc-dev/evaluation_suite/internal/runtime/lifecycle"
	runtimeStorage "github.com/urunc-dev/evaluation_suite/internal/runtime/storage"
)

func NewRunCommand() *cobra.Command {
	var file string
	var output string
	var resultsDir string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run benchmark trials from an experiment manifest",
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

			env, err := environment.Capture(cmd.Context())
			if err != nil {
				return err
			}

			runID := orchestrator.NewRunID(time.Now())

			store := artifact.NewStore(resultsDir)

			paths, err := store.InitRun(runID, file, p, env)
			if err != nil {
				return err
			}

			containerdClient, err := containerd.New("/run/containerd/containerd.sock")
			if err != nil {
				return err
			}
			defer containerdClient.Close()

			containerdNamespace := namespaces.WithNamespace(cmd.Context(), "default")

			lifecycleAdapterFactory := func(trial plan.Trial) (harnessruntime.Adapter, error) {
				return runtimeLifecycle.NewAdapter(containerdClient, &containerdNamespace), nil
			}

			storageAdapterFactory := func(trial plan.Trial) (harnessruntime.Adapter, error) {
				return runtimeStorage.NewAdapter(containerdClient, &containerdNamespace), nil
			}

			orch := orchestrator.New(lifecycleAdapterFactory, storageAdapterFactory)

			result, err := orch.Run(cmd.Context(), p, orchestrator.Options{
				RunID: runID,
			})
			if err != nil {
				return err
			}

			if err := store.WriteRunResult(result); err != nil {
				return err
			}

			switch output {
			case "json":
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"run":         result,
					"environment": env,
					"artifacts":   paths,
				})

			case "summary":
				fmt.Fprintf(
					cmd.OutOrStdout(),
					"run=%s dryRun=%t trials=%d duration=%s artifacts=%s environmentWarnings=%d\n",
					result.RunID,
					result.DryRun,
					len(result.Trials),
					result.Duration,
					paths.RunDir,
					len(env.Warnings),
				)

				for _, trial := range result.Trials {
					fmt.Fprintf(
						cmd.OutOrStdout(),
						"%s status=%s runtime=%s experiment=%s workload=%s stages=%d",
						trial.Trial.ID,
						trial.Status,
						trial.Trial.RuntimeName,
						trial.Trial.ExperimentName,
						trial.Trial.WorkloadName,
						len(trial.RuntimeStages),
					)
				}

				if len(env.Warnings) > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "environment warnings:")
					for _, warning := range env.Warnings {
						fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", warning)
					}
				}

				return nil

			default:
				return fmt.Errorf("unsupported output format %q: expected json or summary", output)
			}
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to experiment manifest")
	cmd.Flags().StringVarP(&output, "output", "o", "summary", "Output format: summary or json")
	cmd.Flags().StringVar(&resultsDir, "results-dir", "results", "Directory where run artifacts are written")

	_ = cmd.MarkFlagRequired("file")

	return cmd
}
