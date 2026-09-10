/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	plantonaiv1 "github.com/plantonhq/planton/operator/api/v1"
)

// spec.email's admission rules, run in the REAL API server (envtest applies
// the generated CRD), so these pin the rules' semantics and the messages an
// administrator reads at apply time, and the suite bootstrap catches
// malformed CEL before it ships.
var _ = Describe("PlantonPlatform spec.email admission", func() {
	const namespace = "default"
	ctx := context.Background()

	from := plantonaiv1.EmailFromSpec{Address: "no-reply@planton.acme.com"}
	oauth2 := &plantonaiv1.EmailSMTPOAuth2Spec{
		User:            "planton@acme.com",
		TokenURL:        "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
		Scope:           "https://outlook.office365.com/.default",
		ClientID:        "client-id",
		ClientSecretRef: plantonaiv1.SecretKeyRef{Name: "planton-email-oauth", Key: "client-secret"},
	}

	platformWithEmail := func(name string, email *plantonaiv1.EmailSpec) *plantonaiv1.PlantonPlatform {
		return &plantonaiv1.PlantonPlatform{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       plantonaiv1.PlantonPlatformSpec{Version: "v1.0.0", Email: email},
		}
	}

	accept := func(name string, email *plantonaiv1.EmailSpec) *plantonaiv1.PlantonPlatform {
		p := platformWithEmail(name, email)
		Expect(k8sClient.Create(ctx, p)).To(Succeed())
		return p
	}

	reject := func(name string, email *plantonaiv1.EmailSpec, message string) {
		err := k8sClient.Create(ctx, platformWithEmail(name, email))
		Expect(err).To(HaveOccurred(), "the API server must refuse %s", name)
		Expect(err.Error()).To(ContainSubstring(message), "the rejection must explain itself, got: %v", err)
	}

	It("should accept a password relay with the defaults filled in", func() {
		p := accept("cel-email-smtp-password", &plantonaiv1.EmailSpec{
			From: from,
			SMTP: &plantonaiv1.EmailSMTPSpec{Host: "smtp.office365.com", CredentialsSecretName: "planton-email"},
		})
		defer func() { Expect(k8sClient.Delete(ctx, p)).To(Succeed()) }()

		Expect(p.Spec.Email.SMTP.Port).To(Equal(int32(587)), "587 is the submission default")
		Expect(p.Spec.Email.SMTP.Security).To(Equal(plantonaiv1.EmailSMTPSecurityStartTLS), "STARTTLS is the default")
		Expect(p.Spec.Email.From.Name).To(Equal("Planton"), "the display name defaults to the product's")
	})

	It("should accept an unauthenticated internal relay in the clear", func() {
		p := accept("cel-email-smtp-open-relay", &plantonaiv1.EmailSpec{
			From: from,
			SMTP: &plantonaiv1.EmailSMTPSpec{Host: "smtp-relay.corp.acme.com", Port: 25, Security: plantonaiv1.EmailSMTPSecurityNone},
		})
		Expect(k8sClient.Delete(ctx, p)).To(Succeed())
	})

	It("should accept an OAuth2 relay behind a private CA", func() {
		p := accept("cel-email-smtp-oauth2", &plantonaiv1.EmailSpec{
			From: from,
			SMTP: &plantonaiv1.EmailSMTPSpec{
				Host:              "smtp.office365.com",
				OAuth2:            oauth2,
				CABundleSecretRef: &plantonaiv1.SecretKeyRef{Name: "corp-ca", Key: "ca.crt"},
			},
		})
		Expect(k8sClient.Delete(ctx, p)).To(Succeed())
	})

	It("should accept the Resend arm", func() {
		p := accept("cel-email-resend", &plantonaiv1.EmailSpec{
			From:   from,
			Resend: &plantonaiv1.EmailResendSpec{APIKeySecretRef: plantonaiv1.SecretKeyRef{Name: "planton-email", Key: "api-key"}},
		})
		Expect(k8sClient.Delete(ctx, p)).To(Succeed())
	})

	It("should reject both arms and neither arm", func() {
		reject("cel-email-both-arms", &plantonaiv1.EmailSpec{
			From:   from,
			SMTP:   &plantonaiv1.EmailSMTPSpec{Host: "smtp.example.com"},
			Resend: &plantonaiv1.EmailResendSpec{APIKeySecretRef: plantonaiv1.SecretKeyRef{Name: "planton-email", Key: "api-key"}},
		}, "email declares exactly one provider")

		reject("cel-email-no-arm", &plantonaiv1.EmailSpec{From: from}, "email declares exactly one provider")
	})

	It("should reject a password and an OAuth2 grant on the same relay", func() {
		reject("cel-email-two-ways-in", &plantonaiv1.EmailSpec{
			From: from,
			SMTP: &plantonaiv1.EmailSMTPSpec{Host: "smtp.office365.com", CredentialsSecretName: "planton-email", OAuth2: oauth2},
		}, "smtp authenticates one way")
	})

	It("should reject credentials over a plaintext connection", func() {
		reject("cel-email-password-in-the-clear", &plantonaiv1.EmailSpec{
			From: from,
			SMTP: &plantonaiv1.EmailSMTPSpec{Host: "smtp.example.com", Security: plantonaiv1.EmailSMTPSecurityNone, CredentialsSecretName: "planton-email"},
		}, "security: none would send credentials in the clear")

		reject("cel-email-token-in-the-clear", &plantonaiv1.EmailSpec{
			From: from,
			SMTP: &plantonaiv1.EmailSMTPSpec{Host: "smtp.example.com", Security: plantonaiv1.EmailSMTPSecurityNone, OAuth2: oauth2},
		}, "security: none would send credentials in the clear")
	})

	It("should reject a declaration without a sender address", func() {
		reject("cel-email-no-from", &plantonaiv1.EmailSpec{
			SMTP: &plantonaiv1.EmailSMTPSpec{Host: "smtp.example.com"},
		}, "spec.email.from.address")
	})

	It("should reject a security word outside the vocabulary and a port outside the range", func() {
		reject("cel-email-bad-security", &plantonaiv1.EmailSpec{
			From: from,
			SMTP: &plantonaiv1.EmailSMTPSpec{Host: "smtp.example.com", Security: "ssl"},
		}, "spec.email.smtp.security")

		reject("cel-email-bad-port", &plantonaiv1.EmailSpec{
			From: from,
			SMTP: &plantonaiv1.EmailSMTPSpec{Host: "smtp.example.com", Port: 70000},
		}, "spec.email.smtp.port")
	})

	It("should reject an OAuth2 token endpoint that is not https", func() {
		insecure := *oauth2
		insecure.TokenURL = "http://login.example.com/token"
		reject("cel-email-oauth2-http", &plantonaiv1.EmailSpec{
			From: from,
			SMTP: &plantonaiv1.EmailSMTPSpec{Host: "smtp.example.com", OAuth2: &insecure},
		}, "spec.email.smtp.oauth2.tokenUrl")
	})
})
