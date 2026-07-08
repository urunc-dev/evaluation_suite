// Copyright (c) 2026-present, urunc-dev
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Version string = "development"

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "evaluation_suite",
	Short:   "Runtime Benchmarking tool",
	Long:    `A tool to benchmark OCI rutimes and compare their performance. It provides a set of commands to run benchmarks, collect results, and generate reports.`,
	Version: Version,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Runtime Benchmark Evaluation Suite " + Version)
		fmt.Println("Run 'evaluation_suite --help' to see available commands")
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(NewValidateCommand())
	rootCmd.AddCommand(NewPlanCommand())
	rootCmd.AddCommand(NewRunCommand())
	rootCmd.AddCommand(NewDoctorCommand())

}
