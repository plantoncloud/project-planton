package pulumimodule

import (
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const (
	DownloadUrl = "https://raw.githubusercontent.com/plantoncloud/project-planton/ca48cc8be896bd51d398f13f5bbb541d72cb334a/module-git-repos.yaml"
)

type DefaultPulumiModules struct {
	Atlas      map[string]string `yaml:"atlas"`
	Aws        map[string]string `yaml:"aws"`
	Confluent  map[string]string `yaml:"confluent"`
	Gcp        map[string]string `yaml:"gcp"`
	Kubernetes map[string]string `yaml:"kubernetes"`
	Snowflake  map[string]string `yaml:"snowflake"`
}

func GetCloneUrl(kindName string) (string, error) {
	defaultModules, err := downloadModuleInfo(DownloadUrl)
	if err != nil {
		return "", errors.Wrapf(err, "failed to download module info")
	}
	cloneUrl, err := getCloneUrlFromModules(defaultModules, kindName)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get clone url for the kind name")
	}
	return cloneUrl, nil
}

func convertKindName(kindName string) string {
	// This uses a Regex to find places where there is an uppercase letter
	// that is followed by a lowercase letter and separates the words using a hyphen
	re := regexp.MustCompile("([a-z])([A-Z])")
	// Replace the matches found by the regex with a hyphen and the matched uppercase letter in lowercase
	formattedName := re.ReplaceAllStringFunc(kindName, func(match string) string {
		return match[:1] + "-" + strings.ToLower(match[1:])
	})
	// Convert the final string to lowercase and return it
	return strings.ToLower(formattedName)
}

func downloadModuleInfo(url string) (DefaultPulumiModules, error) {
	resp, err := http.Get(url)
	if err != nil {
		return DefaultPulumiModules{}, errors.Wrap(err, "failed to fetch yaml")
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return DefaultPulumiModules{}, errors.Wrap(err, "failed to read response body")
	}

	var modules DefaultPulumiModules
	err = yaml.Unmarshal(body, &modules)
	if err != nil {
		return DefaultPulumiModules{}, errors.Wrapf(err, "failed to unmarshal yaml")
	}

	return modules, nil
}

func getCloneUrlFromModules(modules DefaultPulumiModules, kindName string) (string, error) {
	formattedKindName := convertKindName(kindName)

	if url, found := modules.Atlas[formattedKindName]; found {
		return url, nil
	}
	if url, found := modules.Aws[formattedKindName]; found {
		return url, nil
	}
	if url, found := modules.Confluent[formattedKindName]; found {
		return url, nil
	}
	if url, found := modules.Gcp[formattedKindName]; found {
		return url, nil
	}
	if url, found := modules.Kubernetes[formattedKindName]; found {
		return url, nil
	}
	if url, found := modules.Snowflake[formattedKindName]; found {
		return url, nil
	}

	return "", errors.New("clone url not found for the provided kind name")
}
