package source

import (
	"bytes"
	"context"
	"os"

	"github.com/go-git/go-git/v5/plumbing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

var _ = Describe("git.Unpack", func() {
	It("should fail if bundle source type is not git", func() {
		cfg := ctrl.GetConfigOrDie()
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{})
		Expect(err).To(BeNil())
		git := Git{mgr.GetClient(), "fakenamespace"}
		bundle := &rukpakv1alpha1.Bundle{
			Spec: rukpakv1alpha1.BundleSpec{
				Source: rukpakv1alpha1.BundleSource{
					Type: "faketype",
				},
			},
		}
		_, err = git.Unpack(context.TODO(), bundle)
		Expect(err.Error()).To(ContainSubstring(`bundle source type "faketype" not supported`))
	})
	It("should fail if bundle git source is not set", func() {
		cfg := ctrl.GetConfigOrDie()
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{})
		Expect(err).To(BeNil())
		git := Git{mgr.GetClient(), "fakenamespace"}
		bundle := &rukpakv1alpha1.Bundle{
			Spec: rukpakv1alpha1.BundleSpec{
				Source: rukpakv1alpha1.BundleSource{
					Type: rukpakv1alpha1.SourceTypeGit,
				},
			},
		}
		_, err = git.Unpack(context.TODO(), bundle)
		Expect(err.Error()).To(ContainSubstring(`bundle source git configuration is unset`))
	})

})

