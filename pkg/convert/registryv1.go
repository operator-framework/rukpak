package convert

import (
	"errors"
	"fmt"
	"hash"
	"hash/fnv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RegistryV1 holds the contents of a registry+v1 bundle.
type RegistryV1 struct {
	CSV    v1alpha1.ClusterServiceVersion
	CRDs   []apiextensionsv1.CustomResourceDefinition
	Others []client.Object

	overrides struct {
		installNamespace string
		targetNamespaces []string
	}
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
