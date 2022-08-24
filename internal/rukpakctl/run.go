package rukpakctl

import (
	"context"
	"fmt"
	"hash/fnv"
	"io/fs"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	plain "github.com/operator-framework/rukpak/internal/provisioner/plain/types"
	"github.com/operator-framework/rukpak/internal/util"
)

// Run implements rukpakctl's `run` subcommand
type Run struct {
	Config *rest.Config

	SystemNamespace   string
	UploadServiceName string
	CASecretName      string
}

// RunOptions define extra options used for Run.
type RunOptions struct {
	BundleDeploymentProvisionerClassName string
	BundleProvisionerClassName           string
	Log                                  func(format string, v ...interface{})
}

// Run runs the provided bundle using a bundle deployment with the given bundleDeploymentName.
// The RunOptions enable further configuration, such as the provisioner class names to use and
// an optional logger. By default, the plain provisioner for the bundle and bundle deployment.
// Run returns a boolean value indicating whether the bundle deployment was created or modified
// and an error value if any error occurs.
func (r *Run) Run(ctx context.Context, bundleDeploymentName string, bundle fs.FS, opts RunOptions) (bool, error) {
	if opts.BundleDeploymentProvisionerClassName == "" {
		opts.BundleDeploymentProvisionerClassName = plain.ProvisionerID
	}

	if opts.BundleProvisionerClassName == "" {
		opts.BundleProvisionerClassName = plain.ProvisionerID
	}
	if opts.Log == nil {
		opts.Log = func(_ string, _ ...interface{}) {}
	}

	sch := scheme.Scheme
	if err := rukpakv1alpha1.AddToScheme(sch); err != nil {
		return false, err
	}
	cl, err := client.New(r.Config, client.Options{Scheme: sch})
	if err != nil {
		return false, err
	}

	digest := fnv.New64a()
	if err := util.FSToTarGZ(digest, bundle); err != nil {
		return false, err
	}

	bundleLabels := map[string]string{
		"app":          bundleDeploymentName,
		"bundleDigest": fmt.Sprintf("%x", digest.Sum(nil)),
	}

	bd := buildBundleDeployment(bundleDeploymentName, bundleLabels, opts.BundleDeploymentProvisionerClassName, opts.BundleProvisionerClassName)
	if err := cl.Patch(ctx, bd, client.Apply, client.ForceOwnership, client.FieldOwner("rukpakctl")); err != nil {
		return false, fmt.Errorf("apply bundle deployment: %v", err)
	}
	opts.Log("bundledeployment.core.rukpak.io %q applied\n", bundleDeploymentName)

	dynCl, err := dynamic.NewForConfig(r.Config)
	if err != nil {
		return false, fmt.Errorf("build dynamic client: %v", err)
	}

	bundleName, err := getBundleName(ctx, dynCl, bundleLabels)
	if err != nil {
		return false, fmt.Errorf("failed to get bundle name: %v", err)
	}

	rukpakCA, err := GetClusterCA(ctx, cl, types.NamespacedName{Namespace: r.SystemNamespace, Name: r.CASecretName})
	if err != nil {
		return false, err
	}

	bu := BundleUploader{
		UploadServiceNamespace: r.SystemNamespace,
		UploadServiceName:      r.UploadServiceName,
		Cfg:                    r.Config,
		RootCAs:                rukpakCA,
	}
	modified, err := bu.Upload(ctx, bundleName, bundle)
	if err != nil {
		return false, fmt.Errorf("failed to upload bundle: %v", err)
	}
	if !modified {
		opts.Log("bundle %q is already up-to-date\n", bundleName)
	} else {
		opts.Log("successfully uploaded bundle content for %q\n", bundleName)
	}
	return modified, nil
}

func buildBundleDeployment(bdName string, bundleLabels map[string]string, biPCN, bPNC string) *unstructured.Unstructured {
	// We use unstructured here to avoid problems of serializing default values when sending patches to the apiserver.
	// If you use a typed object, any default values from that struct get serialized into the JSON patch, which could
	// cause unrelated fields to be patched back to the default value even though that isn't the intention. Using an
	// unstructured ensures that the patch contains only what is specified. Using unstructured like this is basically
	// identical to "kubectl apply -f"
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": rukpakv1alpha1.GroupVersion.String(),
		"kind":       rukpakv1alpha1.BundleDeploymentKind,
		"metadata": map[string]interface{}{
			"name": bdName,
		},
		"spec": map[string]interface{}{
			"provisionerClassName": biPCN,
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": bundleLabels,
				},
				"spec": map[string]interface{}{
					"provisionerClassName": bPNC,
					"source": map[string]interface{}{
						"type":   rukpakv1alpha1.SourceTypeUpload,
						"upload": &rukpakv1alpha1.UploadSource{},
					},
				},
			},
		},
	}}
}

func getBundleName(ctx context.Context, dynCl dynamic.Interface, bundleLabels map[string]string) (string, error) {
	watch, err := dynCl.Resource(rukpakv1alpha1.GroupVersion.WithResource("bundles")).Watch(ctx, metav1.ListOptions{Watch: true, LabelSelector: labels.FormatLabels(bundleLabels)})
	if err != nil {
		return "", fmt.Errorf("watch bundles: %v", err)
	}
	defer watch.Stop()

	select {
	case evt := <-watch.ResultChan():
		return evt.Object.(client.Object).GetName(), nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