var _ = Describe("git.createCloneOptions", func() {
	It("should fail if bundle git source does not contain a repository", func() {
		cfg := ctrl.GetConfigOrDie()
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{})
		Expect(err).To(BeNil())
		git := Git{mgr.GetClient(), "fakenamespace"}
		bundle := &rukpakv1alpha1.Bundle{
			Spec: rukpakv1alpha1.BundleSpec{
				Source: rukpakv1alpha1.BundleSource{
					Type: rukpakv1alpha1.SourceTypeGit,
					Git: &rukpakv1alpha1.GitSource{
						Repository: "",
					},
				},
			},
		}
		cloneOpts, err := git.createCloneOptions(bundle, bytes.Buffer{})
		Expect(cloneOpts).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("missing git source information: repository must be provided"))
	})
	It("should pass with minimal arguments", func() {
		cfg := ctrl.GetConfigOrDie()
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{})
		Expect(err).To(BeNil())
		git := Git{mgr.GetClient(), "fakenamespace"}
		bundle := &rukpakv1alpha1.Bundle{
			Spec: rukpakv1alpha1.BundleSpec{
				Source: rukpakv1alpha1.BundleSource{
					Type: rukpakv1alpha1.SourceTypeGit,
					Git: &rukpakv1alpha1.GitSource{
						Repository: "therepo",
					},
				},
			},
		}
		cloneOpts, err := git.createCloneOptions(bundle, bytes.Buffer{})
		Expect(err).To(BeNil())
		Expect(cloneOpts.URL).To(Equal("therepo"))
	})
	It("should set Reference name to Branch", func() {
		cfg := ctrl.GetConfigOrDie()
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{})
		Expect(err).To(BeNil())
		git := Git{mgr.GetClient(), "fakenamespace"}
		bundle := &rukpakv1alpha1.Bundle{
			Spec: rukpakv1alpha1.BundleSpec{
				Source: rukpakv1alpha1.BundleSource{
					Type: rukpakv1alpha1.SourceTypeGit,
					Git: &rukpakv1alpha1.GitSource{
						Repository: "therepo",
						Ref: rukpakv1alpha1.GitRef{
							Branch: "thebranch",
						},
					},
				},
			},
		}
		cloneOpts, err := git.createCloneOptions(bundle, bytes.Buffer{})
		Expect(err).To(BeNil())
		Expect(cloneOpts.ReferenceName).To(Equal(plumbing.ReferenceName("refs/heads/thebranch")))
	})
	It("should set Reference name to Tag", func() {
		cfg := ctrl.GetConfigOrDie()
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{})
		Expect(err).To(BeNil())
		git := Git{mgr.GetClient(), "fakenamespace"}
		bundle := &rukpakv1alpha1.Bundle{
			Spec: rukpakv1alpha1.BundleSpec{
				Source: rukpakv1alpha1.BundleSource{
					Type: rukpakv1alpha1.SourceTypeGit,
					Git: &rukpakv1alpha1.GitSource{
						Repository: "therepo",
						Ref: rukpakv1alpha1.GitRef{
							Tag: "thetag",
						},
					},
				},
			},
		}
		cloneOpts, err := git.createCloneOptions(bundle, bytes.Buffer{})
		Expect(err).To(BeNil())
		Expect(cloneOpts.ReferenceName).To(Equal(plumbing.ReferenceName("refs/tags/thetag")))
	})
	It("should set not accept both Branch and Tag being set at the same time", func() {
		cfg := ctrl.GetConfigOrDie()
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{})
		Expect(err).To(BeNil())
		git := Git{mgr.GetClient(), "fakenamespace"}
		bundle := &rukpakv1alpha1.Bundle{
			Spec: rukpakv1alpha1.BundleSpec{
				Source: rukpakv1alpha1.BundleSource{
					Type: rukpakv1alpha1.SourceTypeGit,
					Git: &rukpakv1alpha1.GitSource{
						Repository: "therepo",
						Ref: rukpakv1alpha1.GitRef{
							Tag:    "thetag",
							Branch: "thebranch",
						},
					},
				},
			},
		}
		cloneOpts, err := git.createCloneOptions(bundle, bytes.Buffer{})
		Expect(cloneOpts).To(BeNil())
		Expect(err.Error()).To(ContainSubstring("git branch and tag cannot be set simultaneously"))
	})
})
var _ = Describe("git.configAuth", func() {

	It("should be a no-op if there is no Secret", func() {
		cfg := ctrl.GetConfigOrDie()
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{})
		Expect(err).To(BeNil())
		git := Git{mgr.GetClient(), "fakenamespace"}
		bundle := &rukpakv1alpha1.Bundle{
			Spec: rukpakv1alpha1.BundleSpec{
				Source: rukpakv1alpha1.BundleSource{
					Type: rukpakv1alpha1.SourceTypeGit,
					Git: &rukpakv1alpha1.GitSource{
						Repository: "therepo",
					},
				},
			},
		}
		auth, err := git.configAuth(context.TODO(), bundle)
		Expect(err).To(BeNil())
		Expect(auth).To(BeNil())
	})
	It("should set basic auth from Secret", func() {
		schemeBuilder := runtime.NewSchemeBuilder(
			kscheme.AddToScheme,
			rukpakv1alpha1.AddToScheme,
		)
		scheme := runtime.NewScheme()
		Expect(schemeBuilder.AddToScheme(scheme)).ShouldNot(HaveOccurred())

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "secrets",
				Name:      "secrets",
			},
		}
		Expect(client.Create(context.TODO(), ns)).To(Succeed())

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitsecret",
				Namespace: "secrets",
			},
			Data: map[string][]byte{"username": []byte("itsme"), "password": []byte("wouldntyouliketoknow")},
		}
		Expect(client.Create(context.TODO(), secret)).To(Succeed())

		git := Git{client, "secrets"}
		bundle := &rukpakv1alpha1.Bundle{
			Spec: rukpakv1alpha1.BundleSpec{
				Source: rukpakv1alpha1.BundleSource{
					Type: rukpakv1alpha1.SourceTypeGit,
					Git: &rukpakv1alpha1.GitSource{
						Repository: "http://starts.with.http.edu",
						Auth: rukpakv1alpha1.Authorization{
							Secret: corev1.LocalObjectReference{
								Name: "gitsecret",
							},
						},
					},
				},
			},
		}
		auth, err := git.configAuth(context.TODO(), bundle)
		Expect(err).To(BeNil())
		// Password is masked
		Expect(auth.String()).To(ContainSubstring("itsme:*******"))
	})
	It("should use a secure cert from Secret", func() {
		schemeBuilder := runtime.NewSchemeBuilder(
			kscheme.AddToScheme,
			rukpakv1alpha1.AddToScheme,
		)
		scheme := runtime.NewScheme()
		Expect(schemeBuilder.AddToScheme(scheme)).ShouldNot(HaveOccurred())

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "secrets",
				Name:      "secrets",
			},
		}
		Expect(client.Create(context.TODO(), ns)).To(Succeed())

		privateKey, _ := os.ReadFile("../../testdata/source/git/private-key.pem")
		knownhosts, _ := os.ReadFile("../../testdata/source/git/known")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitsecret",
				Namespace: "secrets",
			},
			Data: map[string][]byte{"ssh-privatekey": privateKey, "ssh-knownhosts": knownhosts},
		}
		Expect(client.Create(context.TODO(), secret)).To(Succeed())

		git := Git{client, "secrets"}
		bundle := &rukpakv1alpha1.Bundle{
			Spec: rukpakv1alpha1.BundleSpec{
				Source: rukpakv1alpha1.BundleSource{
					Type: rukpakv1alpha1.SourceTypeGit,
					Git: &rukpakv1alpha1.GitSource{
						Repository: "doesnt.start.with.http.edu",
						Auth: rukpakv1alpha1.Authorization{
							Secret: corev1.LocalObjectReference{
								Name: "gitsecret",
							},
						},
					},
				},
			},
		}
		auth, err := git.configAuth(context.TODO(), bundle)
		Expect(err).To(BeNil())
		Expect(auth).To(ContainSubstring("user: git, name: ssh-public-keys"))

	})
	It("should use a cert with SkipVerify", func() {
		schemeBuilder := runtime.NewSchemeBuilder(
			kscheme.AddToScheme,
			rukpakv1alpha1.AddToScheme,
		)
		scheme := runtime.NewScheme()
		Expect(schemeBuilder.AddToScheme(scheme)).ShouldNot(HaveOccurred())

		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "secrets",
				Name:      "secrets",
			},
		}
		Expect(client.Create(context.TODO(), ns)).To(Succeed())

		privateKey, _ := os.ReadFile("../../testdata/source/git/private-key.pem")
		knownhosts, _ := os.ReadFile("../../testdata/source/git/known")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gitsecret",
				Namespace: "secrets",
			},
			Data: map[string][]byte{"ssh-privatekey": privateKey, "ssh-knownhosts": knownhosts},
		}
		Expect(client.Create(context.TODO(), secret)).To(Succeed())

		git := Git{client, "secrets"}
		bundle := &rukpakv1alpha1.Bundle{
			Spec: rukpakv1alpha1.BundleSpec{
				Source: rukpakv1alpha1.BundleSource{
					Type: rukpakv1alpha1.SourceTypeGit,
					Git: &rukpakv1alpha1.GitSource{
						Repository: "doesnt.start.with.http.edu",
						Auth: rukpakv1alpha1.Authorization{
							Secret: corev1.LocalObjectReference{
								Name: "gitsecret",
							},
							InsecureSkipVerify: true,
						},
					},
				},
			},
		}
		auth, err := git.configAuth(context.TODO(), bundle)
		Expect(err).To(BeNil())
		// Is there anything I can check to ensure this is set?
		Expect(auth).To(ContainSubstring("user: git, name: ssh-public-keys"))
	})
})
