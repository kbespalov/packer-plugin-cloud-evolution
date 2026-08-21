# Troubleshooting

## `packer init` cannot download the plugin

Packer fetches a published GitHub Release for tag `vX.Y.Z` with:

- `packer-plugin-cloud-evolution_vX.Y.Z_SHA256SUMS`
- `packer-plugin-cloud-evolution_vX.Y.Z_x5.0_{os}_{arch}.zip`

A draft release, a source-only GitHub “Release”, or a missing `SHA256SUMS` file will fail. Use `make dev` to install a local binary.

In `required_plugins`, the source must be `github.com/kbespalov/cloud-evolution` (not `…/packer-plugin-cloud-evolution`). Packer inserts `packer-plugin-` itself.

`>= 0.1.0-dev` is invalid. Packer rejects prerelease constraints.

## `There is no function named "env"`

Packer allows `env()` only as a `variable` `default`, not inside `source { }`. See [examples/minimal/build.pkr.hcl](../examples/minimal/build.pkr.hcl).

## SSH never connects

- `zone` must be the availability zone **UUID**, not `ru.AZ-2`.
- Public Ubuntu-24.04 injects `public_key` only when `linux_login` is `ubuntu` (the default).
- If the default NIC security group blocks port 22, set `CLOUDRU_EVOLUTION_SECURITY_GROUP_IDS`.
- First boot of public Ubuntu is slow: guest cloud-init runs `package_update` against `mirror.yandex.ru`, which Evolution guests often cannot reach. Packer waits on SSH, then `cloud-init status --wait`. A bake of 15–25 minutes is normal.

## `organization_quota_exceeded` / `compute.image.custome`

The org custom-image quota is small. Delete an unused private image and retry. The builder VM, floating IP, and disk are still torn down.

## `image_name` already exists

Names are unique per project. Override:

```bash
packer build -var image_name=example-golden-dev examples/minimal
```

## First boot of the baked image still uses `mirror.yandex.ru`

`examples/minimal` does not rewrite apt sources. Use [examples/ytsaurus](../examples/ytsaurus) (it points Ubuntu 24.04 `ubuntu.sources` at `archive.ubuntu.com`) or add that step to your provisioner. You cannot inject `cloud_init` on the **public** source image; you can on the **private** result.

## Cleanup said “delete it manually”

Teardown needs DNS/API reachability from the machine running Packer. If `compute.api.cloud.ru` does not resolve, leftover VMs, floating IPs, and disks stay in the project.
