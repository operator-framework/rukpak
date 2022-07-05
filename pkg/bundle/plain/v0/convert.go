package v0

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"testing/fstest"
	"time"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/rukpak/internal/util"
	"github.com/operator-framework/rukpak/pkg/bundle"
	registryv1 "github.com/operator-framework/rukpak/pkg/bundle/registry/v1"
	"github.com/operator-framework/rukpak/pkg/manifest"
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
	fsys := make(fstest.MapFS)
	b := Bundle{
		FS:             in.FS, // HACK: This is just so the scheme can be used during conversion.
		createdSvcAccs: make(map[string]struct{}),
	}
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
	if err := b.extractCsvDeployments(fsys, csv); err != nil {
		return nil, err
	}
	if err := b.extractCsvRBAC(fsys, csv); err != nil {
		return nil, err
	}

	b.FS = manifest.New(fsys, manifest.WithManifestDirs("manifests"))
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

func (b *Bundle) extractCsvDeployments(fsys fstest.MapFS, csv *operatorsv1alpha1.ClusterServiceVersion) error {
	annotations := csv.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["olm.targetNamespaces"] = strings.Join(b.targetNamespaces, ",")

	for _, depSpec := range csv.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		if err := b.newDeployment(fsys, depSpec, annotations); err != nil {
			return err
		}
		if err := b.newServiceAccount(fsys, depSpec.Spec.Template.Spec.ServiceAccountName); err != nil {
			return err
		}
	}

	return nil
}

func (b *Bundle) extractCsvRBAC(fsys fstest.MapFS, csv *operatorsv1alpha1.ClusterServiceVersion) error {
	for _, permission := range csv.Spec.InstallStrategy.StrategySpec.Permissions {
		name := b.generateName(
			fmt.Sprintf("%s-%s", csv.GetName(), permission.ServiceAccountName),
			[]interface{}{csv.GetName(), permission},
		)
		if err := b.newServiceAccount(fsys, permission.ServiceAccountName); err != nil {
			return err
		}
		if err := b.newRoles(fsys, name, permission); err != nil {
			return err
		}
		if err := b.newRoleBindings(fsys, name, permission); err != nil {
			return err
		}
		if err := b.newClusterRoles(fsys, name, permission); err != nil {
			return err
		}
		if err := b.newClusterRoleBindings(fsys, name, permission); err != nil {
			return err
		}
	}

	return nil
}

var serviceAccountGVK = corev1.SchemeGroupVersion.WithKind("ServiceAccount")

func (b *Bundle) newServiceAccount(fsys fstest.MapFS, name string) error {
	obj, err := b.Scheme().New(serviceAccountGVK)
	if err != nil {
		return err
	}

	svcAcc := obj.(*corev1.ServiceAccount)
	svcAcc.SetGroupVersionKind(serviceAccountGVK)
	svcAcc.Namespace = b.installNamespace
	svcAcc.Name = name
	return b.writeObjects(fsys, svcAcc.Name+"_generated.yaml", svcAcc) // TODO(ryantking): Better name
}

var deploymentGVK = appsv1.SchemeGroupVersion.WithKind("Deployment")

func (b *Bundle) newDeployment(
	fsys fstest.MapFS,
	depSpec operatorsv1alpha1.StrategyDeploymentSpec,
	annotations map[string]string,
) error {
	obj, err := b.Scheme().New(deploymentGVK)
	if err != nil {
		return err
	}

	dep := obj.(*appsv1.Deployment)
	dep.SetGroupVersionKind(deploymentGVK)
	dep.Namespace = b.installNamespace
	dep.Name = depSpec.Name
	dep.Labels = depSpec.Label
	dep.Annotations = annotations
	dep.Spec = depSpec.Spec
	return b.writeObjects(fsys, dep.Name+"_generated.yaml", dep) // TODO(ryantking): Better name
}

var roleGVK = rbacv1.SchemeGroupVersion.WithKind("Role")

