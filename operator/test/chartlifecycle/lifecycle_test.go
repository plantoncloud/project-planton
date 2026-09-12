//go:build e2e

package chartlifecycle

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/chart"
)

const (
	operatorNamespace = "planton-chart-lifecycle"
	operatorRelease   = "planton-operator"

	platformNamespace = "planton-chart-lifecycle-platform"
	platformRelease   = "planton"
)

// scaledToZero installs a published chart without running its operator: the
// suite proves ownership and schema, never a platform boot.
var scaledToZero = map[string]any{"replicaCount": 0}

func workingTreeImage() map[string]any {
	repo, tag, _ := strings.Cut(managerImage, ":")
	return map[string]any{"image": map[string]any{"repository": repo, "tag": tag}}
}

func merged(maps ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

var _ = Describe("the operator chart owns its definitions", Ordered, func() {
	var operatorChart, platformChart *chart.Chart

	BeforeAll(func() {
		operatorChart = workingTreeChart("planton-operator")
		platformChart = workingTreeChart("planton")
		clean(operatorNamespace, platformNamespace)
	})

	AfterEach(func() {
		h := newHelm(operatorNamespace)
		Expect(h.uninstall(operatorRelease)).To(Succeed())
		Expect(h.uninstall(operatorRelease + "-two")).To(Succeed())
		Expect(newHelm(platformNamespace).uninstall(platformRelease)).To(Succeed())
		clean(operatorNamespace, platformNamespace)
	})

	It("installs and owns both definitions, starts the manager against them, keeps them across an uninstall, and adopts them on reinstall", func() {
		h := newHelm(operatorNamespace)

		By("installing the working-tree chart with the working-tree image")
		Expect(h.install(operatorRelease, operatorChart, workingTreeImage())).To(Succeed())

		By("both definitions are established and owned by the release, with the keep policy")
		for _, crd := range []string{platformCRD, identityProviderCRD} {
			_, err := kubectl("wait", "--for=condition=Established", "crd/"+crd, "--timeout=60s")
			Expect(err).NotTo(HaveOccurred())
			Expect(ownedBy(crd, operatorRelease, operatorNamespace)).To(BeTrue(), crd+" must carry Helm ownership for this release")
			Expect(crdAnnotation(crd, "helm.sh/resource-policy")).To(Equal("keep"))
		}

		By("the manager starts with both event sources, without Helm waiting for the definitions")
		_, err := kubectl("wait", "--for=condition=Available", "deployment/"+operatorRelease, "-n", operatorNamespace, "--timeout=180s")
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() string {
			logs, _ := kubectl("logs", "deployment/"+operatorRelease, "-n", operatorNamespace)
			return logs
		}, 2*time.Minute, 2*time.Second).Should(ContainSubstring("Starting workers"))

		By("a second operator release on the same cluster is refused with the owner named")
		err = h.install(operatorRelease+"-two", operatorChart, scaledToZero)
		Expect(err).To(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "the chart said:\n%s\n", err)
		Expect(err.Error()).To(ContainSubstring("is owned by Helm release " + operatorRelease + " in namespace " + operatorNamespace))
		Expect(err.Error()).To(ContainSubstring("--set crds.enabled=false"))

		By("uninstalling keeps both definitions")
		Expect(h.uninstall(operatorRelease)).To(Succeed())
		Expect(crdExists(platformCRD)).To(BeTrue())
		Expect(crdExists(identityProviderCRD)).To(BeTrue())

		By("reinstalling under the same release adopts them")
		Expect(h.install(operatorRelease, operatorChart, merged(workingTreeImage(), scaledToZero))).To(Succeed())
		Expect(ownedBy(platformCRD, operatorRelease, operatorNamespace)).To(BeTrue())
	})

	It("removes the definitions on uninstall when crds.keep=false", func() {
		h := newHelm(operatorNamespace)
		Expect(h.install(operatorRelease, operatorChart, merged(scaledToZero, map[string]any{"crds": map[string]any{"keep": false}}))).To(Succeed())
		Expect(crdExists(platformCRD)).To(BeTrue())
		Expect(crdAnnotation(platformCRD, "helm.sh/resource-policy")).To(BeEmpty())

		Expect(h.uninstall(operatorRelease)).To(Succeed())
		Eventually(func() bool { return crdExists(platformCRD) || crdExists(identityProviderCRD) }, time.Minute, time.Second).Should(BeFalse())
	})

	It("adopts a definition that an earlier chart installed once, and the upgrade carries the new schema", func() {
		h := newHelm(operatorNamespace)

		By("installing the last chart that used Helm's install-once crds/ directory")
		Expect(h.install(operatorRelease, publishedChart("planton-operator", lastInstallOnceOperatorChart), scaledToZero)).To(Succeed())
		Expect(crdExists(platformCRD)).To(BeTrue())
		Expect(crdAnnotation(platformCRD, "meta.helm.sh/release-name")).To(BeEmpty(), "an install-once definition belongs to no release")
		Expect(clusterSchema(platformCRD)).NotTo(Equal(controllerGenSchema("planton.ai_plantonplatforms.yaml")), "the published schema must differ from the working tree's for this proof to mean anything")

		By("upgrading to the working-tree chart stops with the adoption explained")
		err := h.upgrade(operatorRelease, operatorChart, scaledToZero)
		Expect(err).To(HaveOccurred())
		_, _ = fmt.Fprintf(GinkgoWriter, "the chart said:\n%s\n", err)
		Expect(err.Error()).To(ContainSubstring("observed: CustomResourceDefinition " + platformCRD + " exists on this cluster but belongs to no Helm release"))
		Expect(err.Error()).To(ContainSubstring("kubectl label crd " + platformCRD + " app.kubernetes.io/managed-by=Helm"))
		Expect(err.Error()).To(ContainSubstring("kubectl annotate crd " + platformCRD + " meta.helm.sh/release-name=" + operatorRelease + " meta.helm.sh/release-namespace=" + operatorNamespace))

		By("running the printed commands verbatim completes the adoption")
		Expect(runKubectlLinesOf(err.Error())).To(Equal(2))
		Expect(h.upgrade(operatorRelease, operatorChart, scaledToZero)).To(Succeed())

		By("both definitions are owned and the platform schema is the working tree's")
		Expect(ownedBy(platformCRD, operatorRelease, operatorNamespace)).To(BeTrue())
		Expect(ownedBy(identityProviderCRD, operatorRelease, operatorNamespace)).To(BeTrue())
		Expect(crdAnnotation(platformCRD, "helm.sh/resource-policy")).To(Equal("keep"))
		Expect(clusterSchema(platformCRD)).To(Equal(controllerGenSchema("planton.ai_plantonplatforms.yaml")))
	})

	It("moves an install that bundled the operator to two releases without running two operators", func() {
		platform := newHelm(platformNamespace)
		operator := newHelm(platformNamespace)

		By("installing the last platform chart that bundled the operator")
		bundling := publishedChart("planton", lastBundlingPlatformChart)
		Expect(platform.install(platformRelease, bundling, map[string]any{"planton-operator": scaledToZero})).To(Succeed())
		_, err := kubectl("get", "plantonplatform", platformRelease, "-n", platformNamespace)
		Expect(err).NotTo(HaveOccurred(), "the bundled install declares a platform")
		Expect(deploymentsLabelled(platformNamespace, "app.kubernetes.io/name=planton-operator")).To(Equal(1), "the bundled install carries an operator")
		Expect(crdAnnotation(platformCRD, "meta.helm.sh/release-name")).To(BeEmpty())

		By("step 1: the definition is handed to the future operator release")
		_, err = kubectl("label", "crd", platformCRD, "app.kubernetes.io/managed-by=Helm")
		Expect(err).NotTo(HaveOccurred())
		_, err = kubectl("annotate", "crd", platformCRD, "meta.helm.sh/release-name="+operatorRelease, "meta.helm.sh/release-namespace="+platformNamespace)
		Expect(err).NotTo(HaveOccurred())

		By("step 2: upgrading the platform release removes the bundled operator and keeps the platform at the version it runs")
		// The bundling chart defaulted spec.version to its appVersion; the
		// pin-free chart requires the version named, so the move passes the
		// one the platform already runs and changes the release shape only.
		runningVersion := map[string]any{"platform": map[string]any{"spec": map[string]any{"version": bundling.Metadata.AppVersion}}}
		Expect(platform.upgrade(platformRelease, platformChart, runningVersion)).To(Succeed())
		Eventually(func() int { return deploymentsLabelled(platformNamespace, "app.kubernetes.io/name=planton-operator") }, time.Minute, time.Second).Should(Equal(0))
		_, err = kubectl("get", "plantonplatform", platformRelease, "-n", platformNamespace)
		Expect(err).NotTo(HaveOccurred(), "the platform resource must survive the move")
		Expect(crdExists(platformCRD)).To(BeTrue(), "the definition must survive the move")

		By("step 3: the operator installs on its own release, adopts the definition, and upgrades its schema")
		Expect(operator.install(operatorRelease, operatorChart, scaledToZero)).To(Succeed())
		Expect(ownedBy(platformCRD, operatorRelease, platformNamespace)).To(BeTrue())
		Expect(ownedBy(identityProviderCRD, operatorRelease, platformNamespace)).To(BeTrue())
		Expect(clusterSchema(platformCRD)).To(Equal(controllerGenSchema("planton.ai_plantonplatforms.yaml")))

		Expect(operator.uninstall(operatorRelease)).To(Succeed())
	})
})

// deploymentsLabelled counts the Deployments in a namespace matching a label
// selector; it is how the suite sees an operator come and go without knowing
// the name a chart gives it.
func deploymentsLabelled(namespace, selector string) int {
	out, err := kubectl("get", "deployments", "-n", namespace, "-l", selector, "-o", "name")
	if err != nil {
		return -1
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return 0
	}
	return len(strings.Split(out, "\n"))
}
