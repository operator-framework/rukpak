package probing

import (
	"errors"
	"fmt"

	"github.com/google/cel-go/cel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apiserver/pkg/cel/library"
)

type celProbe struct {
	Program cel.Program
	Message string
}

var ErrCELInvalidEvaluationType = errors.New(
	"cel expression must evaluate to a bool")

func newCELProbe(rule, message string) (*celProbe, error) {
	env, err := cel.NewEnv(
		cel.Variable("self", cel.DynType),
		cel.HomogeneousAggregateLiterals(),
		cel.EagerlyValidateDeclarations(true),
		cel.DefaultUTCTimeZone(true),

		// TODO: this doesn't exist in the working version of the cel library
		// ext.Strings(ext.StringsVersion(0)),
		library.URLs(),
		library.Regex(),
		library.Lists(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating CEL env: %w", err)
	}

	ast, issues := env.Compile(rule)
	if issues != nil {
		return nil, fmt.Errorf("compiling CEL: %w", issues.Err())
	}
	if ast.OutputType() != cel.BoolType {
		return nil, ErrCELInvalidEvaluationType
	}

	prgm, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("CEL program failed: %w", err)
	}

	return &celProbe{
		Program: prgm,
		Message: message,
	}, nil
}

func (p *celProbe) Probe(obj *unstructured.Unstructured) (success bool, message string) {
	val, _, err := p.Program.Eval(map[string]any{
		"self": obj.Object,
	})
	if err != nil {
		return false, fmt.Sprintf("CEL program failed: %v", err)
	}

	return val.Value().(bool), p.Message
}
