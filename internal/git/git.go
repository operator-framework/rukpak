package git

import (
	"errors"
	"fmt"
	"net/url"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

const (
	defaultDirectory = "./"
	repositoryName   = "repo"
	sslNoVerify      = "GIT_SSL_NO_VERIFY=true "
)

type checkoutCmd struct {
	rukpakv1alpha1.GitSource
}

func CloneCommandFor(s rukpakv1alpha1.GitSource) (string, error) {
	cmd := &checkoutCmd{GitSource: s}
	err := cmd.Validate()
	if err != nil {
		return "", err
	}
	return cmd.String()
}

func (c *checkoutCmd) String() (string, error) {
	var checkoutCommand string
	var repository = c.Repository
	var directory = c.Directory
	var branch = c.Ref.Branch
	var commit = c.Ref.Commit
	var tag = c.Ref.Tag
	var preset string

	if c.SecretName != "" {
		repositoryURL, err := url.Parse(repository)
		if err != nil {
			return "", fmt.Errorf("authorization is supported for https: %w", err)
		}
		repositoryURL.User = url.UserPassword("$USER", "$TOKEN")
		repository = repositoryURL.String()
	}
	if c.SslNoVerify {
		preset = sslNoVerify
	}

	if directory == "" {
		directory = defaultDirectory
	}

	if commit != "" {
		checkoutCommand = fmt.Sprintf("%sgit clone %s %s && cd %s && %sgit checkout %s && rm -r .git && cp -r %s/. /bundle",
			preset, repository, repositoryName, repositoryName, preset, commit, directory)
		return checkoutCommand, nil
	}

	if tag != "" {
		checkoutCommand = fmt.Sprintf("%sgit clone --depth 1 --branch %s %s %s && cd %s && %sgit checkout tags/%s && rm -r .git && cp -r %s/. /bundle",
			preset, tag, repository, repositoryName, repositoryName, preset, tag, directory)
		return checkoutCommand, nil
	}

	checkoutCommand = fmt.Sprintf("%sgit clone --depth 1 --branch %s %s %s && cd %s && %sgit checkout %s && rm -r .git && cp -r %s/. /bundle",
		preset, branch, repository, repositoryName, repositoryName, preset, branch, directory)
	return checkoutCommand, nil
}

func (c *checkoutCmd) Validate() error {
	var branch = c.Ref.Branch
	var commit = c.Ref.Commit
	var tag = c.Ref.Tag

	var branchSet = branch != ""
	var commitSet = commit != ""
	var tagSet = tag != ""

	if !branchSet && !commitSet && !tagSet {
		return errors.New("must specify one of the git source options: one of [Branch|Commit|Tag]")
	}

	if branchSet && commitSet {
		return errors.New("cannot specify both branch and commit: only one is allowed")
	}

	if branchSet && tagSet {
		return errors.New("cannot specify both branch and tag: only one is allowed")
	}

	if commitSet && tagSet {
		return errors.New("cannot specify both commit and tag: only one is allowed")
	}

	return nil
}
