package credentials

import (
	"github.com/pkg/errors"
	"github.com/plantoncloud/project-planton/internal/fileutil"
	"os"
)

const (
	dockerCredentialKey  = "dockerCredential"
	dockerCredentialYaml = "docker-credential.yaml"
)

func AddDockerCredential(stackInputContentMap map[string]string, stackInputOptions StackInputCredentialOptions) (map[string]string, error) {
	if stackInputOptions.DockerCredential != "" {
		credentialContent, err := os.ReadFile(stackInputOptions.DockerCredential)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read file: %s", stackInputOptions.DockerCredential)
		}
		stackInputContentMap[dockerCredentialKey] = string(credentialContent)
	}
	return stackInputContentMap, nil
}

func LoadDockerCredential(dir string) (string, error) {
	path := dir + "/" + dockerCredentialYaml
	isExists, err := fileutil.IsExists(path)
	if err != nil {
		return "", errors.Wrapf(err, "failed to check file: %s", path)
	}
	if !isExists {
		return "", nil
	}
	return path, nil
}
