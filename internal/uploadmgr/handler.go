package uploadmgr

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/util"
)

const DefaultBundleCacheDir = "/var/cache/uploads"

//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=list;watch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/status,verbs=update;patch
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

func NewUploadHandler(cl client.Client, storageDir string) http.Handler {
	r := mux.NewRouter()
	r.Methods(http.MethodGet).Path("/uploads/{bundleName}.tgz").Handler(http.StripPrefix("/uploads/", http.FileServer(http.FS(&util.FilesOnlyFilesystem{FS: os.DirFS(storageDir)}))))
	r.Methods(http.MethodPut).Path("/uploads/{bundleName}.tgz").Handler(newPutHandler(cl, storageDir))
	return r
}

func newPutHandler(cl client.Client, storageDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bundleName := mux.Vars(r)["bundleName"]

		bundle := &rukpakv1alpha1.Bundle{}
		if err := cl.Get(r.Context(), types.NamespacedName{Name: bundleName}, bundle); err != nil {
			http.Error(w, err.Error(), int(getCode(err)))
			return
		}
		if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeUpload {
			http.Error(w, fmt.Sprintf("bundle source type is %q; expected %q", bundle.Spec.Source.Type, rukpakv1alpha1.SourceTypeUpload), http.StatusConflict)
			return
		}

		uploadBundleData, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("read request body: %v", err), http.StatusInternalServerError)
			return
		}
		bundleFilePath := bundlePath(storageDir, bundleName)
		if existingData, err := os.ReadFile(bundleFilePath); err == nil {
			if bytes.Equal(uploadBundleData, existingData) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		if bundle.Status.Phase == rukpakv1alpha1.PhaseUnpacked {
			http.Error(w, "bundle has already been unpacked, cannot change content of existing bundle", http.StatusConflict)
			return
		}

		bundleFile, err := os.Create(bundleFilePath)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to store bundle data: %v", err), http.StatusInternalServerError)
			return
		}
		defer bundleFile.Close()

		if _, err := bundleFile.Write(uploadBundleData); err != nil {
			http.Error(w, fmt.Sprintf("failed to store bundle data: %v", err), http.StatusInternalServerError)
			return
		}

		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := cl.Get(r.Context(), types.NamespacedName{Name: bundleName}, bundle); err != nil {
				return err
			}
			if bundle.Status.Phase == rukpakv1alpha1.PhaseUnpacked {
				return nil
			}

			bundle.Status.Phase = rukpakv1alpha1.PhasePending
			meta.SetStatusCondition(&bundle.Status.Conditions, metav1.Condition{
				Type:    rukpakv1alpha1.TypeUnpacked,
				Status:  metav1.ConditionFalse,
				Reason:  rukpakv1alpha1.ReasonUnpackPending,
				Message: "received bundle upload, waiting for provisioner to unpack it.",
			})
			return cl.Status().Update(r.Context(), bundle)
		}); err != nil {
			http.Error(w, err.Error(), int(getCode(err)))
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
}

func getCode(err error) int32 {
	if status := apierrors.APIStatus(nil); errors.As(err, &status) {
		return status.Status().Code
	}
	return http.StatusInternalServerError
}

func bundlePath(baseDir, bundleName string) string {
	return filepath.Join(baseDir, fmt.Sprintf("%s.tgz", bundleName))
}
