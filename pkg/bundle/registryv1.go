package bundle

import (
	"bytes"
	"errors"
	"fmt"
	"hash"
	"hash/fnv"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Bundle = RegistryV1{}

// RegistryV1 holds the contents of a registry+v1 bundle.
type RegistryV1 struct {
	files  map[string]ManifestFile
	csv    operatorsv1alpha1.ClusterServiceVersion
	crds   []apiextensionsv1.CustomResourceDefinition
	others []client.Object
}

// NewRegistryV1 converts a filesystem to a registry+v1 bundle
func NewRegistryV1(fsys fs.FS) (*RegistryV1, error) {
	entries, err := fs.ReadDir(fsys, manifestsDir)
	if err != nil {
		return nil, err
	}

	bundle := RegistryV1{files: make(map[string]ManifestFile, len(entries))}
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, fmt.Errorf("subdirectories not allowed within manifests directory, found: %q", entry.Name())
		}
		path := filepath.Join(manifestsDir, entry.Name())
		if err := bundle.addObjectsFromFile(fsys, path); err != nil {
			return nil, err
		}
	}

	return &bundle, nil
}

func (b RegistryV1) addObjectsFromFile(fsys fs.FS, path string) error {
	objs, err := b.slurpManifestFile(fsys, path)
	if err != nil {
		return fmt.Errorf("unable to read manifest file: %q: %s", path, err.Error())
	}

	objsForFile := make([]*client.Object, len(objs))
	for i, obj := range objs {
		objsForFile[i] = &obj
		switch typedObj := obj.(type) {
		case *operatorsv1alpha1.ClusterServiceVersion:
			b.csv = *typedObj
		case *apiextensionsv1.CustomResourceDefinition:
			b.crds = append(b.crds, *typedObj)
		default:
			b.others = append(b.others, obj)
		}
	}

	return nil
}

func (b RegistryV1) slurpManifestFile(fsys fs.FS, path string) ([]client.Object, error) {
	f, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	objs := make([]client.Object, 0, 1)
	dec := yaml.NewYAMLOrJSONDecoder(f, 1024)
	for {
		var unstructuredObj unstructured.Unstructured
		if err := dec.Decode(&unstructuredObj); errors.Is(err, io.EOF) {
			return objs, nil
		} else if err != nil {
			return nil, err
		}
		obj, err := scheme.New(unstructuredObj.GroupVersionKind())
		if err != nil {
			return nil, err
		}
		if err := scheme.Convert(unstructuredObj, &obj, nil); err != nil {
			return nil, err
		}
		objs = append(objs, obj.(client.Object))
	}
}

// Open returns a file pointing at the manifest identified by the filepath.
func (r RegistryV1) Open(name string) (fs.File, error) {
	file, ok := r.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}

	return &openManifestFile{
		path: name,
		r:    bytes.NewReader([]byte{}),
		manifestFileInfo: manifestFileInfo{
			name:         filepath.Base(name),
			ManifestFile: file,
		},
	}, nil
}

// CSV returns the ClusterServiceVersion manifest.
func (r RegistryV1) CSV() *operatorsv1alpha1.ClusterServiceVersion {
	return &r.csv
}

// CSV returns the ClusterServiceVersion manifest.
func (r RegistryV1) CRDs() []apiextensionsv1.CustomResourceDefinition {
	return r.crds
}

// Others returns all other manifest files.
func (r RegistryV1) Others() []client.Object {
	return r.others
}

