# packer-plugin-cloud-evolution

[![Go](https://img.shields.io/badge/go-1.22+-00ADD8)](https://go.dev)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Packer builder for **Cloud.ru Evolution Compute**. It creates a **private**
image from an existing catalog image: VM → SSH provision → stop → detach boot
disk → `POST /api/v1/images`.

This is a community plugin, not an official Cloud.ru product. Cloud.ru's own
Packer write-up uses QEMU + a RAW upload; CCE `turbo-node` is Huawei/Advanced.

Layout follows [hashicorp/packer-plugin-yandex](https://github.com/hashicorp/packer-plugin-yandex):
`Driver`, Packer `multistep`, communicator SSH, teardown.

## Install

Repo name must stay `packer-plugin-*` for `packer init`. After a GitHub
release with GoReleaser assets:

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

Locally:

```bash
make test
make dev
```

## Minimal template

```hcl
source "cloud-evolution" "ubuntu" {
  key_id          = env("EVOLUTION_KEY_ID")
  key_secret      = env("EVOLUTION_KEY_SECRET")
  project_id      = env("EVOLUTION_PROJECT_ID")
  zone            = var.zone          # availability zone UUID
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

See [example/build.pkr.hcl](example/build.pkr.hcl) and
[docs/builders/cloud-evolution.mdx](docs/builders/cloud-evolution.mdx).

Auth: `EVOLUTION_KEY_ID`, `EVOLUTION_KEY_SECRET`, `EVOLUTION_PROJECT_ID`.
`EVOLUTION_TOKEN` works but expires in about an hour. The SA needs create
rights on VMs, disks, floating IPs, and images (`platform.project.admin`).

## What the API actually does

| Trap | Plugin |
| --- | --- |
| `POST /api/v1.1/vms` is a **JSON array** | always `[one]` |
| `new_floating_ip` on create NIC is `extra_forbidden` | FIP after the NIC exists |
| Public Ubuntu rejects `cloud_init` | never sent on the builder VM |
| Public Ubuntu injects keys only for `name=ubuntu` | default `linux_login` |
| `public_key` must be a raw `ssh-ed25519 AAAA…` | communicator key pair |
| Image-from-disk needs disk `available` | stop → detach → wait → POST image |
| `availability_zones[].enabled` is `extra_forbidden` | omitted |
| List uses `offset`/`limit` | never `page_size` |

The baked image is `type=private`, so a later VM create may send `cloud_init`
(base64).

`image_name` must match `^[a-zA-Z][a-zA-Z0-9.\-_]*$` and be unique in the
project. Evolution has no client-token header.

## Development

```bash
make test          # gofmt, vet, race
make build
PACKER_ACC=1 go test ./builder/evolution -run TestAcc -timeout 120m
```

Do not commit key IDs or secrets.

## License

Apache-2.0. Cloud.ru, Evolution, and Packer are trademarks of their owners.
