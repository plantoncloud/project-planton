package tofu

import (
	"fmt"
	"github.com/plantonhq/planton/internal/cli/ui"

	"github.com/plantonhq/planton/internal/cli/flag"
	"github.com/plantonhq/planton/pkg/crkreflect"
	"github.com/plantonhq/planton/pkg/iac/tofu/generators"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var GenerateVariables = &cobra.Command{
	Use:   "generate-variables <component>",
	Short: "Generate Terraform variables for a specified component",
	Long: `The "generate-variables" command takes a specified planton 
component type (e.g., "S3Bucket", "RedisKubernetes") and generates 
Terraform variable definitions (variables.tf) and a corresponding 
terraform.tfvars file.

This command instantiates an empty object of the specified component kind 
under the hood, and then converts that empty object into a Terraform-compatible 
variables file. These variables can then be passed into Terraform modules, 
streamlining infrastructure provisioning and ensuring a consistent, 
declarative workflow.`,
	Example: `
  # Generate variables for an S3Bucket component
  planton tofu generate-variables S3Bucket

  # Generate variables for a RedisKubernetes component
  planton tofu generate-variables RedisKubernetes
`,
	Args: cobra.ExactArgs(1), // "s3-bucket", "redis-kubernetes", etc.
	Run:  generateVariablesHandler,
}

func init() {
	GenerateVariables.Flags().String(string(flag.OutputFile), "", "output file for Terraform variables")
}

func generateVariablesHandler(cmd *cobra.Command, args []string) {
	kindName := args[0]

	outputFile, err := cmd.Flags().GetString(string(flag.OutputFile))
	flag.HandleFlagErr(err, flag.OutputFile)

	cloudResourceKind := crkreflect.KindFromString(kindName)

	manifestObject := crkreflect.ToMessageMap[cloudResourceKind]

	if manifestObject == nil {
		ui.Failure(
			fmt.Sprintf("no spec message is registered for kind %s", cloudResourceKind.String()),
			"the kind exists in the catalog enum but its proto package is not linked into this binary",
			"run `make generate-cloud-resource-kind-map` and rebuild, then retry",
		)
	}

	variablesTfContent, err := generators.ProtoToVariablesTF(manifestObject)
	if err != nil {
		ui.Failure(
			fmt.Sprintf("the Terraform variables could not be generated: %v", err),
			"the kind's spec message could not be walked into HCL variable declarations",
			"report it at https://github.com/plantonhq/planton/issues naming the kind",
		)
	}
	if outputFile != "" {
		// A committed variables.tf ends with a newline (tofu fmt's shape); the
		// generator's text does not carry one, so add it here rather than
		// leaving every regenerated module with a "no newline at end of file"
		// diff hunk.
		if err := os.WriteFile(outputFile, []byte(strings.TrimRight(variablesTfContent, "\n")+"\n"), 0644); err != nil {
			ui.Failure(
				fmt.Sprintf("the generated variables could not be written to %s: %v", outputFile, err),
				"the output path is not writable, or its parent directory does not exist",
				"create the parent directory or point --output-file at a writable path",
			)
		}
		log.Infof("Terraform variables written to file %s", outputFile)
	} else {
		// fmt.Println writes to stdout; the builtin println writes to stderr,
		// which silently breaks `> variables.tf` capture in scripts.
		fmt.Println(variablesTfContent)
	}
}
