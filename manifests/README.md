# Rukpak Manifests

Manifests for Rukpak are organized into the `base/` and `overlays/` folders. This is to allow developers to develop with rukpak using alternative certificate management solutions by applying a separate overlay on top of the `base/` kustomization folder.

`base/` includes everything required to install rukpak except for certificate management related items. Those items are contained in a separate overlay, in this case `overlays/cert-manager`.

At the moment, rukpak contains just one overlay using [cert-manager](https://github.com/cert-manager/cert-manager). To run `kustomize` commands against the entire set of rukpak's manifests, the `manifests/overlays/cert-manager` folder should be targeted.