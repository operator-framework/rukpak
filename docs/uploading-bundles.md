# Uploading Bundles

## Summary

A Bundle can reference content from an uploaded bundle directory instead of a remote container image or a git repository by
using the `upload` source type in the Bundle manifest. This enables one to easily source content locally without external
repositories/registries.

The `rukpakctl run` command can be used to create or update a `BundleDeployment` that references
an `upload` bundle.

## Running an `upload` bundle

To run an `upload` bundle, simply invoke the `rukpakctl run` subcommand with a `BundleDeployment` name and the path
to a local directory that contains a bundle.

```console
rukpakctl run <bundleDeploymentName> <bundleDir>
```

By default, `rukpakctl run` assumes that the bundle is a plain bundle and uses `core.rukpak.io/plain` as the
provisioner class names for both the bundle template and the bundle deployment spec.

The `--bundle-provisioner-class` and `--bundle-deployment-provisioner-class` flags can be used to
configure the provisioner classes that are used to unpack and deploy the bundle, respectively.

## Updating an existing `upload` bundle

The `upload` source type also supports pivoting from one `upload` bundle to another. To initiate
a pivot, simply run the same `rukpakctl run` command with a bundle directory that contains different
content than the existing bundle. The `rukpakctl` client will automatically hash the contents of the bundle
directory and include that hash as a label in the bundle template to ensure that the template changes,
thus causing the bundle deployment provisioner to begin a pivot.

Because hashing is used to determine differences between bundle revisions, `rukpakctl run` supports
an iterative development cycle of:
1. Make changes in the bundle directory
2. Run `rukpakctl run`
3. Goto step 1

## A note about immutability

The `upload` source upload handler rejects uploads for non-`upload` bundles and for `upload` bundles
that have already been unpacked. This preserves the immutable property of bundles.

## Example

1. Run the bundle from a local testdata directory
   ```console
   $ rukpakctl run combo ../testdata/bundles/plain-v0/valid
   bundledeployment.core.rukpak.io "combo" applied
   successfully uploaded bundle content for "combo-7fdb455bf7"
   ```

2. Check the status of the bundle
   ```console
   $ kubectl get bundle -l app=combo
   NAME               TYPE     PHASE      AGE
   combo-7fdb455bf7   upload   Unpacked   104s
   ```

3. Check the status of the bundle deployment
   ```console
   $ kubectl get bundledeployments.core.rukpak.io combo
   NAME    ACTIVE BUNDLE      INSTALL STATE           AGE
   combo   combo-7fdb455bf7   InstallationSucceeded   2m46s
   ```