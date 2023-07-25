# This loads a helper function that isn't part of core Tilt that simplifies restarting the process in the container
# when files changes.
load('ext://restart_process', 'docker_build_with_restart')

# These are the 4 binaries that make up the 4 deployments for rukpak.
binaries = ["helm", "core", "webhooks", "crdvalidator"]

# This is how we build each binary
build_cmd = '''
mkdir -p .tiltbuild/bin
CGO_ENABLED=0 GOOS=linux go build -o .tiltbuild/bin/{binary} ./cmd/{binary}
'''

# All of our binaries and images are built the same way, so we can iterate and substitute the binary name where needed.
for binary in binaries:
    # Treat the main binary as a local resource, so we can automatically rebuild it when any of the deps change. This
    # builds it locally, targeting linux, so it can run in a linux container.
    local_resource(
        '{}_binary'.format(binary),
        cmd = build_cmd.format(binary=binary),
        deps = ['api', 'cmd/{}'.format(binary), 'internal', 'pkg', 'go.mod', 'go.sum']
    )

    # Configure our image build. If the file in live_update.sync (.tiltbuild/bin/$binary) changes, Tilt
    # copies it to the running container and restarts it.
    docker_build_with_restart(
        # This has to match an image in the k8s_yaml we call below, so Tilt knows to use this image for our Deployment,
        # instead of the actual image specified in the yaml.
        ref = 'quay.io/operator-framework/rukpak:{}'.format(binary),
        # This is the `docker build` context, and because we're only copying in the binary we've already had Tilt build
        # locally, we set the context to the directory containing the binary.
        context = '.tiltbuild/bin',
        # We use a slimmed-down Dockerfile that only has $binary in it.
        dockerfile_contents = '''
FROM gcr.io/distroless/static:debug
EXPOSE 8080
WORKDIR /
COPY {} /.
        '''.format(binary),
        # The set of files Tilt should include in the build. In this case, it's just the binary we built above.
        only = binary,
        # If .tiltbuild/bin/$binary changes, Tilt will copy it into the running container and restart the process.
        live_update = [
            sync('.tiltbuild/bin/{}'.format(binary), '/{}'.format(binary)),
        ],
        # The command to run in the container.
        entrypoint = "/{}".format(binary),
    )

# Tell Tilt what to deploy by running kustomize and then doing some manipulation to make things work for Tilt.
objects = decode_yaml_stream(kustomize('manifests/overlays/cert-manager'))
for o in objects:
    if o['kind'] != 'Deployment':
        # We only need to modify Deployments, so we can skip this
        continue

    # For Tilt's live_update functionality to work, we have to run the container as root. Otherwise, Tilt won't
    # be able to untar on top of /$binary in the container's file system (this is how live update
    # works). If the container definition says runAsNonRoot=true, we flip it to false.
    if 'securityContext' in o['spec']['template']['spec']:
        o['spec']['template']['spec']['securityContext']['runAsNonRoot'] = False

    # The rukpak Deployment manifests all use the same image, quay.io/operator-framework/rukpak:devel. Tilt needs each
    # Deployment's image to be unique. We replace the :devel tag with what is effectively :$binary, e.g. :helm.
    for c in o['spec']['template']['spec']['containers']:
        if c['name'] == 'kube-rbac-proxy':
            continue
        # The container's command is the same as the binary name with a leading / in front. Replace the / with a : to
        # turn it into a valid image tag.
        command = c['command'][0].replace('/',':')
        # Update the image so instead of :devel it's :$binary
        c['image'] = c['image'].replace(':devel', command)

# Now apply all the yaml
k8s_yaml(encode_yaml_stream(objects))
