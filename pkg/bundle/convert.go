package bundle

import (
	"fmt"
	"io/fs"
)

var conversionFuncs = map[string]interface{}{}

func conversionKey[IN, OUT fs.FS]() string {
	var (
		in  IN
		out OUT
	)

	return fmt.Sprintf("%T->%T", in, out)
}

// RegisterConversionFunc informs the package how to convert from one bundle format to another.
//
// This function is best put in the `init` function of packages that define bundle formats.
func RegisterConversionFunc[IN, OUT fs.FS](f func(bundle IN, opts ...func(*OUT)) (*OUT, error)) {
	conversionFuncs[conversionKey[IN, OUT]()] = f
}

// Convert attempts to convert one bundle to a different format.
//
// It can only perform conversions that it has explicitly been made aware of via `RegisterConversionFunc`.
// All options are expected to be defined in the package that provides the type for OUT.
// For example, options for `plainv0.Bundle` will also be in the `plainv0` package.
func Convert[IN, OUT fs.FS](in IN, opts ...func(*OUT)) (*OUT, error) {
	f, ok := conversionFuncs[conversionKey[IN, OUT]()]
	if !ok {
		var out OUT
		return nil, fmt.Errorf("cannot convert from %T to %T", in, out)
	}

	return f.(func(IN, ...func(*OUT)) (*OUT, error))(in, opts...)
}
