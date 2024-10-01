package pulumistack

import (
	"buf.build/gen/go/plantoncloud/project-planton/protocolbuffers/go/project/planton/shared/pulumi"
	"context"
	"github.com/pkg/errors"
	"github.com/plantoncloud/project-planton/internal/pulumimodule"
	"github.com/plantoncloud/project-planton/internal/stackinput"
	cliworkspace "github.com/plantoncloud/project-planton/internal/workspace"
	commonsstackinput "github.com/plantoncloud/pulumi-module-golang-commons/pkg/stackinput"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"strings"
)

// ExtractProjectName extracts the project name from the stack FQDN.
func ExtractProjectName(stackFqdn string) (string, error) {
	parts := strings.Split(stackFqdn, "/")
	if len(parts) != 3 {
		return "", errors.New("invalid stack fqdn format, expected format <org>/<project>/<stack>")
	}
	return parts[1], nil
}

func Run(stackFqdn, targetManifestPath, kubernetesClusterManifestPath string,
	pulumiOperation pulumi.PulumiOperationType, isUpdatePreview bool) error {
	kindName, err := stackinput.ExtractKindFromTargetManifest(targetManifestPath)
	if err != nil {
		return errors.Wrapf(err, "failed to extract kind from %s stack input yaml", targetManifestPath)
	}

	cloneUrl, err := pulumimodule.GetCloneUrl(kindName)
	if err != nil {
		return errors.Wrapf(err, "failed to get clone url for %s kind", kindName)
	}

	gitRepo := auto.GitRepo{
		URL:     cloneUrl,
		Shallow: false,
	}

	stackWorkspaceDir, err := cliworkspace.GetWorkspaceDir(stackFqdn)
	if err != nil {
		return errors.Wrapf(err, "failed to get %s stack worspace directory", stackFqdn)
	}

	pulumiProjectName, err := ExtractProjectName(stackFqdn)
	if err != nil {
		return errors.Wrapf(err, "failed to extract project name from %s stack fqdn", stackFqdn)
	}

	stackInputYamlContent, err := stackinput.BuildStackInputYaml(targetManifestPath, kubernetesClusterManifestPath)
	if err != nil {
		return errors.Wrap(err, "failed to build stack input yaml")
	}

	//setup pulumi automation-api stack
	pulumiStack, err := auto.UpsertStackLocalSource(
		context.Background(),
		stackFqdn,
		stackWorkspaceDir,
		auto.Repo(gitRepo),
		auto.Project(workspace.Project{
			Name: tokens.PackageName(pulumiProjectName),
			Runtime: workspace.NewProjectRuntimeInfo(
				pulumi.PulumiProjectRuntime_go.String(),
				map[string]interface{}{}),
		}),
		auto.WorkDir(stackWorkspaceDir),
		auto.EnvVars(map[string]string{
			commonsstackinput.YamlContentEnvVar: stackInputYamlContent,
		}))

	if err != nil {
		return errors.Wrapf(err, "failed to setup pulumi automation-api stack %s", stackFqdn)
	}

	switch pulumiOperation {
	case pulumi.PulumiOperationType_refresh:
		if _, err := pulumiStack.Refresh(context.Background()); err != nil {
			return errors.Wrapf(err, "failed to refresh %s stack", stackFqdn)
		}
	case pulumi.PulumiOperationType_update:
		if isUpdatePreview {
			if _, err := pulumiStack.Preview(context.Background()); err != nil {
				return errors.Wrapf(err, "failed to preview %s stack", stackFqdn)
			}
		} else {
			if _, err := pulumiStack.Up(context.Background()); err != nil {
				return errors.Wrapf(err, "failed to update %s stack", stackFqdn)
			}
		}
	case pulumi.PulumiOperationType_destroy:
		if _, err := pulumiStack.Destroy(context.Background()); err != nil {
			return errors.Wrapf(err, "failed to destroy %s stack", stackFqdn)
		}
	}
	return nil
}
