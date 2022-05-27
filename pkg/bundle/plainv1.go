package bundle

import (
	"fmt"
	"hash"
	"hash/fnv"
	"io/fs"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/operator-framework/rukpak/pkg/manifest"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PlainV0 holds a plain v1 bundle.
type PlainV0 struct {
	manifest.FS

	// TODO(ryantking): Do we need this breakdown? Or can we just store them as a single []client.Object
	deployments         []*appsv1.Deployment
	serviceAccounts     map[string]*corev1.ServiceAccount
	roles               []*rbacv1.Role
	roleBindings        []*rbacv1.RoleBinding
	clusterRoles        []*rbacv1.ClusterRole
	clusterRoleBindings []*rbacv1.ClusterRoleBinding
	crds                []*apiextensionsv1.CustomResourceDefinition
	others              []client.Object
}

// NewPlainV0 reads in a filesystem that contains a plain+v0 bundle.
//
// If the filesystem is itself another known bundle format, it will convert it to the plain+v0 format.
// Otherwise it will treat the filesystem as a valid plain+v0 layout and parse its contents into manifests.
func NewPlainV0(fsys fs.FS, opts ...Option) (*PlainV0, error) {
	var allOpts options
	for _, opt := range opts {
		opt.apply(&allOpts)
	}

	switch bundle := fsys.(type) {
	case RegistryV1:
		return newPlainV0FromRegistryV1(bundle, allOpts)
	default:
		return newPlainV0FromFS(fsys, allOpts)
	}
}

// Open opens a manifest flile.
func (b PlainV0) Open(name string) (fs.File, error) {
	return b.FS.Open(strings.TrimPrefix(name, "/manifests"))
}

// Manifests returns all objects contained in the bundle.
func (b PlainV0) Manifests() []client.Object {
	objs := make(
		[]client.Object, 0,
		len(b.deployments)+len(b.serviceAccounts)+len(b.roles)+len(b.roleBindings)+
			len(b.clusterRoles)+len(b.clusterRoleBindings),
	)

	for name := range b.serviceAccounts {
		objs = append(objs, b.serviceAccounts[name])
	}
	addToObjectSlice(&objs, b.deployments)
	addToObjectSlice(&objs, b.roles)
	addToObjectSlice(&objs, b.roleBindings)
	addToObjectSlice(&objs, b.clusterRoles)
	addToObjectSlice(&objs, b.clusterRoleBindings)
	addToObjectSlice(&objs, b.crds)
	addToObjectSlice(&objs, b.others)
	return objs
}

func addToObjectSlice[T client.Object](objs *[]client.Object, newObjs []T) {
	for i := range newObjs {
		*objs = append(*objs, newObjs[i])
	}
}

func newPlainV0FromFS(baseFS fs.FS, opts options) (*PlainV0, error) {
	fsys, err := newManifestFS(baseFS)
	if err != nil {
		return nil, err
	}

	// TODO(ryantking): Parse the objects into their categories

	return &PlainV0{FS: fsys}, nil
}

func newPlainV0FromRegistryV1(srcBundle RegistryV1, opts options) (*PlainV0, error) {
	bundle := PlainV0{
		FS:              srcBundle.FS,
		crds:            srcBundle.crds,
		serviceAccounts: make(map[string]*corev1.ServiceAccount),
		others:          srcBundle.others,
	}
	bundle.setDefaultOptions(srcBundle, &opts)
	if err := bundle.validateTargetNamespaces(srcBundle, opts); err != nil {
		return nil, err
	}

	// TODO(ryantking): All the new manifests should be added to a file in the filesystem.
	if err := bundle.convertRegistryV1Deployments(srcBundle, opts); err != nil {
		return nil, err
	}
	if err := bundle.convertRegistryV1RBAC(srcBundle, opts); err != nil {
		return nil, err
	}
	if err := bundle.convertRegistryV1ClusterRBAC(srcBundle, opts); err != nil {
		return nil, err
	}

	return &bundle, nil
}

func (b *PlainV0) setDefaultOptions(srcBundle RegistryV1, opts *options) {
	if opts.installNamespace == "" {
		opts.installNamespace = srcBundle.CSV().Annotations["operatorframework.io/suggested-namespace"]
	}
	if opts.installNamespace == "" {
		// TODO(ryantking): Does srcBundle.CSV().GetName() give us package name
		// per @joelanford's install namespace detection suggestion?
		opts.installNamespace = fmt.Sprintf("%s-system", srcBundle.csv.GetName())
	}

	supportedInstallModes := b.supportedInstallModes(srcBundle)
	if opts.targetNamespaces == nil {
		if supportedInstallModes.Has(string(v1alpha1.InstallModeTypeAllNamespaces)) {
			opts.targetNamespaces = []string{}
		} else if supportedInstallModes.Has(string(v1alpha1.InstallModeTypeOwnNamespace)) {
			opts.targetNamespaces = []string{opts.installNamespace}
		}
	}
}

func (b PlainV0) validateTargetNamespaces(srcBundle RegistryV1, opts options) error {
	var (
		set                   = sets.NewString(opts.targetNamespaces...)
		supportedInstallModes = b.supportedInstallModes(srcBundle)
		allNamespaces         = supportedInstallModes.Has(string(v1alpha1.InstallModeTypeAllNamespaces))
		singleNamespace       = supportedInstallModes.Has(string(v1alpha1.InstallModeTypeSingleNamespace))
		ownNamespace          = supportedInstallModes.Has(string(v1alpha1.InstallModeTypeOwnNamespace))
		multiNamespace        = supportedInstallModes.Has(string(v1alpha1.InstallModeTypeMultiNamespace))
	)

	if set.Len() == 0 && allNamespaces {
		return nil
	}
	if set.Len() == 1 &&
		((set.Has("") && allNamespaces) ||
			(singleNamespace && !set.Has(opts.installNamespace)) ||
			(ownNamespace && set.Has(opts.installNamespace))) {
		return nil
	}
	if set.Len() > 1 && multiNamespace {
		return nil
	}

	return fmt.Errorf(
		"supported install modes %v do not support target namespaces %v",
		supportedInstallModes.List(), opts.targetNamespaces,
	)
}

func (b PlainV0) supportedInstallModes(srcBundle RegistryV1) sets.String {
	supportedInstallModes := sets.NewString()
	for _, im := range srcBundle.CSV().Spec.InstallModes {
		if im.Supported {
			supportedInstallModes.Insert(string(im.Type))
		}
	}

	return supportedInstallModes
}

func (b *PlainV0) convertRegistryV1Deployments(srcBundle RegistryV1, opts options) error {
	annotations := srcBundle.CSV().Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["olm.targetNamespaces"] = strings.Join(opts.targetNamespaces, ",")

	for _, depSpec := range srcBundle.CSV().Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		obj, err := scheme.New(appsv1.SchemeGroupVersion.WithKind("Deployment"))
		if err != nil {
			return err
		}

		dep := obj.(*appsv1.Deployment)
		dep.Namespace = opts.installNamespace
		dep.Name = depSpec.Name
		dep.Labels = depSpec.Label
		dep.Annotations = annotations
		dep.Spec = depSpec.Spec
		b.deployments = append(b.deployments, dep)

		if err := b.addServiceAccount(depSpec.Spec.Template.Spec.ServiceAccountName, opts.installNamespace); err != nil {
			return err
		}
	}

	return nil
}

