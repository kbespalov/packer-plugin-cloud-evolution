# YTsaurus / tractoai node golden

Bakes a **private** Ubuntu image with:

- Docker + containerd
- chrony, curl, jq, python3
- swap off
- sysctl knobs YT jobs usually need (`vm.max_map_count`, file/inotify limits)
- `/etc/ytsaurus/image-release` stamp

It does **not** install `ytserver-all` (no public apt repo from an Evolution
guest). Put the server packages on a second pass against this private image,
where VM create may send `cloud_init`.

```bash
export CLOUDRU_EVOLUTION_KEY_ID=...
export CLOUDRU_EVOLUTION_KEY_SECRET=...
export CLOUDRU_EVOLUTION_PROJECT_ID=...
export CLOUDRU_EVOLUTION_ZONE_ID=...
export CLOUDRU_EVOLUTION_SUBNET_ID=...
export CLOUDRU_EVOLUTION_FLAVOR_ID=...
export CLOUDRU_EVOLUTION_SOURCE_IMAGE_ID=...

make -C ../.. dev
packer build .
```

Override the image name if `ytsaurus-ubuntu-24-04` already exists:

```bash
packer build -var image_name=ytsaurus-ubuntu-24-04-dev .
# or: CLOUDRU_EVOLUTION_IMAGE_NAME=ytsaurus-ubuntu-24-04-dev packer build .
```
