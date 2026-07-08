package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urunc-dev/evaluation_suite/internal/manifest"
)

func NewValidateCommand() *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate an experiment manifest",
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

			fmt.Fprintln(cmd.OutOrStdout(), "manifest is valid")
			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to experiment manifest")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}
