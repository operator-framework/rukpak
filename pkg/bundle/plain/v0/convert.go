package v0

import (
	"fmt"
	"hash/fnv"
	"strings"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/rukpak/internal/util"
	"github.com/operator-framework/rukpak/pkg/bundle"
	registryv1 "github.com/operator-framework/rukpak/pkg/bundle/registry/v1"
)

// WithInstallNamespace is an option for converting other bundles to `plain+v0`.
//
// The option returned by this function can be used in any conversion function that outputs a `plain+v0` bundle. The
// install namespace is the name of the Kubernetes namespace that the contents of the bundle should be installed to. It
// can sometimes be inferred by the contents of the source bundle, but not in every case. This option will override any
// inferred namespace.
func WithInstallNamespace(namespace string) func(*Bundle) {
	return func(b *Bundle) {
		b.installNamespace = namespace
	}
}

// WithTargetNamespaces is an option for converting other bundles to `plain+v0`.
//
// The option returned by this function can be used in any conversion function that outputs a `plain+v0` bundle. The
// target namespaces is a list of Kubernetes namespaces that the the bundle's resulting objects expect access to.
// Depending on the install mode, these namespaces are used when constructing RBAC manifests.
func WithTargetNamespaces(namespaces ...string) func(*Bundle) {
	return func(b *Bundle) {
		b.targetNamespaces = namespaces
	}
}

func init() {
	bundle.RegisterConversionFunc(convertFromRegistryV1)
}

func convertFromRegistryV1(in registryv1.Bundle, opts ...func(*Bundle)) (*Bundle, error) {
	b := Bundle{FS: in.FS, createdSvcAccs: make(map[string]struct{})}
	for _, opt := range opts {
		opt(&b)
	}
	csv, err := in.CSV()
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
	if _, ok := b.createdSvcAccs[name]; ok {
		return nil
	}

	obj, err := b.Scheme().New(corev1.SchemeGroupVersion.WithKind("ServiceAccount"))
	if err != nil {
		return err
	}

	svcAcc := obj.(*corev1.ServiceAccount)
	svcAcc.Namespace = b.installNamespace
	svcAcc.Name = name
	b.StoreObjects(svcAcc.Name+"_generated.yaml", svcAcc) // TODO(ryantking): Better name
	b.createdSvcAccs[name] = struct{}{}
	return nil
}

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
	b.StoreObjects(dep.Name+"_generated.yaml", dep) // TODO(ryantking): Better name
	return nil
}

func (b *Bundle) newRoles(
	name string,
	permission operatorsv1alpha1.StrategyDeploymentPermissions,
) error {
	roles := make([]client.Object, 0, len(b.targetNamespaces))
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

	b.StoreObjects(role.Name+"_generated.yaml", roles...) // TODO(ryantking): Better name
	return nil
}

func (b *Bundle) newRoleBindings(
	name string,
	permission operatorsv1alpha1.StrategyDeploymentPermissions,
) error {
	roleBindings := make([]client.Object, 0, len(b.targetNamespaces))
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

	b.StoreObjects(roleBinding.Name+"_generated.yaml", roleBindings...) // TODO(ryantking): Better name
	return nil
}

func (b *Bundle) newClusterRoles(
	name string,
	permission operatorsv1alpha1.StrategyDeploymentPermissions,
) error {
	roles := make([]client.Object, 0, len(b.targetNamespaces))
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

	b.StoreObjects(role.Name+"_generated.yaml", roles...) // TODO(ryantking): Better name
	return nil
}

func (b *Bundle) newClusterRoleBindings(
	name string,
	permission operatorsv1alpha1.StrategyDeploymentPermissions,
) error {
	roleBindings := make([]client.Object, 0, len(b.targetNamespaces))
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

	b.StoreObjects(roleBinding.Name+"_generated.yaml", roleBindings...) // TODO(ryantking): Better name
	return nil
}

func (b Bundle) generateName(base string, o interface{}) string {
	const maxNameLength = 63
	hasher := fnv.New32a()

	util.DeepHashObject(hasher, o)
	hashStr := rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
	if len(base)+len(hashStr) > maxNameLength {
		base = base[:maxNameLength-len(hashStr)-1]
	}

	return fmt.Sprintf("%s-%s", base, hashStr)
}
