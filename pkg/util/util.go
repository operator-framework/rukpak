package util

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		gvks, unversioned, err := cl.Scheme().ObjectKinds(owner)
		if err != nil {
			log.Error(err, "get GVKs for owner")
			return nil
		}
		if unversioned {
			log.Error(err, "owner cannot be an unversioned type")
			return nil
		}

		type ownerInfo struct {
			key types.NamespacedName
			gvk schema.GroupVersionKind
		}
		var oi *ownerInfo

	refLoop:
		for _, ref := range obj.GetOwnerReferences() {
			gv, err := schema.ParseGroupVersion(ref.APIVersion)
			if err != nil {
				log.Error(err, fmt.Sprintf("parse group version %q", ref.APIVersion))
				return nil
			}
			refGVK := gv.WithKind(ref.Kind)
			for _, gvk := range gvks {
				if refGVK == gvk && ref.Controller != nil && *ref.Controller {
					oi = &ownerInfo{
						key: types.NamespacedName{Name: ref.Name},
						gvk: gvk,
					}
					break refLoop
				}
			}
		}
		if oi == nil {
			return nil
		}
		if err := cl.Get(ctx, oi.key, owner); err != nil {
			log.Error(err, "get owner", "kind", oi.gvk, "name", oi.key.Name)
			return nil
		}
		if owner.ProvisionerClassName() != provisionerClassName {
			return nil
		}
		return []reconcile.Request{{NamespacedName: oi.key}}
	})
}
