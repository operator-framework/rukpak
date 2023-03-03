package configmapsyncer

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	configMapInjectFromSecretName  = "core.rukpak.io/inject-from-secret-name"
	configMapInjectFromSecretKey   = "core.rukpak.io/inject-from-secret-key"
	configMapInjectToDataKey       = "core.rukpak.io/inject-to-data-key"
	configMapInjectToBinaryDataKey = "core.rukpak.io/inject-to-binarydata-key"
)

// Reconciler syncs secret fields to configmaps.
type Reconciler struct {
	Client client.Client
	Cache  cache.Cache
}

// Reconcile syncs a secret field to a configmap field based on the presence
// of annotations "core.rukpak.io/inject-from-secret-name" and
// "core.rukpak.io/inject-from-secret-key" (configmaps without BOTH of these
// annotations are ignored). When these annotations are present, this
// reconciler manages all content in the data and binaryData fields.
//
// If the annotation "core.rukpak.io/inject-to-data-key" is present, Reconcile
// creates a data key containing the secret value. Otherwise, the configmap
// data will be empty.
//
// If the annotation "core.rukpak.io/inject-to-binarydata-key" is present,
// Reconcile creates a binaryData key containing the secret value. Otherwise,
// the configmap binaryData will be empty.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.V(1).Info("starting reconciliation")
	defer l.V(1).Info("ending reconciliation")

	// Get configmap from cache and lookup its secret name annotation
	cm := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx, req.NamespacedName, cm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	secretName, ok := cm.Annotations[configMapInjectFromSecretName]
	if !ok {
		return ctrl.Result{}, nil
	}

	// Get referenced secret name (in the same namespace as the configmap)
	secret := &corev1.Secret{}
	l.V(1).Info("inject from secret", "secretName", secretName)
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: secretName}, secret); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Get the referenced secret's key and value
	secretKey, ok := cm.Annotations[configMapInjectFromSecretKey]
	if !ok {
		return ctrl.Result{}, nil
	}
	secretValue := secret.Data[secretKey]

	// If the configmap asks for injection into a binaryData key
	// generate the expected binary data.
	cmBinaryDataKey := cm.Annotations[configMapInjectToBinaryDataKey]
	expectedBinaryData := map[string][]byte(nil)
	if cmBinaryDataKey != "" {
		expectedBinaryData = map[string][]byte{cmBinaryDataKey: secretValue}
	}

	// If the configmap asks for injection into a data key
	// generate the expected data.
	cmDataKey := cm.Annotations[configMapInjectToDataKey]
	expectedData := map[string]string(nil)
	if cmDataKey != "" {
		expectedData = map[string]string{cmDataKey: string(secretValue)}
	}

	// If binaryData and data already have the expected contents,
	// there's no need to do anything, so return early.
	if equality.Semantic.DeepEqual(cm.BinaryData, expectedBinaryData) && equality.Semantic.DeepEqual(cm.Data, expectedData) {
		return ctrl.Result{}, nil
	}

	// Set the expected binaryData and data fields on the configmap
	// and then update it.
	cm.BinaryData = expectedBinaryData
	cm.Data = expectedData
	return ctrl.Result{}, r.Client.Update(ctx, cm)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	ctrlr, err := controller.New("configmapsyncer", mgr, controller.Options{
		Reconciler: r,
	})
	if err != nil {
		return err
	}

	if err := ctrlr.Watch(
		source.NewKindWithCache(&corev1.ConfigMap{}, r.Cache),
		&handler.EnqueueRequestForObject{},
		configMapHasInjectionAnnotationsPredicate(),
	); err != nil {
		return err
	}

	if err := ctrlr.Watch(
		source.NewKindWithCache(&corev1.Secret{}, r.Cache),
		secretToConfigMapMapper(r.Client),
	); err != nil {
		return err
	}
	return nil
}

func configMapHasInjectionAnnotationsPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		cm := object.(*corev1.ConfigMap)
		if _, ok := cm.Annotations[configMapInjectFromSecretName]; !ok {
			return false
		}
		if _, ok := cm.Annotations[configMapInjectFromSecretKey]; !ok {
			return false
		}
		return true
	})
}

func secretToConfigMapMapper(cl client.Reader) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(object client.Object) []reconcile.Request {
		cmList := &corev1.ConfigMapList{}
		if err := cl.List(context.TODO(), cmList, client.InNamespace(object.GetNamespace())); err != nil {
			return nil
		}
		reqs := []reconcile.Request{}
		for _, cm := range cmList.Items {
			if cm.Annotations[configMapInjectFromSecretName] == object.GetName() {
				reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&cm)})
			}
		}
		return reqs
	})
}
