# Examples

Two Packer templates that bake a **private** Evolution image with this plugin.
Both read credentials and placement from `CLOUDRU_EVOLUTION_*` via Packer
`variable` defaults (`env()` is only valid there, not inside `source { }`).
Do not put keys in HCL.

Install the plugin first (`packer init` cannot download it until there is a
GitHub release). From the repository root:

```bash
make test
make dev
```

`make dev` installs version `0.1.0`. Templates require `>= 0.1.0`.

## Environment

```bash
export CLOUDRU_EVOLUTION_KEY_ID=...
export CLOUDRU_EVOLUTION_KEY_SECRET=...
export CLOUDRU_EVOLUTION_PROJECT_ID=...
export CLOUDRU_EVOLUTION_ZONE_ID=...            # availability zone UUID
export CLOUDRU_EVOLUTION_SUBNET_ID=...
export CLOUDRU_EVOLUTION_FLAVOR_ID=...
export CLOUDRU_EVOLUTION_SOURCE_IMAGE_ID=...    # public Ubuntu-24.04 works
```

Optional:

```bash
# export CLOUDRU_EVOLUTION_SECURITY_GROUP_IDS=...  # comma-separated, if default SG blocks SSH
# export CLOUDRU_EVOLUTION_IMAGE_NAME=...
```

`zone_id` is the availability zone UUID, not the display name (`ru.AZ-2`).
Copy [cloudru.pkrvars.hcl.example](cloudru.pkrvars.hcl.example) if you prefer
`-var-file` for placement IDs. Keep real `*.pkrvars.hcl` out of git.

## What each template bakes

| Directory | Image | Guest work |
| --- | --- | --- |
| [`minimal`](minimal) | Private clone of the source image | Wait for cloud-init |
| [`ytsaurus`](ytsaurus) | Ubuntu golden for YTsaurus / tractoai nodes | Docker, sysctl, no swap |

`minimal` is the smoke test: prove the plugin, get a private image that later
accepts `cloud_init`. `ytsaurus` is the node golden. It does not install
`ytserver-all` — see [ytsaurus/README.md](ytsaurus/README.md).

## Run

From the repository root:

```bash
packer build examples/minimal
packer build examples/ytsaurus
```

`image_name` must be unique in the project. The defaults are
`example-golden` and `ytsaurus-ubuntu-24-04`. Override when that name already
exists:

```bash
packer build -var image_name=example-golden-dev examples/minimal
packer build -var image_name=ytsaurus-ubuntu-24-04-dev examples/ytsaurus
```

Or set `CLOUDRU_EVOLUTION_IMAGE_NAME` (the templates fall back to it only
where the variable default is empty; `ytsaurus` hard-defaults the name, so
use `-var`).

Org quota `compute.image.custome` (custom images) is small. A 422
`organization_quota_exceeded` means delete an unused private image and retry.
The builder VM, floating IP, and disk are still torn down.
