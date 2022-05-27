package bundle

// RegistryV1Option is an option that can configure the conversion of a registry+v1 bundle.
type Option interface {
	// this is private to avoid exposing the options struct to the public API
	apply(*options)
}

// WithInstallNamespace overrides the install namespace with the given value.
func WithInstallNamespace(namespace string) Option {
	return optionFunc(func(opts *options) {
		opts.installNamespace = namespace
	})
}

// WithTargetNamespaces overrides the target namespace with the given values.
func WithTargetNamespaces(namespaces []string) Option {
	return optionFunc(func(opts *options) {
		opts.targetNamespaces = namespaces
	})
}

type options struct {
	installNamespace string
	targetNamespaces []string
}

type optionFunc func(*options)

func (f optionFunc) apply(opts *options) {
	f(opts)
}
