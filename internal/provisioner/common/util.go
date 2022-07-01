package common

// TODO: Update file name

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apimachyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/source"
	updater "github.com/operator-framework/rukpak/internal/updater/bundle"
)

func UpdateStatusUnpackPending(u *updater.Updater, result *source.Result) {
	u.UpdateStatus(
		updater.EnsureResolvedSource(nil),
		updater.EnsureContentURL(""),
		updater.SetPhase(rukpakv1alpha1.PhasePending),
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonUnpackPending,
			Message: result.Message,
		}),
	)
}

func UpdateStatusUnpacking(u *updater.Updater, result *source.Result) {
	u.UpdateStatus(
		updater.EnsureResolvedSource(nil),
		updater.EnsureContentURL(""),
		updater.SetPhase(rukpakv1alpha1.PhaseUnpacking),
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonUnpacking,
			Message: result.Message,
		}),
	)
}

func UpdateStatusUnpacked(u *updater.Updater, result *source.Result, contentURL string) {
	u.UpdateStatus(
		updater.EnsureResolvedSource(result.ResolvedSource),
		updater.EnsureContentURL(contentURL),
		updater.SetPhase(rukpakv1alpha1.PhaseUnpacked),
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionTrue,
			Reason:  rukpakv1alpha1.ReasonUnpackSuccessful,
			Message: result.Message,
		}),
	)
}

func UpdateStatusUnpackFailing(u *updater.Updater, err error) error {
	u.UpdateStatus(
		updater.EnsureResolvedSource(nil),
		updater.EnsureContentURL(""),
		updater.SetPhase(rukpakv1alpha1.PhaseFailing),
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonUnpackFailed,
			Message: err.Error(),
		}),
	)
	return err
}

func GetObjects(bundleFS fs.FS) ([]client.Object, error) {
	var objects []client.Object
	const manifestsDir = "manifests"

	entries, err := fs.ReadDir(bundleFS, manifestsDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			return nil, fmt.Errorf("subdirectories are not allowed within the %q directory of the bundle image filesystem: found %q", manifestsDir, filepath.Join(manifestsDir, e.Name()))
		}
		fileData, err := fs.ReadFile(bundleFS, filepath.Join(manifestsDir, e.Name()))
		if err != nil {
			return nil, err
		}

		dec := apimachyaml.NewYAMLOrJSONDecoder(bytes.NewReader(fileData), 1024)
		for {
			obj := unstructured.Unstructured{}
			err := dec.Decode(&obj)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("read %q: %v", e.Name(), err)
			}
			objects = append(objects, &obj)
		}
	}
	return objects, nil
}
