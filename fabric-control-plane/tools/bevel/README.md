# Bevel development toolchain

Build and verify from the backend repository:

```powershell
.\scripts\check-bevel.ps1 -Build -BevelCheckout 'C:\path\to\bevel'
```

This creates a Linux development image containing Ansible, its bundled collections, Helm 3 and kubectl. Downloads of Helm and kubectl are checked against publisher SHA256 files. The Python base image and direct Python package versions are pinned. Transitive packages and Debian packages are not fully locked; this is not a qualified release image. The script validates the inspected Bevel commit, tool versions and the Fabric deployment playbook syntax with a read-only checkout. It does not mount a Docker socket, kubeconfig or cloud credentials, execute deployment tasks, or create a Kubernetes cluster. Temporary validation containers remove themselves.

The inspected fork is commit `8824ddc9c461f8d427ab2b140fb56211d89d4ebe`. Its Fabric schema admits 2.5.0 but not the local runtime's 2.5.15. Its orderer templates emit `helm.fluxcd.io/v1` and shared setup installs legacy Flux. These require adapter modernization and end-to-end deployment qualification, not just a version string replacement. Modern Ansible also requires playbook runtime compatibility checks beyond syntax validation.

This image is a tool environment, not the `/opt/sathonate/bin/cloud-runner` executable referenced by candidate exports. Those jobs remain suspended. No cluster context is currently configured; EKS deployment cannot be verified without a real target and locally configured scoped credentials. A local Kubernetes test cluster can qualify Kubernetes behavior, but cannot establish AWS IAM, EBS, DNS or EKS qualification.
