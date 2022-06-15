package bundle

import (
	"fmt"
	"hash"
	"hash/fnv"
	"io/fs"
	"strings"

	"github.com/davecgh/go-spew/spew"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/operator-framework/rukpak/pkg/bundle"
	registryv1 "github.com/operator-framework/rukpak/pkg/bundle/registry/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Bundle holds the contents of a plain+v0 bundle.
type Bundle struct {
	bundle.FS

	// external settings
	installNamespace string
	targetNamespaces []string
}

// New creates a new plain+v0 bundle at the root of the given filesystem.
//
// If the file system containse another known bundle format, it will be
// converted to a plain+v0 bundle.
func New(fsys fs.FS, opts ...func(*Bundle)) Bundle {
	var b Bundle
	for _, opt := range opts {
		opt(&b)
	}

	if bundleFS, ok := fsys.(bundle.FS); ok {
		b.FS = bundleFS
	} else {
		b.FS = bundle.New(fsys, bundle.WithManifestDir("manifests"))
	}

	return b
}

// FromRegistryV1 converts a registry+v1 bundle into a plain+v0 bundle.
//
// This function removes all files besides the `Dockerfile` and `manifests`
// directory and creates stand alone manifests from the CSV file.
func FromRegistryV1(srcBundle registryv1.Bundle, opts ...func(*Bundle)) (*Bundle, error) {
	b := Bundle{FS: srcBundle.FS}
	for _, opt := range opts {
		opt(&b)
	}
	csv, err := srcBundle.CSV()
	if err != nil {
		return nil, err
	}
	if err := b.validateOptions(csv); err != nil {
		return nil, err
	}
	if err := b.extractCsvDeployments(csv); err != nil {
		return nil, err
	}
	if err := b.extractCsvRBAC(csv); err != nil {
		return nil, err
	}

	return &b, nil
}

func (b *Bundle) validateOptions(csv *operatorsv1alpha1.ClusterServiceVersion) error {
	b.validateInstallNamespace(csv)
	return b.validateTargetNamespaces(csv)
}

func (b *Bundle) validateInstallNamespace(csv *operatorsv1alpha1.ClusterServiceVersion) {
	if b.installNamespace != "" {
		return
	}
	if ns, ok := csv.Annotations["operatorframework.io/suggested-namespace"]; ok {
		b.installNamespace = ns
		return
	}

	// TODO(ryantking): Does srcBundle.CSV().GetName() give us package name
	// per @joelanford's install namespace detection suggestion?
	b.installNamespace = fmt.Sprintf("%s-system", csv.GetName())
}

func (b *Bundle) validateTargetNamespaces(csv *operatorsv1alpha1.ClusterServiceVersion) error {
	var (
		supportedInstallModes = b.supportedInstallModes(csv)
		supportsAllNS         = supportedInstallModes.Has(string(operatorsv1alpha1.InstallModeTypeAllNamespaces))
		supportsSingleNS      = supportedInstallModes.Has(string(operatorsv1alpha1.InstallModeTypeSingleNamespace))
		supportsOwnNS         = supportedInstallModes.Has(string(operatorsv1alpha1.InstallModeTypeOwnNamespace))
		supportsMultiNS       = supportedInstallModes.Has(string(operatorsv1alpha1.InstallModeTypeMultiNamespace))
	)

	if len(b.targetNamespaces) == 0 && supportsOwnNS {
		b.targetNamespaces = []string{b.installNamespace}
	}

	targetNamespaces := sets.NewString(b.targetNamespaces...)
	if targetNamespaces.Len() == 0 && supportsAllNS {
		return nil
	}
	if targetNamespaces.Len() == 1 &&
		((targetNamespaces.Has("") && supportsAllNS) ||
			(supportsSingleNS && !targetNamespaces.Has(b.installNamespace)) ||
			(supportsOwnNS && targetNamespaces.Has(b.installNamespace))) {
		return nil
	}
	if targetNamespaces.Len() > 1 && supportsMultiNS {
		return nil
	}

	return fmt.Errorf(
		"supported install modes %v do not support target namespaces %v",
		supportedInstallModes.List(), b.targetNamespaces,
	)
}

func (b Bundle) supportedInstallModes(csv *operatorsv1alpha1.ClusterServiceVersion) sets.String {
	supportedInstallModes := sets.NewString()
	for _, im := range csv.Spec.InstallModes {
		if im.Supported {
			supportedInstallModes.Insert(string(im.Type))
		}
	}

	return supportedInstallModes
}

func (b *Bundle) extractCsvDeployments(csv *operatorsv1alpha1.ClusterServiceVersion) error {
	annotations := csv.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["olm.targetNamespaces"] = strings.Join(b.targetNamespaces, ",")

	for _, depSpec := range csv.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		if err := b.newDeployment(depSpec, annotations); err != nil {
			return err
		}
		if err := b.newServiceAccount(depSpec.Spec.Template.Spec.ServiceAccountName); err != nil {
			return err
		}
	}

	return nil
}

func (b *Bundle) extractCsvRBAC(csv *operatorsv1alpha1.ClusterServiceVersion) error {
	for _, permission := range csv.Spec.InstallStrategy.StrategySpec.Permissions {
		name := b.generateName(
			fmt.Sprintf("%s-%s", csv.GetName(), permission.ServiceAccountName),
			[]interface{}{csv.GetName(), permission},
		)
		if err := b.newServiceAccount(permission.ServiceAccountName); err != nil {
			return err
		}
		if err := b.newRoles(name, permission); err != nil {
			return err
		}
		if err := b.newRoleBindings(name, permission); err != nil {
			return err
		}
		if err := b.newClusterRoles(name, permission); err != nil {
			return err
		}
		if err := b.newClusterRoleBindings(name, permission); err != nil {
			return err
		}
	}

	return nil
}

