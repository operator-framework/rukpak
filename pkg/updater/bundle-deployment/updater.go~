package bundledeployment

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/updater"
)

func NewBundleDeploymentUpdater(client client.Client) Updater {
	return Updater{
		client: client,
	}
}

type Updater struct {
	client            client.Client
	updateStatusFuncs []UpdateStatusFunc
}

type UpdateStatusFunc func(bd *rukpakv1alpha1.BundleDeploymentStatus) bool

func (u *Updater) UpdateStatus(fs ...UpdateStatusFunc) {
	u.updateStatusFuncs = append(u.updateStatusFuncs, fs...)
}

func (u *Updater) Apply(ctx context.Context, bd *rukpakv1alpha1.BundleDeployment) error {
	backoff := retry.DefaultRetry

	return retry.RetryOnConflict(backoff, func() error {
		if err := u.client.Get(ctx, client.ObjectKeyFromObject(bd), bd); err != nil {
			return err
		}
		needsStatusUpdate := false
		for _, f := range u.updateStatusFuncs {
			needsStatusUpdate = f(&bd.Status) || needsStatusUpdate
		}
		if needsStatusUpdate {
			log.FromContext(ctx).Info("applying status changes")
			return u.client.Status().Update(ctx, bd)
		}
		return nil
	})
}

func EnsureCondition(condition metav1.Condition) UpdateStatusFunc {
	return func(status *rukpakv1alpha1.BundleDeploymentStatus) bool {
		existing := meta.FindStatusCondition(status.Conditions, condition.Type)
		if existing == nil || !updater.ConditionsSemanticallyEqual(*existing, condition) {
			meta.SetStatusCondition(&status.Conditions, condition)
			return true
		}
		return false
	}
}

func EnsureInstalledName(bundleName string) UpdateStatusFunc {
	return func(status *rukpakv1alpha1.BundleDeploymentStatus) bool {
		if status.ActiveBundle == bundleName {
			return false
		}
		status.ActiveBundle = bundleName
		return true
	}
}
