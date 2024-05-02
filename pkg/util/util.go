package util

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
)

var (
	// ErrMaxGeneratedLimit is the error returned by the BundleDeployment controller
	// when the configured maximum number of Bundles that match a label selector
	// has been reached.
	ErrMaxGeneratedLimit = errors.New("reached the maximum generated Bundle limit")
)

func BundleDeploymentProvisionerFilter(provisionerClassName string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		b := obj.(*rukpakv1alpha2.BundleDeployment)
		return b.Spec.ProvisionerClassName == provisionerClassName
	})
}

type ProvisionerClassNameGetter interface {
	client.Object
	ProvisionerClassName() string
}

// MapOwneeToOwnerProvisionerHandler is a handler implementation that finds an owner reference in the event object that
// references the provided owner. If a reference for the provided owner is found AND that owner's provisioner class name
// matches the provided provisionerClassName, this handler enqueues a request for that owner to be reconciled.
func MapOwneeToOwnerProvisionerHandler(cl client.Client, log logr.Logger, provisionerClassName string, owner ProvisionerClassNameGetter) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		ownerGVK, err := apiutil.GVKForObject(owner, cl.Scheme())
		if err != nil {
			log.Error(err, "map ownee to owner: lookup GVK for owner")
			return nil
		}
		owneeGVK, err := apiutil.GVKForObject(obj, cl.Scheme())
		if err != nil {
			log.Error(err, "map ownee to owner: lookup GVK for ownee")
			return nil
		}

		type ownerInfo struct {
			key types.NamespacedName
			gvk schema.GroupVersionKind
		}
		var oi *ownerInfo

		for _, ref := range obj.GetOwnerReferences() {
			gv, err := schema.ParseGroupVersion(ref.APIVersion)
			if err != nil {
				log.Error(err, fmt.Sprintf("map ownee to owner: parse ownee's owner reference group version %q", ref.APIVersion))
				return nil
			}
			refGVK := gv.WithKind(ref.Kind)
			if refGVK == ownerGVK && ref.Controller != nil && *ref.Controller {
				oi = &ownerInfo{
					key: types.NamespacedName{Name: ref.Name},
					gvk: ownerGVK,
				}
				break
			}
		}
		if oi == nil {
			return nil
		}

		if err := cl.Get(ctx, oi.key, owner); client.IgnoreNotFound(err) != nil {
			log.Info("map ownee to owner: get owner",
				"ownee", client.ObjectKeyFromObject(obj),
				"owneeKind", owneeGVK,
				"owner", oi.key,
				"ownerKind", oi.gvk,
				"error", err.Error(),
			)
			return nil
		}
		if owner.ProvisionerClassName() != provisionerClassName {
			return nil
		}
		return []reconcile.Request{{NamespacedName: oi.key}}
	})
}

func MapConfigMapToBundleDeployment(ctx context.Context, cl client.Client, cmNamespace string, cm corev1.ConfigMap) []*rukpakv1alpha2.BundleDeployment {
	bundleDeploymentList := &rukpakv1alpha2.BundleDeploymentList{}
	if err := cl.List(ctx, bundleDeploymentList); err != nil {
		return nil
	}
	var bs []*rukpakv1alpha2.BundleDeployment
	for _, b := range bundleDeploymentList.Items {
		b := b
		for _, cmSource := range b.Spec.Source.ConfigMaps {
			cmName := cmSource.ConfigMap.Name
			if cm.Name == cmName && cm.Namespace == cmNamespace {
				bs = append(bs, &b)
			}
		}
	}
	return bs
}

func MapConfigMapToBundleDeploymentHandler(cl client.Client, configMapNamespace string, provisionerClassName string) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		cm := object.(*corev1.ConfigMap)
		var requests []reconcile.Request
		matchingBundleDeployment := MapConfigMapToBundleDeployment(ctx, cl, configMapNamespace, *cm)
		for _, b := range matchingBundleDeployment {
			if b.Spec.ProvisionerClassName != provisionerClassName {
				continue
			}
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)})
		}
		return requests
	})
}

const (
	// maxBundleNameLength must be aligned with the Bundle CRD metadata.name length validation, defined in:
	// <repoRoot>/manifests/base/apis/crds/patches/bundle_validation.yaml
	maxBundleNameLength = 52

	// maxBundleDeploymentNameLength must be aligned with the BundleDeployment CRD metadata.name length validation,
	// defined in: <repoRoot>/manifests/base/apis/crds/patches/bundledeployment_validation.yaml
	maxBundleDeploymentNameLength = 45
)