func (b *Bundle) newRoles(
	fsys fstest.MapFS,
	name string,
	permission operatorsv1alpha1.StrategyDeploymentPermissions,
) error {
	roles := make([]client.Object, 0, len(b.targetNamespaces))
	gvk := rbacv1.SchemeGroupVersion.WithKind("Role")
	obj, err := b.Scheme().New(gvk)
	if err != nil {
		return err
	}

	role := obj.(*rbacv1.Role)
	role.SetGroupVersionKind(roleGVK)
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

	if len(roles) == 0 {
		return nil
	}

	return b.writeObjects(fsys, role.Name+"_generated.yaml", roles...) // TODO(ryantking): Better name
}

var roleBindingGVK = rbacv1.SchemeGroupVersion.WithKind("RoleBinding")

func (b *Bundle) newRoleBindings(
	fsys fstest.MapFS,
	name string,
	permission operatorsv1alpha1.StrategyDeploymentPermissions,
) error {
	roleBindings := make([]client.Object, 0, len(b.targetNamespaces))
	obj, err := b.Scheme().New(rbacv1.SchemeGroupVersion.WithKind("RoleBinding"))
	if err != nil {
		return err
	}

	roleBinding := obj.(*rbacv1.RoleBinding)
	roleBinding.SetGroupVersionKind(roleBindingGVK)
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

	if len(roleBindings) == 0 {
		return nil
	}

	return b.writeObjects(fsys, roleBinding.Name+"_generated.yaml", roleBindings...) // TODO(ryantking): Better name
}

var clusterRoleGVK = rbacv1.SchemeGroupVersion.WithKind("ClusterRole")

func (b *Bundle) newClusterRoles(
	fsys fstest.MapFS,
	name string,
	permission operatorsv1alpha1.StrategyDeploymentPermissions,
) error {
	roles := make([]client.Object, 0, len(b.targetNamespaces))
	obj, err := b.Scheme().New(clusterRoleGVK)
	if err != nil {
		return err
	}

	role := obj.(*rbacv1.ClusterRole)
	role.SetGroupVersionKind(clusterRoleGVK)
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

	if len(roles) == 0 {
		return nil
	}

	return b.writeObjects(fsys, role.Name+"_generated.yaml", roles...) // TODO(ryantking): Better name
}

var clusterRoleBindingGVK = rbacv1.SchemeGroupVersion.WithKind("ClusterRoleBinding")

func (b *Bundle) newClusterRoleBindings(
	fsys fstest.MapFS,
	name string,
	permission operatorsv1alpha1.StrategyDeploymentPermissions,
) error {
	roleBindings := make([]client.Object, 0, len(b.targetNamespaces))
	obj, err := b.Scheme().New(clusterRoleBindingGVK)
	if err != nil {
		return err
	}

	roleBinding := obj.(*rbacv1.ClusterRoleBinding)
	roleBinding.SetGroupVersionKind(clusterRoleBindingGVK)
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

	if len(roleBindings) == 0 {
		return nil
	}

	return b.writeObjects(fsys, roleBinding.Name+"_generated.yaml", roleBindings...) // TODO(ryantking): Validate name
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

func (b Bundle) writeObjects(fsys fstest.MapFS, name string, objs ...client.Object) error {
	var data bytes.Buffer

	enc := yaml.NewEncoder(&data)
	for _, obj := range objs {
		if err := enc.Encode(obj); err != nil {
			return fmt.Errorf("encoding %s.%s: %w", obj.GetNamespace(), obj.GetName(), err)
		}
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("closing encoder: %w", err)
	}

	fsys[filepath.Join("manifests", name)] = &fstest.MapFile{
		Data:    data.Bytes(),
		Mode:    os.ModePerm,
		ModTime: time.Now(),
		// TODO(ryantking): Worry about Sys at all?
		// Sys: nil,
	}

	return nil
}
