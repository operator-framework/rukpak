# Http source

## Summary

The http source provides a compressed archive file (`tgz` format) downloadable by the http protocol as the source of the bundle.
The `source.type` for the http source is `http`. When creating a http source, a URL of the compressed archive file must be specified.
It is expected that a proper format of bundle content is present
in the compressed archive file.

## Example

Referencing a compressed archive file in a github repository release archive:

```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: BundleDeployment
metadata:
  name: my-ahoy
spec:
  provisionerClassName: core-rukpak-io-helm
  template:
    metadata:
      labels:
        app: my-ahoy
    spec:
      provisionerClassName: core-rukpak-io-helm
      source:
        http:
          url: https://github.com/helm/examples/releases/download/hello-world-0.1.0/hello-world-0.1.0.tgz  
        type: http
```

## Authorization

An http source can provide authorization for access to private compressed archives by creating a secret in the namespace that the provisioner is deployed.
The secret used in the http source is based around [Basic authentication secret](https://kubernetes.io/docs/concepts/configuration/secret/#basic-authentication-secret)
and is expected to contain `data.username` and `data.password` for the username and password, respectively.
If the `http.auth.secret.insecureSkipVerify` is set true, the download operation will accept any certificate presented by the server and any host name in that
certificate. In this mode, TLS is susceptible to machine-in-the-middle attacks unless custom verification is
used. This should be used only for testing.

### Example with authorization

1. Create the secret

```sh
kubectl create secret generic accesssecret --type "kubernetes.io/basic-auth" --from-literal=username=myusername --from-literal=password=mypassword -n rukpak-system
```

2. Create a bundle deployment referencing a private compressed archive file:

```bash
kubectl apply -f -<<EOF
apiVersion: core.rukpak.io/v1alpha1
kind: BundleDeployment
metadata:
  name: my-ahoy
spec:
  provisionerClassName: core-rukpak-io-helm
  template:
    metadata:
      labels:
        app: my-ahoy
    spec:
      provisionerClassName: core-rukpak-io-helm
      source:
        http:
          url: https://github.com/helm/examples/releases/download/hello-world-0.1.0/hello-world-0.1.0.tgz
          auth:
            secret:
              name: accesssecret
        type: http
EOF
```
