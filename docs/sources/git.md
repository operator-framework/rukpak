# Git source

## Summary

The git source provides the contents in a git repository as the source of the bundle.  The `source.type` for the git source is `git`.
When creating a git source, a reference to a particular commit, tag, or branch must be provided in addition to the URL of the
repository and an optional directory in the repository. It is expected that a proper format of bundle content is present
in the particular commit/tag/branch at the directory specified.

## Examples

### Referencing a git repository by a commit

```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: Bundle
metadata:
  name: combo-commit-ref
spec:
  source:
    type: git
    git:
      ref:
        commit: d40082c96e6f0d297aa316d84020d307f95dc453
      repository: https://github.com/operator-framework/combo
  provisionerClassName: core-rukpak-io-plain
```

### Referencing a git repository by a tag

```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: Bundle
metadata:
  name: combo-tag-ref
spec:
  source:
    type: git
    git:
      ref:
        tag: v0.0.2
      repository: https://github.com/operator-framework/combo
  provisionerClassName: core-rukpak-io-plain
```

### Referencing a git repository by a branch

```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: Bundle
metadata:
  name: combo-branch-ref
spec:
  source:
    type: git
    git:
      ref:
        branch: main
      repository: https://github.com/operator-framework/combo
  provisionerClassName: core-rukpak-io-plain
```

### Referencing a different content directory than the default

```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: Bundle
metadata:
  name: combo-branch-ref-deploy
spec:
  source:
    type: git
    git:
      ref:
        branch: main
      directory: ./dev/deploy
      repository: https://github.com/exdx/combo-bundle
  provisionerClassName: core-rukpak-io-plain
```

## Private git repositories

A git source can reference contents in a private git repository by creating a secret in the namespace that the provisioner is deployed.
For `https` URL, the secret is a [Basic authentication secret](https://kubernetes.io/docs/concepts/configuration/secret/#basic-authentication-secret)
and expected to contain `data.username` and `data.password` for the username and password, respectively.
For `ssh` URL, the secret is a [SSH authentication secrets](https://kubernetes.io/docs/concepts/configuration/secret/#ssh-authentication-secrets)
and expected to contain `data.ssh-privatekey` and `data.ssh-knownhosts` for the ssh privatekey and the host entry in the known_hosts file respectively.
If the `git.auth.insecureSkipVerify` is set true, the clone operation will accept any certificate presented by the server and any host name in that
certificate. In this mode, TLS is susceptible to machine-in-the-middle attacks unless custom verification is
used. This should be used only for testing.

### Example steps for `https` URL

1. Create the secret

```sh
echo -n 'user name' > username.txt
echo -n 'password' > password.txt
kubectl create secret generic gitsecret --type "kubernetes.io/basic-auth" --from-file=username=./username.txt --from-file=password=./password.txt -n rukpak-system
```

2. Find an existing private git repository or create a new one that is private by default

3. Create a bundle referencing a private git repository:

```bash
kubectl apply -f -<<EOF
apiVersion: core.rukpak.io/v1alpha1
kind: Bundle
metadata:
  name: combo-private
spec:
  source:
    type: git
    git:
      ref:
        branch: main
      repository: https://github.com/<username>/combo
      auth:
        secret:
          name: gitsecret
          insecureSkipVerify: false
  provisionerClassName: core-rukpak-io-plain
EOF
```

### Example steps for ssh URL

1. Create the secret

```sh
cat ~/.ssh/known_hosts | grep 'github' > ssh_knownhosts.txt
kubectl create secret generic gitsecret --type "kubernetes.io/ssh-auth" --from-file=ssh-privatekey=~/.ssh/id_rsa --from-file=ssh-knownhosts=./ssh_konwnhosts.txt -n rukpak-system
```

2. Find an existing private git repository or create a new one that is private by default

3. Create a bundle referencing a private git repository:

```bash
kubectl apply -f -<<EOF
apiVersion: core.rukpak.io/v1alpha1
kind: Bundle
metadata:
  name: combo-private
spec:
  source:
    type: git
    git:
      ref:
        branch: main
      repository: git@github.com:<username>/combo
      auth:
        secret:
          name: gitsecret
          insecureSkipVerify: false
  provisionerClassName: core-rukpak-io-plain
EOF
```