func (b *PlainV0) convertRegistryV1RBAC(srcBundle RegistryV1, opts options) error {
	for _, ns := range opts.targetNamespaces {
		if ns == "" {
			continue
		}

		for _, permission := range srcBundle.CSV().Spec.InstallStrategy.StrategySpec.Permissions {
			if err := b.addServiceAccount(permission.ServiceAccountName, opts.installNamespace); err != nil {
				return err
			}

			name := b.generateName(
				fmt.Sprintf("%s-%s", srcBundle.csv.GetName(), permission.ServiceAccountName),
				[]interface{}{srcBundle.csv.GetName(), permission},
			)

			obj, err := scheme.New(rbacv1.SchemeGroupVersion.WithKind("Role"))
			if err != nil {
				return err
			}

			role := obj.(*rbacv1.Role)
			role.Name = name
			role.Namespace = ns
			role.Rules = permission.Rules
			b.roles = append(b.roles, role)

			obj, err = scheme.New(rbacv1.SchemeGroupVersion.WithKind("RoleBinding"))
			if err != nil {
				return err
			}

			roleBinding := obj.(*rbacv1.RoleBinding)
			roleBinding.Name = name
			roleBinding.Namespace = ns
			roleBinding.Subjects = []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      permission.ServiceAccountName,
					Namespace: opts.installNamespace,
				},
			}
			roleBinding.RoleRef = rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     name,
			}
			b.roleBindings = append(b.roleBindings, roleBinding)
		}
	}

	return nil
}

func (b *PlainV0) convertRegistryV1ClusterRBAC(srcBundle RegistryV1, opts options) error {
	for _, permission := range srcBundle.CSV().Spec.InstallStrategy.StrategySpec.ClusterPermissions {
		if err := b.addServiceAccount(permission.ServiceAccountName, opts.installNamespace); err != nil {
			return err
		}

		name := b.generateName(
			fmt.Sprintf("%s-%s", srcBundle.csv.GetName(), permission.ServiceAccountName),
			[]interface{}{srcBundle.csv.GetName(), permission},
		)

		obj, err := scheme.New(rbacv1.SchemeGroupVersion.WithKind("ClusterRole"))
		if err != nil {
			return err
		}

		role := obj.(*rbacv1.ClusterRole)
		role.Name = name
		role.Rules = permission.Rules
		b.clusterRoles = append(b.clusterRoles, role)

		obj, err = scheme.New(rbacv1.SchemeGroupVersion.WithKind("ClusterRoleBinding"))
		if err != nil {
			return err
		}

		roleBinding := obj.(*rbacv1.ClusterRoleBinding)
		roleBinding.Name = name
		roleBinding.Subjects = []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      permission.ServiceAccountName,
				Namespace: opts.installNamespace,
			},
		}
		roleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     name,
		}
		b.clusterRoleBindings = append(b.clusterRoleBindings, roleBinding)
	}

	return nil
}

func (b *PlainV0) addServiceAccount(name, namespace string) error {
	obj, err := scheme.New(corev1.SchemeGroupVersion.WithKind("ServiceAccount"))
	if err != nil {
		return err
	}

	svcAcc := obj.(*corev1.ServiceAccount)
	svcAcc.Namespace = namespace
	svcAcc.Name = name
	b.serviceAccounts[name] = svcAcc
	return nil
}

func (b PlainV0) generateName(base string, o interface{}) string {
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
func (b PlainV0) deepHashObject(hasher hash.Hash, objectToWrite interface{}) {
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
