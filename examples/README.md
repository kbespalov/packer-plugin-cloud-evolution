# Examples

Install the plugin (`packer init examples/minimal`, or `make dev` from source)
and set `CLOUDRU_EVOLUTION_*` as in the [README quick start](../README.md#quick-start).
Do not put keys in HCL. `env()` is valid only as a variable `default`.

| Directory | What it bakes |
| --- | --- |
| [`minimal`](minimal) | Private clone of the source image |
| [`ytsaurus`](ytsaurus) | Ubuntu golden: Docker, sysctl, no swap |

```bash
packer build examples/minimal
packer build -var image_name=example-golden-dev examples/minimal
packer build examples/ytsaurus
```

`image_name` must be unique. `ytsaurus` defaults to `ytsaurus-ubuntu-24-04`;
override with `-var`. Placement IDs can also go in a gitignored `*.pkrvars.hcl`
(see [cloudru.pkrvars.hcl.example](cloudru.pkrvars.hcl.example)).

Quota and SSH issues: [docs/troubleshooting.md](../docs/troubleshooting.md).