func GenerateBundleName(bdName, hash string) string {
	if len(bdName) > maxBundleDeploymentNameLength {
		// This should never happen because we have validation on the BundleDeployment CRD to ensure
		// that the name is no more than 45 characters. But just to be safe...
		bdName = bdName[:maxBundleDeploymentNameLength]
	}

	if len(hash) > maxBundleNameLength-len(bdName)-1 {
		hash = hash[:maxBundleNameLength-len(bdName)-1]
	}
	return fmt.Sprintf("%s-%s", bdName, hash)
}

// PodNamespace checks whether the controller is running in a Pod vs.
// being run locally by inspecting the namespace file that gets mounted
// automatically for Pods at runtime. If that file doesn't exist, then
// return DefaultSystemNamespace.
func PodNamespace() string {
	namespace, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return DefaultSystemNamespace
	}
	return string(namespace)
}

func newLabelSelector(name, kind string) labels.Selector {
	kindRequirement, err := labels.NewRequirement(CoreOwnerKindKey, selection.Equals, []string{kind})
	if err != nil {
		return nil
	}
	nameRequirement, err := labels.NewRequirement(CoreOwnerNameKey, selection.Equals, []string{name})
	if err != nil {
		return nil
	}
	return labels.NewSelector().Add(*kindRequirement, *nameRequirement)
}

// NewBundleDeploymentLabelSelector is responsible for constructing a label.Selector
// for any underlying resources that are associated with the BundleDeployment parameter.
func NewBundleDeploymentLabelSelector(bd *rukpakv1alpha2.BundleDeployment) labels.Selector {
	return newLabelSelector(bd.GetName(), rukpakv1alpha2.BundleDeploymentKind)
}

func CreateOrRecreate(ctx context.Context, cl client.Client, obj client.Object, f controllerutil.MutateFn) (controllerutil.OperationResult, error) {
	key := client.ObjectKeyFromObject(obj)
	if err := cl.Get(ctx, key, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return controllerutil.OperationResultNone, err
		}
		if err := mutate(f, key, obj); err != nil {
			return controllerutil.OperationResultNone, err
		}
		if err := cl.Create(ctx, obj); err != nil {
			return controllerutil.OperationResultNone, err
		}
		return controllerutil.OperationResultCreated, nil
	}

	existing := obj.DeepCopyObject() //nolint
	if err := mutate(f, key, obj); err != nil {
		return controllerutil.OperationResultNone, err
	}

	if equality.Semantic.DeepEqual(existing, obj) {
		return controllerutil.OperationResultNone, nil
	}

	if err := wait.PollUntilContextCancel(ctx, time.Millisecond*5, true, func(conditionCtx context.Context) (bool, error) {
		if err := cl.Delete(conditionCtx, obj); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}); err != nil {
		return controllerutil.OperationResultNone, err
	}

	obj.SetUID("")
	obj.SetResourceVersion("")
	obj.SetGeneration(0)
	if err := cl.Create(ctx, obj); err != nil {
		return controllerutil.OperationResultNone, err
	}
	return controllerutil.OperationResultUpdated, nil
}

// mutate wraps a MutateFn and applies validation to its result.
func mutate(f controllerutil.MutateFn, key client.ObjectKey, obj client.Object) error {
	if err := f(); err != nil {
		return err
	}
	if newKey := client.ObjectKeyFromObject(obj); key != newKey {
		return fmt.Errorf("MutateFn cannot mutate object name and/or object namespace")
	}
	return nil
}

func MergeMaps(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func LoadCertPool(certFile string) (*x509.CertPool, error) {
	rootCAPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	for block, rest := pem.Decode(rootCAPEM); block != nil; block, rest = pem.Decode(rest) {
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certPool.AddCert(cert)
	}
	return certPool, nil
}

func ManifestObjects(r io.Reader, name string) ([]client.Object, error) {
	result := resource.NewLocalBuilder().Flatten().Unstructured().Stream(r, name).Do()
	if err := result.Err(); err != nil {
		return nil, err
	}
	infos, err := result.Infos()
	if err != nil {
		return nil, err
	}
	return infosToObjects(infos), nil
}

func infosToObjects(infos []*resource.Info) []client.Object {
	objects := make([]client.Object, 0, len(infos))
	for _, info := range infos {
		clientObject := info.Object.(client.Object)
		objects = append(objects, clientObject)
	}
	return objects
}
