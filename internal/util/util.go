package util

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

func BundleProvisionerFilter(provisionerClassName string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		b := obj.(*rukpakv1alpha1.Bundle)
		return b.Spec.ProvisionerClassName == provisionerClassName
	})
}

func BundleInstanceProvisionerFilter(provisionerClassName string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		b := obj.(*rukpakv1alpha1.BundleInstance)
		return b.Spec.ProvisionerClassName == provisionerClassName
	})
}

func MapBundleToBundleInstanceHandler(cl client.Client, log logr.Logger) handler.MapFunc {
	return func(object client.Object) []reconcile.Request {
		b := object.(*rukpakv1alpha1.Bundle)

		bundleInstances := &rukpakv1alpha1.BundleInstanceList{}
		if err := cl.List(context.Background(), bundleInstances); err != nil {
			log.WithName("mapBundleToBundleInstanceHandler").Error(err, "list bundles")
			return nil
		}
		var requests []reconcile.Request
		for _, bi := range bundleInstances.Items {
			bi := bi
			for _, ref := range b.GetOwnerReferences() {
				if ref.Name == bi.GetName() {
					requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&bi)})
				}
			}
		}
		return requests
	}
}

// GetBundlesForBundleInstanceSelector is responsible for returning a list of
// Bundle resource that exist on cluster that match the label selector specified
// in the BI parameter's spec.Selector field.
func GetBundlesForBundleInstanceSelector(ctx context.Context, c client.Client, bi *rukpakv1alpha1.BundleInstance) ([]*rukpakv1alpha1.Bundle, error) {
	bundleSelector, err := metav1.LabelSelectorAsSelector(bi.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the %s label selector: %w", bi.Spec.Selector.String(), err)
	}
	bundleList := &rukpakv1alpha1.BundleList{}
	if err := c.List(ctx, bundleList, &client.ListOptions{
		LabelSelector: bundleSelector,
	}); err != nil {
		return nil, fmt.Errorf("failed to list bundles using the %s selector: %w", bundleSelector.String(), err)
	}
	bundles := []*rukpakv1alpha1.Bundle{}
	for _, b := range bundleList.Items {
		bundles = append(bundles, b.DeepCopy())
	}
	return bundles, nil
}

// CheckExistingBundlesMatchesTemplate evaluates whether the existing list of Bundle objects
// match the desired Bundle template that's specified in a BundleInstance object. If a match
// is found, that Bundle object is returned, so callers are responsible for nil checking the result.
func CheckExistingBundlesMatchesTemplate(existingBundles []*rukpakv1alpha1.Bundle, desiredBundleTemplate *rukpakv1alpha1.BundleTemplate) *rukpakv1alpha1.Bundle {
	for _, bundle := range existingBundles {
		if !CheckDesiredBundleTemplate(bundle, desiredBundleTemplate) {
			continue
		}
		return bundle
	}
	return nil
}

// CheckDesiredBundleTemplate is responsible for determining whether the existingBundle
// parameter is semantically equal to the desiredBundle Bundle template.
func CheckDesiredBundleTemplate(existingBundle *rukpakv1alpha1.Bundle, desiredBundle *rukpakv1alpha1.BundleTemplate) bool {
	return equality.Semantic.DeepEqual(existingBundle.Spec, desiredBundle.Spec) &&
		equality.Semantic.DeepEqual(existingBundle.Annotations, desiredBundle.Annotations) &&
		equality.Semantic.DeepEqual(existingBundle.Labels, desiredBundle.Labels)
}

// GenerateBundleName is responsible for generating a unique
// Bundle metadata.Name using the biName parameter as the base.
func GenerateBundleName(biName string) string {
	return fmt.Sprintf("%s-%s", biName, rand.String(5))
}

// GetPodNamespace checks whether the controller is running in a Pod vs.
// being run locally by inspecting the namespace file that gets mounted
// automatically for Pods at runtime. If that file doesn't exist, then
// return the @defaultNamespace namespace parameter.
func PodNamespace(defaultNamespace string) string {
	namespace, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return defaultNamespace
	}
	return string(namespace)
}

func PodName(provisionerName, bundleName string) string {
	return fmt.Sprintf("%s-unpack-bundle-%s", provisionerName, bundleName)
}

func BundleLabels(bundleName string) map[string]string {
	return map[string]string{"core.rukpak.io/bundle-name": bundleName}
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

// NewBundleLabelSelector is responsible for constructing a label.Selector
// for any underlying resources that are associated with the Bundle parameter.
func NewBundleLabelSelector(bundle *rukpakv1alpha1.Bundle) labels.Selector {
	return newLabelSelector(bundle.GetName(), rukpakv1alpha1.BundleKind)
}

// NewBundleInstanceLabelSelector is responsible for constructing a label.Selector
// for any underlying resources that are associated with the BundleInstance parameter.
func NewBundleInstanceLabelSelector(bi *rukpakv1alpha1.BundleInstance) labels.Selector {
	return newLabelSelector(bi.GetName(), rukpakv1alpha1.BundleInstanceKind)
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

	if err := wait.PollImmediateUntil(time.Millisecond*5, func() (done bool, err error) {
		if err := cl.Delete(ctx, obj); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}, ctx.Done()); err != nil {
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