func (b *Bundle) newServiceAccount(name string) error {
	obj, err := b.Scheme().New(corev1.SchemeGroupVersion.WithKind("ServiceAccount"))
	if err != nil {
		return err
	}

	svcAcc := obj.(*corev1.ServiceAccount)
	svcAcc.Namespace = b.installNamespace
	svcAcc.Name = name
	return b.StoreObjects(svcAcc.Name+"_generated.yaml", svcAcc) // TODO(ryantking): Better name

}

// TODO Finish moving the StoreCalls to inside of the new* functions

func (b *Bundle) newDeployment(
	depSpec operatorsv1alpha1.StrategyDeploymentSpec,
	annotations map[string]string,
) error {
	obj, err := b.Scheme().New(appsv1.SchemeGroupVersion.WithKind("Deployment"))
	if err != nil {
		return err
	}

	dep := obj.(*appsv1.Deployment)
	dep.Namespace = b.installNamespace
	dep.Name = depSpec.Name
	dep.Labels = depSpec.Label
	dep.Annotations = annotations
	dep.Spec = depSpec.Spec
	return b.StoreObjects(dep.Name+"_generated.yaml", dep) // TODO(ryantking): Better name
}

func (b *Bundle) newRoles(
	name string,
	permission operatorsv1alpha1.StrategyDeploymentPermissions,
) error {
	roles := make([]*rbacv1.Role, 0, len(b.targetNamespaces))
	obj, err := b.Scheme().New(rbacv1.SchemeGroupVersion.WithKind("Role"))
	if err != nil {
		return err
	}

	role := obj.(*rbacv1.Role)
	role.Name = name
	role.Rules = permission.Rules
	for _, ns := range b.targetNamespaces {
		if ns == "" {
			continue
		}

		role := *role
		role.Namespace = ns
		roles = append(roles, &role)
	}

	return b.StoreObjects(role.Name+"_generated.yaml", role) // TODO(ryantking): Better name
}

func (b *Bundle) newRoleBindings(
	name string,
	permission operatorsv1alpha1.StrategyDeploymentPermissions,
) error {
	roleBindings := make([]*rbacv1.RoleBinding, 0, len(b.targetNamespaces))
	obj, err := b.Scheme().New(rbacv1.SchemeGroupVersion.WithKind("RoleBinding"))
	if err != nil {
		return err
	}

	roleBinding := obj.(*rbacv1.RoleBinding)
	roleBinding.Name = name
	roleBinding.Subjects = []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      permission.ServiceAccountName,
			Namespace: b.installNamespace,
		},
	}
	roleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "Role",
		Name:     name,
	}

	for _, ns := range b.targetNamespaces {
		if ns == "" {
			continue
		}

		roleBinding := *roleBinding
		roleBinding.Namespace = ns
		roleBindings = append(roleBindings, &roleBinding)
	}

	return b.StoreObjects(roleBinding.Name+"_generated.yaml", roleBinding) // TODO(ryantking): Better name
}

func (b *Bundle) newClusterRoles(
	name string,
	permission operatorsv1alpha1.StrategyDeploymentPermissions,
) error {
	roles := make([]*rbacv1.ClusterRole, 0, len(b.targetNamespaces))
	obj, err := b.Scheme().New(rbacv1.SchemeGroupVersion.WithKind("ClusterRole"))
	if err != nil {
		return err
	}

	role := obj.(*rbacv1.ClusterRole)
	role.Name = name
	role.Rules = permission.Rules
	for _, ns := range b.targetNamespaces {
		if ns == "" {
			continue
		}

		role := *role
		role.Namespace = ns
		roles = append(roles, &role)
	}

	return b.StoreObjects(role.Name+"_generated.yaml", role) // TODO(ryantking): Better name
}

func (b *Bundle) newClusterRoleBindings(
	name string,
	permission operatorsv1alpha1.StrategyDeploymentPermissions,
) error {
	roleBindings := make([]*rbacv1.ClusterRoleBinding, 0, len(b.targetNamespaces))
	obj, err := b.Scheme().New(rbacv1.SchemeGroupVersion.WithKind("ClusterRoleBinding"))
	if err != nil {
		return err
	}

	roleBinding := obj.(*rbacv1.ClusterRoleBinding)
	roleBinding.Name = name
	roleBinding.Subjects = []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      permission.ServiceAccountName,
			Namespace: b.installNamespace,
		},
	}
	roleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     name,
	}

	for _, ns := range b.targetNamespaces {
		if ns == "" {
			continue
		}

		roleBinding := *roleBinding
		roleBinding.Namespace = ns
		roleBindings = append(roleBindings, &roleBinding)
	}

	return b.StoreObjects(roleBinding.Name+"_generated.yaml", roleBinding) // TODO(ryantking): Better name
}

func (b Bundle) generateName(base string, o interface{}) string {
	const maxNameLength = 63
	hasher := fnv.New32a()

	b.deepHashObject(hasher, o)
	hashStr := rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
	if len(base)+len(hashStr) > maxNameLength {
		base = base[:maxNameLength-len(hashStr)-1]
	}

	return fmt.Sprintf("%s-%s", base, hashStr)
}

// deepHashObject writes specified object to hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func (Bundle) deepHashObject(hasher hash.Hash, objectToWrite interface{}) {
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
