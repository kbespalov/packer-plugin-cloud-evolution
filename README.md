# packer-plugin-cloud-evolution

Unofficial [HashiCorp Packer](https://www.packer.io/) **builder plugin** for
[Cloud.ru](https://cloud.ru) **Evolution Compute** (Cloud.ru Evolution,
Cloud.ru IaaS). Bake a **private golden image** — Ubuntu 24.04 or any catalog
image — by creating a VM, provisioning it over SSH, and snapshotting the boot
disk into the Evolution image catalog (`POST /api/v1/images`).

This is a **Packer builder**, not a Terraform provider. It is a community
project and is **not** published or endorsed by Cloud.ru or HashiCorp.

There is no official Packer builder for Evolution. Cloud.ru’s own write-up
uses QEMU plus a RAW upload; CCE `turbo-node` is Huawei / Cloud.ru Advanced,
not Evolution Compute.

Кратко по-русски: неофициальный плагин Packer для Cloud.ru Evolution
Compute. Собирает private/golden image (Ubuntu) через Compute API. Это не
Terraform-провайдер и не продукт Cloud.ru.

The layout follows [hashicorp/packer-plugin-yandex](https://github.com/hashicorp/packer-plugin-yandex):
a `Driver`, Packer `multistep`, the SSH communicator, and teardown of the
builder VM, floating IP, and boot disk.

Requires [Packer](https://developer.hashicorp.com/packer/install) 1.8+ and
Go 1.22+ to build from source.

## What this is (and is not)

| Name people search | This repository |
| --- | --- |
| Packer plugin / builder for Cloud.ru Evolution | Yes — community |
| Official Cloud.ru Packer | No |
| Terraform provider for Cloud.ru / Evolution | No |
| Cloud.ru Advanced, CCE, `turbo-node`, Huawei | No |
| QEMU + RAW image upload | No — this uses the Compute API |

Cloud.ru, Evolution, Packer, and HashiCorp are trademarks of their owners.
This project only uses those names to describe compatibility.

## How a build works

1. Generate a temporary `ed25519` SSH key (or use `ssh_private_key_file`).
2. Create one VM (`POST /api/v1.1/vms` as a one-element JSON array). Public
   Ubuntu does not accept `cloud_init`; the key is injected via
   `image_metadata` (`name=ubuntu`, raw `public_key`).
3. Allocate a floating IP **after** the NIC exists (`new_floating_ip` on create
   is rejected).
4. SSH in and run your provisioners.
5. Stop the VM, detach the boot disk, wait until the disk is `available`.
6. Create a private image from `disk_id`. Wait until the zone state is ready
   (often 8–12 minutes).
7. Delete the floating IP, then the VM, then the disk.

The result is `type=private`. A later VM create from that image **may** send
`cloud_init` (base64). The builder VM from a public image must not.

The builder fails fast if the VM, disk, or image enters `error`/`failed`
instead of polling until `state_timeout`. A catalog entry that never becomes
ready is deleted on teardown so it does not consume the small custom-image
quota. Every wait has a deadline: `state_timeout` (default 30m) is attached
to the poll context so an in-flight GET cannot outlive it, `poll_interval`
cannot sleep past that deadline, HTTP calls are 30s each, SSH defaults to a
finite `ssh_timeout`, and teardown / `packer build -force` image delete are
capped (10m and 2m).

A full bake is typically 15–25 minutes. Most of the wall clock is guest
cloud-init (public Ubuntu runs `package_update` against `mirror.yandex.ru`)
and Evolution’s image-from-disk job. SSH and API calls are the short part.

## Install

The GitHub repository **must** stay named `packer-plugin-cloud-evolution`.
Packer maps the plugin source to that name by inserting `packer-plugin-`:

| In the template | Resolves to |
| --- | --- |
| `source = "github.com/kbespalov/cloud-evolution"` | `github.com/kbespalov/packer-plugin-cloud-evolution` |
| `required_plugins.cloud-evolution` | plugin name |
| `source "cloud-evolution" "…"` | builder type |

Do not put `packer-plugin-` in the `source` string. Packer would look for
`packer-plugin-packer-plugin-cloud-evolution`.

Every template needs this block:

```hcl
packer {
  required_plugins {
    cloud-evolution = {
      version = ">= 0.1.0"
      source  = "github.com/kbespalov/cloud-evolution"
    }
  }
}
```

### From a GitHub release

After a tagged release with GoReleaser assets (zip + `SHA256SUMS`):

```bash
packer init .
packer build .
```

`packer init` downloads the plugin into Packer’s plugin directory. There is
no release yet, so this path does not work until one is published.

### From source (today)

You need [Packer](https://developer.hashicorp.com/packer/install) and Go 1.22+.

```bash
git clone https://github.com/kbespalov/packer-plugin-cloud-evolution.git
cd packer-plugin-cloud-evolution
make test
make dev
```

`make dev` builds `packer-plugin-cloud-evolution` and installs version `0.1.0`
as:

`~/.config/packer/plugins/github.com/kbespalov/cloud-evolution/`

After that, `packer build` uses the local binary. The `required_plugins`
block still belongs in the template: Packer checks that the installed
version satisfies `>= 0.1.0`. It will not try to download a release if the
plugin is already installed.

`>= 0.1.0-dev` is invalid. Packer rejects prerelease constraints.

## Configuration

Put secrets in the environment, not in HCL. The plugin reads
`CLOUDRU_EVOLUTION_*` only.

| Variable | Purpose |
| --- | --- |
| `CLOUDRU_EVOLUTION_KEY_ID` | IAM access key id |
| `CLOUDRU_EVOLUTION_KEY_SECRET` | IAM secret |
| `CLOUDRU_EVOLUTION_TOKEN` | Optional static bearer (expires in ~1 hour) |
| `CLOUDRU_EVOLUTION_PROJECT_ID` | Evolution project UUID |
| `CLOUDRU_EVOLUTION_ZONE_ID` | Availability zone **UUID**, not `ru.AZ-2` |
| `CLOUDRU_EVOLUTION_SUBNET_ID` | VPC subnet UUID |
| `CLOUDRU_EVOLUTION_FLAVOR_ID` | Flavor UUID |
| `CLOUDRU_EVOLUTION_SOURCE_IMAGE_ID` | Source catalog image UUID |
| `CLOUDRU_EVOLUTION_SECURITY_GROUP_IDS` | Optional, comma-separated NIC SGs |
| `CLOUDRU_EVOLUTION_IMAGE_NAME` | Optional default for `image_name` |
| `CLOUDRU_EVOLUTION_COMPUTE_URL` | Override `https://compute.api.cloud.ru` |
| `CLOUDRU_EVOLUTION_IAM_URL` | Override `https://iam.api.cloud.ru` |

The service account needs create/delete on VMs, disks, floating IPs, and
images (`platform.project.admin` or equivalent). Org floating-IP quota is
tight. Custom image quota (`compute.image.custome`) is also small: a 422
`organization_quota_exceeded` means delete an unused private image first.
The builder VM is still torn down.

`image_name` must match `^[a-zA-Z][a-zA-Z0-9.\-_]*$` and be unique in the
project. Evolution has no client-token header, so a name clash is a hard
error, not an overwrite.

HCL field reference: [docs/builders/cloud-evolution.mdx](docs/builders/cloud-evolution.mdx).

Required fields: `project_id`, `zone`, `subnet_id`, `flavor_id`,
`source_image_id`, `image_name`, plus `key_id`+`key_secret` or `token`.

Useful optionals: `linux_login` (default `ubuntu` — public Ubuntu-24.04 only
injects the SSH key for that login), `disk_size_gb` (default 10),
`use_floating_ip` (default true), `skip_create_image`, `state_timeout`
(default 30m), `ssh_username`, `ssh_timeout`.

## Usage

Export credentials and placement, install the plugin, then build a template
in this repository or your own.

```bash
export CLOUDRU_EVOLUTION_KEY_ID=...
export CLOUDRU_EVOLUTION_KEY_SECRET=...
export CLOUDRU_EVOLUTION_PROJECT_ID=...
export CLOUDRU_EVOLUTION_ZONE_ID=...
export CLOUDRU_EVOLUTION_SUBNET_ID=...
export CLOUDRU_EVOLUTION_FLAVOR_ID=...
export CLOUDRU_EVOLUTION_SOURCE_IMAGE_ID=...   # public Ubuntu-24.04 works
```

```bash
make dev
packer build examples/minimal
```

Override the output image name when the default already exists:

```bash
packer build -var image_name=example-golden-dev examples/minimal
```

A minimal template looks like this. Packer allows `env()` **only** as a variable `default`, not inside
`source { }`. Zone and network IDs are variables so you can pass `-var`
or rely on the same environment defaults used in [examples/](examples/).
The builder also fills empty fields from `CLOUDRU_EVOLUTION_*`.

```hcl
packer {
  required_plugins {
    cloud-evolution = {
      version = ">= 0.1.0"
      source  = "github.com/kbespalov/cloud-evolution"
    }
  }
}

# Packer allows env() only as a variable default, not inside source {}.
variable "key_id" {
  type    = string
  default = env("CLOUDRU_EVOLUTION_KEY_ID")
}

variable "key_secret" {
  type      = string
  default   = env("CLOUDRU_EVOLUTION_KEY_SECRET")
  sensitive = true
}

variable "project_id" {
  type    = string
  default = env("CLOUDRU_EVOLUTION_PROJECT_ID")
}

variable "zone_id" {
  type    = string
  default = env("CLOUDRU_EVOLUTION_ZONE_ID")
}

variable "subnet_id" {
  type    = string
  default = env("CLOUDRU_EVOLUTION_SUBNET_ID")
}

variable "flavor_id" {
  type    = string
  default = env("CLOUDRU_EVOLUTION_FLAVOR_ID")
}

variable "source_image_id" {
  type    = string
  default = env("CLOUDRU_EVOLUTION_SOURCE_IMAGE_ID")
}

source "cloud-evolution" "ubuntu" {
  key_id          = var.key_id
  key_secret      = var.key_secret
  project_id      = var.project_id
  zone            = var.zone_id
  subnet_id       = var.subnet_id
  flavor_id       = var.flavor_id
  source_image_id = var.source_image_id
  image_name      = "example-golden"
  ssh_username    = "ubuntu"
}

build {
  sources = ["source.cloud-evolution.ubuntu"]

  provisioner "shell" {
    inline = [
      "sudo cloud-init status --wait || true",
    ]
  }
}
```

On success Packer prints the private image id. Generated data available to
post-processors: `ImageID`, `ImageName`, `ImageType`, `SourceImageID`,
`SourceImageName`.

## Examples

Ready-to-run templates live under [examples/](examples/). Both expect the
`CLOUDRU_EVOLUTION_*` variables above. Do not commit keys into `*.pkrvars.hcl`
(that pattern is gitignored).

### `examples/minimal`

Private clone of the source image with almost no guest work. Use this to
prove the plugin and to get a private image that later accepts `cloud_init`.

```bash
make dev
packer build examples/minimal
```

### `examples/ytsaurus`

Ubuntu golden aimed at YTsaurus / tractoai nodes: Docker, containerd, chrony,
swap off, sysctl (`vm.max_map_count`, file and inotify limits), and
`/etc/ytsaurus/image-release`. It does **not** install `ytserver-all` — there
is no public apt repo from an Evolution guest. Put server packages on a
second pass against this private image, where VM create may send `cloud_init`.

```bash
make dev
packer build examples/ytsaurus
packer build -var image_name=ytsaurus-ubuntu-24-04-dev examples/ytsaurus
```

The provision script waits for the dpkg lock (cloud-init’s apt often still
holds it) and rewrites Ubuntu 24.04 `ubuntu.sources` away from
`mirror.yandex.ru` when guest DNS cannot resolve that mirror.

See [examples/ytsaurus/README.md](examples/ytsaurus/README.md) for the
package list.

## Evolution API notes

These are live Compute facts the plugin encodes. They are not Packer bugs.

| What the API does | What the plugin does |
| --- | --- |
| `POST /api/v1.1/vms` is a batch handler | always sends `[one]` |
| `new_floating_ip` on create NIC is `extra_forbidden` | allocates FIP after the NIC exists |
| Public Ubuntu rejects `cloud_init` | never sent on the builder VM |
| Public Ubuntu injects keys only for `name=ubuntu` | default `linux_login` |
| `public_key` must be a raw `ssh-ed25519 AAAA…` | communicator key pair |
| Image-from-disk needs disk `available` | stop → detach → wait → POST image |
| `availability_zones[].enabled` is `extra_forbidden` | omitted |
| List pagination is `offset`/`limit` | never `page_size` |
| `DELETE` VM right after FIP delete is `422` | retry until the NIC drops the address |

## Development

```bash
make test          # gofmt, vet, race
make build
PACKER_ACC=1 go test ./builder/evolution -run TestAcc -timeout 120m
```

Acceptance tests charge the project. They need the same env as a bake and
must leave no leftover VMs, FIPs, or disks.

Do not commit key ids, secrets, or tokens. More in [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache-2.0. Cloud.ru, Evolution, and Packer are trademarks of their owners.
