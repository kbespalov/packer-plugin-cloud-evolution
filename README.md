# packer-plugin-cloud-evolution

Build private [Cloud.ru](https://cloud.ru) Evolution Compute images with [HashiCorp Packer](https://www.packer.io/).

Community project, not published by Cloud.ru or HashiCorp. Installing from source needs Packer and Go 1.22+.

## Quick start

`packer init` cannot download this plugin until a GitHub Release exists. Install the local binary:

```bash
git clone https://github.com/kbespalov/packer-plugin-cloud-evolution.git
cd packer-plugin-cloud-evolution
make dev
```

```bash
export CLOUDRU_EVOLUTION_KEY_ID=...
export CLOUDRU_EVOLUTION_KEY_SECRET=...
export CLOUDRU_EVOLUTION_PROJECT_ID=...
export CLOUDRU_EVOLUTION_ZONE_ID=...            # availability zone UUID, not ru.AZ-2
export CLOUDRU_EVOLUTION_SUBNET_ID=...
export CLOUDRU_EVOLUTION_FLAVOR_ID=...
export CLOUDRU_EVOLUTION_SOURCE_IMAGE_ID=...    # public Ubuntu-24.04 works
```

The service account needs create/delete on VMs, disks, floating IPs, and images. A bake creates a VM, floating IP, and disk for about 15–25 minutes; public Ubuntu spends most of that in guest cloud-init.

```bash
packer build examples/minimal
```

That creates a private image named `example-golden`. Packer prints the image id on success, then deletes the builder VM, floating IP, and boot disk.

`image_name` must be unique in the project. If `example-golden` already exists:

```bash
packer build -var image_name=example-golden-dev examples/minimal
```

Put secrets in the environment, not in HCL. `*.pkrvars.hcl` is gitignored.

## Minimal example

Full file: [examples/minimal/build.pkr.hcl](examples/minimal/build.pkr.hcl).

`env()` is valid only as a variable `default`, not inside `source { }`.

```hcl
packer {
  required_plugins {
    cloud-evolution = {
      version = ">= 0.1.0"
      source  = "github.com/kbespalov/cloud-evolution"
    }
  }
}

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
    inline = ["sudo cloud-init status --wait || true"]
  }
}
```

The `source` string is `github.com/kbespalov/cloud-evolution`. Do not add `packer-plugin-`; Packer inserts it.

## Configuration

Empty HCL fields are filled from `CLOUDRU_EVOLUTION_*`.

| HCL | Environment | Required |
| --- | --- | --- |
| `key_id` / `key_secret` | `CLOUDRU_EVOLUTION_KEY_ID`, `CLOUDRU_EVOLUTION_KEY_SECRET` | yes (or `token`) |
| `project_id` | `CLOUDRU_EVOLUTION_PROJECT_ID` | yes |
| `zone` | `CLOUDRU_EVOLUTION_ZONE_ID` | yes (UUID) |
| `subnet_id` | `CLOUDRU_EVOLUTION_SUBNET_ID` | yes |
| `flavor_id` | `CLOUDRU_EVOLUTION_FLAVOR_ID` | yes |
| `source_image_id` | `CLOUDRU_EVOLUTION_SOURCE_IMAGE_ID` | yes |
| `image_name` | `CLOUDRU_EVOLUTION_IMAGE_NAME` | yes |
| `security_group_ids` | `CLOUDRU_EVOLUTION_SECURITY_GROUP_IDS` | if default SG blocks SSH |

Optional HCL: `disk_size_gb` (default 10), `linux_login` (default `ubuntu`), `use_floating_ip` (default true), `ssh_timeout`, `state_timeout` (default 30m).

Full field list: [docs/builders/cloud-evolution.mdx](docs/builders/cloud-evolution.mdx).

## How it works

Packer → temporary VM + floating IP → provision over SSH → stop and detach the boot disk → private catalog image → tear down the VM.

The result is a **private** image. Later VMs created from it may use `cloud_init`; the builder VM from a public image cannot.

Org custom-image quota is small. `organization_quota_exceeded` means delete an unused private image and retry.

## Examples

| Path | What it bakes |
| --- | --- |
| [examples/minimal](examples/minimal) | Private clone of the source image |
| [examples/ytsaurus](examples/ytsaurus) | Ubuntu golden: Docker, sysctl, no swap |

## Documentation

- [Builder reference](docs/builders/cloud-evolution.mdx)
- [Troubleshooting](docs/troubleshooting.md) — SSH, quota, `packer init`, apt mirrors
- [API notes](docs/api-notes.md) — Evolution Compute quirks the plugin already handles

## Development

```bash
make test
make dev
```

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache-2.0. Cloud.ru, Evolution, and Packer are trademarks of their owners.
