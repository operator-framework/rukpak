package git

import (
	"errors"
	"fmt"
	"testing"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

func TestCheckoutCommand(t *testing.T) {
	var gitSources = []struct {
		source   rukpakv1alpha1.GitSource
		expected string
		err      error
	}{
		{
			source: rukpakv1alpha1.GitSource{
				Repository: "https://github.com/operator-framework/combo",
				Ref: rukpakv1alpha1.GitRef{
					Commit: "4567031e158b42263e70a7c63e29f8981a4a6135",
				},
			},
			expected: fmt.Sprintf("git clone %s %s && cd %s && git checkout %s && rm -r .git && cp -r %s/. /bundle",
				"https://github.com/operator-framework/combo", repositoryName, repositoryName, "4567031e158b42263e70a7c63e29f8981a4a6135",
				"./"),
		},
		{
			source: rukpakv1alpha1.GitSource{
				Repository: "https://github.com/operator-framework/combo",
				Ref: rukpakv1alpha1.GitRef{
					Tag: "v0.0.1",
				},
			},
			expected: fmt.Sprintf("git clone --depth 1 --branch %s %s %s && cd %s && git checkout tags/%s && rm -r .git && cp -r %s/. /bundle",
				"v0.0.1", "https://github.com/operator-framework/combo", repositoryName, repositoryName, "v0.0.1", "./"),
		},
		{
			source: rukpakv1alpha1.GitSource{
				Repository: "https://github.com/operator-framework/combo",
				Directory:  "./deploy",
				Ref: rukpakv1alpha1.GitRef{
					Branch: "dev",
				},
			},
			expected: fmt.Sprintf("git clone --depth 1 --branch %s %s %s && cd %s && git checkout %s && rm -r .git && cp -r %s/. /bundle",
				"dev", "https://github.com/operator-framework/combo", repositoryName, repositoryName, "dev", "./deploy"),
		},
		{
			source: rukpakv1alpha1.GitSource{
				Repository: "https://github.com/operator-framework/combo.git",
			},
			expected: "",
			err:      errors.New("must specify one of the git source options: one of [Branch|Commit|Tag]"),
		},
		{
			source: rukpakv1alpha1.GitSource{
				Repository: "https://github.com/operator-framework/combo",
				Ref: rukpakv1alpha1.GitRef{
					Branch: "dev",
					Tag:    "0.0.1",
				},
			},
			expected: "",
			err:      errors.New("cannot specify both branch and tag: only one is allowed"),
		},
		{
			source: rukpakv1alpha1.GitSource{
				Repository: "https://github.com/operator-framework/combo",
				Ref: rukpakv1alpha1.GitRef{
					Tag:    "0.0.1",
					Commit: "d40082c96e6f0d297aa316d84020d307f95dc453",
				},
			},
			expected: "",
			err:      errors.New("cannot specify both commit and tag: only one is allowed"),
		},
		{
			source: rukpakv1alpha1.GitSource{
				Repository: "https://github.com/operator-framework/combo",
				Ref: rukpakv1alpha1.GitRef{
					Branch: "dev",
					Commit: "d40082c96e6f0d297aa316d84020d307f95dc453",
				},
			},
			expected: "",
			err:      errors.New("cannot specify both branch and commit: only one is allowed"),
		},
	}

	for _, tt := range gitSources {
		result, err := CloneCommandFor(tt.source)
		if tt.err != nil && tt.err.Error() != err.Error() {
			t.Fatalf("expected error %s, got %s", tt.err.Error(), err.Error())
		}
		if result != tt.expected {
			t.Fatalf("expected %s, got %s", tt.expected, result)
		}
	}
}
