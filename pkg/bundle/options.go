package bundle

// RegistryV1Option is an option that can configure the conversion of a registry+v1 bundle.
type Option interface {
	applyToPlainV1(r *RegistryV1)
}

type optionFunc func(*RegistryV1)

func (f optionFunc) applyToPlainV1(r *RegistryV1) {
	f(r)
}

// WithInstallNamespace overrides the install namespace with the given value.
func WithInstallNamespace(namespace string) Option {
	return optionFunc(func(r *RegistryV1) {
		r.overrides.installNamespace = namespace
	})
}

// WithTargetNamespaces overrides the target namespace with the given values.
func WithTargetNamespaces(namespaces []string) Option {
	return optionFunc(func(r *RegistryV1) {
		r.overrides.targetNamespaces = namespaces
	})
}
