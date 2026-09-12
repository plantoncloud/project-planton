package failure

import (
	"regexp"
	"strings"
)

// Explain reads raw engine output and returns the three-part explanation of
// every failure it recognizes: Helm's chart-location errors, the Kubernetes
// API server's authorization refusals. It exists for the failures a module
// cannot rephrase itself. An HCL module can only speak in a precondition or
// postcondition, which run on a data source's RESULT; when the read itself
// fails (a repository host that does not resolve, an API server that answers
// Forbidden) the provider's raw text is all the engine prints. Pulumi modules
// classify the same texts in-process; the CLI and the e2e harness run this
// over the engine's output so both engines end in the same words.
//
// Each explainer fires at most once per output (three CRDs refused for the
// same right are one root cause), and stays silent when the output already
// carries its own explanation: a module that could see the fact has already
// spoken in three parts, and repeating it would bury the engine's text.
//
// The engines wrap diagnostics at terminal width and draw boxes around them,
// so matching runs on whitespace-collapsed text with the box characters
// removed; values are read from the collapsed text.
func Explain(engineOutput string) []*Failure {
	text := collapse(engineOutput)
	var failures []*Failure
	for _, e := range explainers {
		if strings.Contains(text, e.signature) {
			continue
		}
		if f := e.explain(text); f != nil {
			failures = append(failures, f)
		}
	}
	return failures
}

// Annotate wraps an engine error with the explanations found in the engine's
// output. The result unwraps to the original error and to each Failure, so
// errors.As finds the explanation wherever the caller checks for one. An
// output nothing recognizes returns err unchanged.
func Annotate(err error, engineOutput string) error {
	if err == nil {
		return nil
	}
	failures := Explain(engineOutput)
	if len(failures) == 0 {
		return err
	}
	return &Explained{Cause: err, Failures: failures}
}

// AnnotateError is Annotate for an error whose text already carries the
// engine's output (a test-harness error that captured the command's output).
func AnnotateError(err error) error {
	if err == nil {
		return nil
	}
	return Annotate(err, err.Error())
}

// Explained is an engine error together with the three-part explanations of
// the raw failures its output carried. The cause stays first so nothing the
// engine said is hidden; the explanations follow.
type Explained struct {
	Cause    error
	Failures []*Failure
}

func (e *Explained) Error() string {
	var b strings.Builder
	b.WriteString(e.Cause.Error())
	for _, f := range e.Failures {
		b.WriteString("\n")
		b.WriteString(f.Error())
	}
	return b.String()
}

// Unwrap exposes the cause and every explanation to errors.Is and errors.As.
func (e *Explained) Unwrap() []error {
	out := make([]error, 0, 1+len(e.Failures))
	out = append(out, e.Cause)
	for _, f := range e.Failures {
		out = append(out, f)
	}
	return out
}

// explainer recognizes one raw failure. signature is a phrase from the
// explainer's OWN three-part text; when the engine output already contains it,
// the module spoke first and the explainer stays silent.
type explainer struct {
	signature string
	explain   func(text string) *Failure
}

var explainers = []explainer{
	{signature: signatureHelmStaleRepositoryCache, explain: explainHelmStaleRepositoryCache},
	{signature: signatureHelmOCIVersionNotPublished, explain: explainHelmOCIVersionNotPublished},
	{signature: signatureHelmVersionNotPublished, explain: explainHelmVersionNotPublished},
	{signature: signatureHelmRepositoryUnreachable, explain: explainHelmRepositoryUnreachable},
	{signature: signatureKubernetesForbidden, explain: explainKubernetesForbidden},
	{signature: signatureKubernetesFieldNotInDefinition, explain: explainKubernetesFieldNotInDefinition},
}

// collapse removes the engines' presentation from a diagnostic: OpenTofu and
// Terraform frame errors in box-drawing characters and wrap at terminal width,
// the e2e runner joins lines with " | ", and Pulumi indents. What remains is
// one line of single-spaced words, which is what the patterns below match.
func collapse(text string) string {
	replaced := strings.NewReplacer("│", " ", "╷", " ", "╵", " ", "─", " ", " | ", " ").Replace(text)
	return strings.Join(strings.Fields(replaced), " ")
}

