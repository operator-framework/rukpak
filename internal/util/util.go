package util

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

const (
	maxGeneratedBundleLimit = 4
)

var (
	// ErrMaxGeneratedLimit is the error returned by the BundleDeployment controller
	// when the configured maximum number of Bundles that match a label selector
	// has been reached.
	ErrMaxGeneratedLimit = errors.New("reached the maximum generated Bundle limit")
)

// reconcileDesiredBundle is responsible for checking whether the desired
// Bundle resource that's specified in the BundleDeployment parameter's
// spec.Template configuration is present on cluster, and if not, creates
// a new Bundle resource matching that desired specification.
func ReconcileDesiredBundle(ctx context.Context, c client.Client, bd *rukpakv1alpha1.BundleDeployment) (*rukpakv1alpha1.Bundle, *rukpakv1alpha1.BundleList, error) {
	// get the set of Bundle resources that already exist on cluster, and sort
	// by metadata.CreationTimestamp in the case there's multiple Bundles
	// that match the label selector.
	existingBundles, err := GetBundlesForBundleDeploymentSelector(ctx, c, bd)
	if err != nil {
		return nil, nil, err
	}
	SortBundlesByCreation(existingBundles)

	// check whether the BI controller has already reached the maximum
	// generated Bundle limit to avoid hotlooping scenarios.
	if len(existingBundles.Items) > maxGeneratedBundleLimit {
		return nil, nil, ErrMaxGeneratedLimit
	}

	// check whether there's an existing Bundle that matches the desired Bundle template
	// specified in the BI resource, and if not, generate a new Bundle that matches the template.
	b := CheckExistingBundlesMatchesTemplate(existingBundles, bd.Spec.Template)
	if b == nil {
		controllerRef := metav1.NewControllerRef(bd, bd.GroupVersionKind())
		hash := GenerateTemplateHash(bd.Spec.Template)

		labels := bd.Spec.Template.Labels
		if len(labels) == 0 {
			labels = make(map[string]string)
		}
		labels[CoreOwnerKindKey] = rukpakv1alpha1.BundleDeploymentKind
		labels[CoreOwnerNameKey] = bd.GetName()
		labels[CoreBundleTemplateHashKey] = hash

		b = &rukpakv1alpha1.Bundle{
			ObjectMeta: metav1.ObjectMeta{
				Name:            GenerateBundleName(bd.GetName(), hash),
				OwnerReferences: []metav1.OwnerReference{*controllerRef},
				Labels:          labels,
				Annotations:     bd.Spec.Template.Annotations,
			},
			Spec: bd.Spec.Template.Spec,
		}
		if err := c.Create(ctx, b); err != nil {
			return nil, nil, err
		}
	}
	return b, existingBundles, err
}

func BundleProvisionerFilter(provisionerClassName string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		b := obj.(*rukpakv1alpha1.Bundle)
		return b.Spec.ProvisionerClassName == provisionerClassName
	})
}

func BundleDeploymentProvisionerFilter(provisionerClassName string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		b := obj.(*rukpakv1alpha1.BundleDeployment)
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
func MapOwneeToOwnerProvisionerHandler(ctx context.Context, cl client.Client, log logr.Logger, provisionerClassName string, owner ProvisionerClassNameGetter) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
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

// MapBundleToBundleDeployment is responsible for finding the BundleDeployment resource
// that's managing this Bundle in the cluster. In the case that this Bundle is a standalone
// resource, then no BundleDeployment will be returned as static creation of Bundle
// resources is not a supported workflow right now.
func MapBundleToBundleDeployment(ctx context.Context, c client.Client, b rukpakv1alpha1.Bundle) *rukpakv1alpha1.BundleDeployment {
	// check whether this is a standalone bundle that was created outside
	// of the normal BundleDeployment controller reconciliation process.
	if bundleOwnerType := b.Labels[CoreOwnerKindKey]; bundleOwnerType != rukpakv1alpha1.BundleDeploymentKind {
		return nil
	}
	bundleOwnerName := b.Labels[CoreOwnerNameKey]
	if bundleOwnerName == "" {
		return nil
	}

	bundleDeployments := &rukpakv1alpha1.BundleDeploymentList{}
	if err := c.List(ctx, bundleDeployments); err != nil {
		return nil
	}
	for _, bd := range bundleDeployments.Items {
		bd := bd

		if bd.GetName() == bundleOwnerName {
			return bd.DeepCopy()
		}
	}
	return nil
}

// MapBundleToBundleDeploymentHandler is responsible for requeuing a BundleDeployment resource
// when a new Bundle event has been encountered. In the case that the Bundle resource is a
// standalone resource, then no BundleDeployment will be returned as static creation of Bundle
// resources is not a supported workflow right now. The provisionerClassName parameter is used
// to filter out BundleDeployments that the caller shouldn't be watching.
func MapBundleToBundleDeploymentHandler(ctx context.Context, cl client.Client, provisionerClassName string) handler.MapFunc {
	return func(object client.Object) []reconcile.Request {
		b := object.(*rukpakv1alpha1.Bundle)

		managingBD := MapBundleToBundleDeployment(ctx, cl, *b)
		if managingBD == nil {
			return nil
		}
		if managingBD.Spec.ProvisionerClassName != provisionerClassName {
			return nil
		}
		return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(managingBD)}}
	}
}
func MapConfigMapToBundles(ctx context.Context, cl client.Client, cmNamespace string, cm corev1.ConfigMap) []*rukpakv1alpha1.Bundle {
	bundleList := &rukpakv1alpha1.BundleList{}
	if err := cl.List(ctx, bundleList); err != nil {
		return nil
	}
	var bs []*rukpakv1alpha1.Bundle
	for _, b := range bundleList.Items {
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
func MapConfigMapToBundlesHandler(ctx context.Context, cl client.Client, configMapNamespace string, provisionerClassName string) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(object client.Object) []reconcile.Request {
		cm := object.(*corev1.ConfigMap)
		var requests []reconcile.Request
		matchingBundles := MapConfigMapToBundles(ctx, cl, configMapNamespace, *cm)
		for _, b := range matchingBundles {
			if b.Spec.ProvisionerClassName != provisionerClassName {
				continue
			}
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(b)})
		}
		return requests
	})
}

