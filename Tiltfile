if not os.path.exists('../tilt-support'):
    fail('Please clone https://github.com/operator-framework/tilt-support to ../tilt-support')

load('../tilt-support/Tiltfile', 'deploy_repo')

repo = {
    'image': 'quay.io/operator-framework/rukpak',
    'yaml': 'manifests/overlays/cert-manager',
    'binaries': {
        'core': 'core',
        'crdvalidator': 'crd-validation-webhook',
        'helm': 'helm-provisioner',
        'webhooks': 'rukpak-webhooks',
    },
    'starting_debug_port': 10000,
}

deploy_repo('rukpak', repo)