// Helm's texts, identical from the helm provider's data sources and from the
// Helm SDK (the provider is a thin wrapper), so one pattern each.
var (
	// chart "podinfo" version "99.99.99" not found in https://stefanprodan.github.io/podinfo repository
	helmVersionNotPublishedPattern = regexp.MustCompile(`chart "([^"]+)" version "([^"]+)" not found in (\S+) repository`)
	// failed to perform "FetchReference" on source: ghcr.io/plantonhq/charts/planton-operator:99.99.99: not found
	helmOCIVersionNotPublishedPattern = regexp.MustCompile(`failed to perform "FetchReference" on source: ([^\s:"]+):([^\s:"]+): not found`)
	// looks like "https://charts.example.invalid" is not a valid chart repository or cannot be reached: Get "...": dial tcp: lookup ...: no such host
	// The transport's own reason follows the colon and is kept, up to the next
	// diagnostic or the end of the output.
	helmRepositoryUnreachablePattern = regexp.MustCompile(`looks like "([^"]+)" is not a valid chart repository or cannot be reached(?:: .{0,300}?)?(?: Error:|$)`)
	// Error making request: GET https://charts.example.invalid/index.yaml giving up after 1 attempt(s): Get "...": dial tcp: ... no such host
	httpIndexUnreachablePattern = regexp.MustCompile(`Error making request: GET (\S+?)/index\.yaml giving up after \d+ attempt\(s\)(?:: .{0,300}?)?(?: Error:|$)`)
	// no cached repo found. (try 'helm repo update'): open .../neo4j-index.yaml: no such file or directory
	helmStaleRepositoryCachePattern = regexp.MustCompile(`no cached repo found`)
	// Unable to locate chart podinfo: ...
	helmLocateChartPattern = regexp.MustCompile(`Unable to locate chart ([^\s:]+)`)
)

func explainHelmVersionNotPublished(text string) *Failure {
	m := helmVersionNotPublishedPattern.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	return HelmVersionNotPublished(m[3], m[1], m[2], m[0])
}

func explainHelmOCIVersionNotPublished(text string) *Failure {
	m := helmOCIVersionNotPublishedPattern.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	// The reference is <registry>/<path>/<chart>; the repository the module
	// knows is oci://<registry>/<path>.
	reference := m[1]
	chart := reference
	repository := "oci://" + reference
	if i := strings.LastIndex(reference, "/"); i > 0 {
		chart = reference[i+1:]
		repository = "oci://" + reference[:i]
	}
	return HelmOCIVersionNotPublished(repository, chart, m[2], m[0])
}

func explainHelmRepositoryUnreachable(text string) *Failure {
	if m := helmRepositoryUnreachablePattern.FindStringSubmatch(text); m != nil {
		return HelmRepositoryUnreachable(m[1], strings.TrimSuffix(m[0], " Error:"))
	}
	if m := httpIndexUnreachablePattern.FindStringSubmatch(text); m != nil {
		return HelmRepositoryUnreachable(m[1], strings.TrimSuffix(m[0], " Error:"))
	}
	return nil
}

func explainHelmStaleRepositoryCache(text string) *Failure {
	m := helmStaleRepositoryCachePattern.FindString(text)
	if m == "" {
		return nil
	}
	chart := "the chart"
	if c := helmLocateChartPattern.FindStringSubmatch(text); c != nil {
		chart = c[1]
	}
	return HelmStaleRepositoryCache(chart, m)
}

// The API server's authorization refusal, as every client surfaces it:
// customresourcedefinitions.apiextensions.k8s.io "x" is forbidden: User "u" cannot patch resource "customresourcedefinitions" in API group "apiextensions.k8s.io" at the cluster scope
// pods "x" is forbidden: User "u" cannot create resource "pods" in API group "" in the namespace "ns"
var kubernetesForbiddenPattern = regexp.MustCompile(`is forbidden: User "([^"]+)" cannot (\w+) resource "([^"]+)" in API group "([^"]*)" (at the cluster scope|in the namespace "[^"]+")`)

func explainKubernetesForbidden(text string) *Failure {
	m := kubernetesForbiddenPattern.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	return KubernetesForbidden(m[1], m[2], m[3], m[4], m[5], m[0])
}

// The API server's refusal of a field the kind's installed definition does
// not declare, in the two shapes it takes. Server-side apply (the kubectl
// provider, Pulumi's kubernetes provider) fails the typed conversion:
// failed to create typed patch object (planton/planton; planton.ai/v1, Kind=PlantonPlatform): .spec.ingress.gatewayRef: field not declared in schema
// A create or update with strict field validation fails the decoder:
// PlantonPlatform in version "v1" cannot be handled as a PlantonPlatform: strict decoding error: unknown field "spec.ingress.gatewayRef"
var (
	kubernetesFieldNotInSchemaPattern   = regexp.MustCompile(`failed to create typed patch object \([^;]+; [^,]+, Kind=(\w+)\): \.([\w.\[\]]+): field not declared in schema`)
	kubernetesStrictUnknownFieldPattern = regexp.MustCompile(`(\w+) in version "[^"]+" cannot be handled as a \w+: strict decoding error: unknown field "([^"]+)"`)
)

func explainKubernetesFieldNotInDefinition(text string) *Failure {
	if m := kubernetesFieldNotInSchemaPattern.FindStringSubmatch(text); m != nil {
		return KubernetesFieldNotInDefinition(m[1], m[2], m[0])
	}
	if m := kubernetesStrictUnknownFieldPattern.FindStringSubmatch(text); m != nil {
		return KubernetesFieldNotInDefinition(m[1], m[2], m[0])
	}
	return nil
}
