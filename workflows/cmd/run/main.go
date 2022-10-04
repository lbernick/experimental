package main

import (
	"fmt"
	"io/ioutil"

	"github.com/tektoncd/experimental/workflows/parse"
	"sigs.k8s.io/yaml"

	"github.com/spf13/cobra"
)

func main() {
	var fileName string

	var runCmd = &cobra.Command{
		Use: "run a workflow from a file",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runWorkflow(fileName); err != nil {
				fmt.Println(err.Error())
			}
		},
	}

	runCmd.Flags().StringVarP(&fileName, "file", "f", "", "workflow.yaml to use")
	runCmd.MarkFlagRequired("file")
	var rootCmd = &cobra.Command{
		Use:  "workflow",
		Args: cobra.MinimumNArgs(1),
	}
	rootCmd.AddCommand(runCmd)
	rootCmd.Execute()
}

func runWorkflow(fileName string) error {
	file, err := ioutil.ReadFile(fileName)
	if err != nil {
		fmt.Printf("error reading file: %+v", err)
	}

	w := parse.ParseWorkflowOrDie(file)

	pr, err := w.ToPipelineRun()
	if err != nil {
		return fmt.Errorf("error converting workflow to pipelinerun: %w", err)
	}
	pry, err := yaml.Marshal(pr)
	if err != nil {
		return fmt.Errorf("error converting pipelinerun to yaml: %w", err)
	}
	fmt.Printf("%s", pry)
	return nil
}