// GetBundlesForBundleDeploymentSelector is responsible for returning a list of
// Bundle resource that exist on cluster that match the label selector specified
// in the BD parameter's spec.Selector field.
func GetBundlesForBundleDeploymentSelector(ctx context.Context, c client.Client, bd *rukpakv1alpha1.BundleDeployment) (*rukpakv1alpha1.BundleList, error) {
	selector := NewBundleDeploymentLabelSelector(bd)
	bundleList := &rukpakv1alpha1.BundleList{}
	if err := c.List(ctx, bundleList, &client.ListOptions{
		LabelSelector: selector,
	}); err != nil {
		return nil, fmt.Errorf("failed to list bundles using the %s selector: %v", selector.String(), err)
	}
	return bundleList, nil
}

// CheckExistingBundlesMatchesTemplate evaluates whether the existing list of Bundle objects
// match the desired Bundle template that's specified in a BundleDeployment object. If a match
// is found, that Bundle object is returned, so callers are responsible for nil checking the result.
func CheckExistingBundlesMatchesTemplate(existingBundles *rukpakv1alpha1.BundleList, desiredBundleTemplate *rukpakv1alpha1.BundleTemplate) *rukpakv1alpha1.Bundle {
	for i := range existingBundles.Items {
		if !CheckDesiredBundleTemplate(&existingBundles.Items[i], desiredBundleTemplate) {
			continue
		}
		return existingBundles.Items[i].DeepCopy()
	}
	return nil
}

// CheckDesiredBundleTemplate is responsible for determining whether the existingBundle
// hash is equal to the desiredBundle Bundle template hash.
func CheckDesiredBundleTemplate(existingBundle *rukpakv1alpha1.Bundle, desiredBundle *rukpakv1alpha1.BundleTemplate) bool {
	if len(existingBundle.Labels) == 0 {
		// Existing Bundle has no labels set, which should never be the case.
		// Return false so that the Bundle is forced to be recreated with the expected labels.
		return false
	}

	existingHash, ok := existingBundle.Labels[CoreBundleTemplateHashKey]
	if !ok {
		// Existing Bundle has no template hash associated with it.
		// Return false so that the Bundle is forced to be recreated with the template hash label.
		return false
	}

	// Check whether the hash of the desired bundle template matches the existing bundle on-cluster.
	desiredHash := GenerateTemplateHash(desiredBundle)
	return existingHash == desiredHash
}

func GenerateTemplateHash(template *rukpakv1alpha1.BundleTemplate) string {
	hasher := fnv.New32a()
	DeepHashObject(hasher, template)
	return rand.SafeEncodeString(fmt.Sprintf("%x", hasher.Sum32())[:6])
}

func GenerateBundleName(bdName, hash string) string {
	return fmt.Sprintf("%s-%s", bdName, hash)
}

// SortBundlesByCreation sorts a BundleList's items by it's
// metadata.CreationTimestamp value.
func SortBundlesByCreation(bundles *rukpakv1alpha1.BundleList) {
	sort.Slice(bundles.Items, func(a, b int) bool {
		return bundles.Items[a].CreationTimestamp.Before(&bundles.Items[b].CreationTimestamp)
	})
}

// GetPodNamespace checks whether the controller is running in a Pod vs.
// being run locally by inspecting the namespace file that gets mounted
// automatically for Pods at runtime. If that file doesn't exist, then
// return the @defaultNamespace namespace parameter.
func PodNamespace(defaultNamespace string) string {
	namespace, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return defaultNamespace
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

// NewBundleLabelSelector is responsible for constructing a label.Selector
// for any underlying resources that are associated with the Bundle parameter.
func NewBundleLabelSelector(bundle *rukpakv1alpha1.Bundle) labels.Selector {
	return newLabelSelector(bundle.GetName(), rukpakv1alpha1.BundleKind)
}

// NewBundleDeploymentLabelSelector is responsible for constructing a label.Selector
// for any underlying resources that are associated with the BundleDeployment parameter.
func NewBundleDeploymentLabelSelector(bd *rukpakv1alpha1.BundleDeployment) labels.Selector {
	return newLabelSelector(bd.GetName(), rukpakv1alpha1.BundleDeploymentKind)
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

	if err := wait.PollImmediateUntil(time.Millisecond*5, func() (bool, error) {
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