// FromRegistryV1 converts a registry+v1 bundle to a plain+v1 list of objects.
func FromRegistryV1(in RegistryV1, opts ...Option) (*PlainV1, error) {
	for _, opt := range opts {
		opt.apply(&in)
	}

	installNamespace, err := in.installNamespace()
	if err != nil {
		return nil, err
	}
	targetNamespaces, err := in.targetNamespaces()
	if err != nil {
		return nil, err
	}
	deployments := []appsv1.Deployment{}
	serviceAccounts := map[string]corev1.ServiceAccount{}
	for _, depSpec := range in.CSV.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		annotations := in.CSV.Annotations
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations["olm.targetNamespaces"] = strings.Join(targetNamespaces, ",")
		deployments = append(deployments, appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: appsv1.SchemeGroupVersion.String(),
			},

			ObjectMeta: metav1.ObjectMeta{
				Namespace:   installNamespace,
				Name:        depSpec.Name,
				Labels:      depSpec.Label,
				Annotations: annotations,
			},
			Spec: depSpec.Spec,
		})
		serviceAccounts[depSpec.Spec.Template.Spec.ServiceAccountName] = corev1.ServiceAccount{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ServiceAccount",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: installNamespace,
				Name:      depSpec.Spec.Template.Spec.ServiceAccountName,
			},
		}
	}

	roles := []rbacv1.Role{}
	roleBindings := []rbacv1.RoleBinding{}
	clusterRoles := []rbacv1.ClusterRole{}
	clusterRoleBindings := []rbacv1.ClusterRoleBinding{}

	for _, ns := range targetNamespaces {
		if ns == "" {
			continue
		}
		for _, permission := range in.CSV.Spec.InstallStrategy.StrategySpec.Permissions {
			if _, ok := serviceAccounts[permission.ServiceAccountName]; !ok {
				serviceAccounts[permission.ServiceAccountName] = corev1.ServiceAccount{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ServiceAccount",
						APIVersion: corev1.SchemeGroupVersion.String(),
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: installNamespace,
						Name:      permission.ServiceAccountName,
					},
				}
			}
			name := generateName(fmt.Sprintf("%s-%s", in.CSV.GetName(), permission.ServiceAccountName), []interface{}{in.CSV.GetName(), permission})
			roles = append(roles, rbacv1.Role{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Role",
					APIVersion: rbacv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      name,
				},
				Rules: permission.Rules,
			})
			roleBindings = append(roleBindings, rbacv1.RoleBinding{
				TypeMeta: metav1.TypeMeta{
					Kind:       "RoleBinding",
					APIVersion: rbacv1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      name,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      permission.ServiceAccountName,
						Namespace: installNamespace,
					},
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: rbacv1.GroupName,
					Kind:     "Role",
					Name:     name,
				},
			})
		}
	}
	for _, permission := range in.CSV.Spec.InstallStrategy.StrategySpec.ClusterPermissions {
		if _, ok := serviceAccounts[permission.ServiceAccountName]; !ok {
			serviceAccounts[permission.ServiceAccountName] = corev1.ServiceAccount{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ServiceAccount",
					APIVersion: corev1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: installNamespace,
					Name:      permission.ServiceAccountName,
				},
			}
		}
		name := generateName(fmt.Sprintf("%s-%s", in.CSV.GetName(), permission.ServiceAccountName), []interface{}{in.CSV.GetName(), permission})
		clusterRoles = append(clusterRoles, rbacv1.ClusterRole{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ClusterRole",
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Rules: permission.Rules,
		})
		clusterRoleBindings = append(clusterRoleBindings, rbacv1.ClusterRoleBinding{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ClusterRoleBinding",
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      permission.ServiceAccountName,
					Namespace: installNamespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     name,
			},
		})
	}

	objs := []client.Object{}
	for _, obj := range serviceAccounts {
		obj := obj
		objs = append(objs, &obj)
	}
	for _, obj := range roles {
		obj := obj
		objs = append(objs, &obj)
	}
	for _, obj := range roleBindings {
		obj := obj
		objs = append(objs, &obj)
	}
	for _, obj := range clusterRoles {
		obj := obj
		objs = append(objs, &obj)
	}
	for _, obj := range clusterRoleBindings {
		obj := obj
		objs = append(objs, &obj)
	}
	for _, obj := range in.CRDs {
		obj := obj
		objs = append(objs, &obj)
	}
	for _, obj := range in.Others {
		obj := obj
		objs = append(objs, &obj)
	}
	for _, obj := range deployments {
		obj := obj
		objs = append(objs, &obj)
	}
	return &PlainV1{Objects: objs}, nil
}

func (r RegistryV1) installNamespace() (string, error) {
	if r.overrides.installNamespace != "" {
		return r.overrides.installNamespace, nil
	}

	installNamespace, ok := r.CSV.Annotations["operatorframework.io/suggested-namespace"]
	if !ok {
		return "", errors.New("unable to detect install namespace")
	}

	return installNamespace, nil
}

func (r RegistryV1) supportedInstallModes() sets.String {
	supportedInstallModes := sets.NewString()
	for _, im := range r.CSV.Spec.InstallModes {
		if im.Supported {
			supportedInstallModes.Insert(string(im.Type))
		}
	}

	return supportedInstallModes
}

func (r RegistryV1) validateTargetNamespaces(installNamespace string, targetNamespaces []string) error {
	var (
		set                   = sets.NewString(targetNamespaces...)
		supportedInstallModes = r.supportedInstallModes()
		allNamespaces         = supportedInstallModes.Has(string(v1alpha1.InstallModeTypeAllNamespaces))
		singleNamespace       = supportedInstallModes.Has(string(v1alpha1.InstallModeTypeSingleNamespace))
		ownNamespace          = supportedInstallModes.Has(string(v1alpha1.InstallModeTypeOwnNamespace))
		multiNamespace        = supportedInstallModes.Has(string(v1alpha1.InstallModeTypeMultiNamespace))
	)

	if set.Len() == 0 && allNamespaces {
		return nil
	}
	if set.Len() == 1 &&
		((set.Has("") && allNamespaces) || singleNamespace || (ownNamespace && set.Has(installNamespace))) {
		return nil
	}
	if set.Len() > 1 && multiNamespace {
		return nil
	}

	return fmt.Errorf("supported install modes %v do not support target namespaces %v", supportedInstallModes.List(), targetNamespaces)
}

func (r RegistryV1) targetNamespaces() ([]string, error) {
	installNamespace, err := r.installNamespace()
	if err != nil {
		return nil, err
	}

	supportedInstallModes := r.supportedInstallModes()
	targetNamespaces := r.overrides.targetNamespaces
	if targetNamespaces == nil {
		if supportedInstallModes.Has(string(v1alpha1.InstallModeTypeAllNamespaces)) {
			targetNamespaces = []string{}
		} else if supportedInstallModes.Has(string(v1alpha1.InstallModeTypeOwnNamespace)) {
			targetNamespaces = []string{installNamespace}
		}
	}

	if err := r.validateTargetNamespaces(installNamespace, targetNamespaces); err != nil {
		return nil, err
	}

	return targetNamespaces, nil
}

const maxNameLength = 63

func generateName(base string, o interface{}) string {
	hasher := fnv.New32a()

	deepHashObject(hasher, o)
	hashStr := rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
	if len(base)+len(hashStr) > maxNameLength {
		base = base[:maxNameLength-len(hashStr)-1]
	}

	return fmt.Sprintf("%s-%s", base, hashStr)
}

// deepHashObject writes specified object to hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func deepHashObject(hasher hash.Hash, objectToWrite interface{}) {
	hasher.Reset()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	if _, err := printer.Fprintf(hasher, "%#v", objectToWrite); err != nil {
		panic(err)
	}
}
