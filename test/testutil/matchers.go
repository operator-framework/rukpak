package testutil

import (
	"fmt"
	"reflect"

	"github.com/onsi/gomega/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EqualObject returns a matcher that compares two Kubernetes objects
func EqualObject(expected interface{}) types.GomegaMatcher {
	return objectMatcher{expected}
}

type objectMatcher struct {
	expected interface{}
}

func (matcher objectMatcher) Match(actual interface{}) (success bool, err error) {
	if _, ok := matcher.expected.(client.Object); !ok {
		return false, fmt.Errorf("EqualObject matcher expects a client.Object")
	}
	if _, ok := actual.(client.Object); !ok {
		return false, fmt.Errorf("EqualObject matcher expects a client.Object")
	}

	return reflect.DeepEqual(actual, matcher.expected), nil
}

func (matcher objectMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%#v\nto equal \n\t%#v", actual, matcher.expected)
}

func (matcher objectMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%#v\nnot to equal\n\t%#v", actual, matcher.expected)
}
