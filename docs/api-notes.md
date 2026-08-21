# Evolution Compute API notes

Live facts the plugin encodes. They are not Packer bugs.

| What the API does | What the plugin does |
| --- | --- |
| `POST /api/v1.1/vms` is a batch handler | always sends `[one]` |
| `new_floating_ip` on create NIC is `extra_forbidden` | allocates a floating IP after the NIC exists |
| Public Ubuntu rejects `cloud_init` | never sent on the builder VM |
| Public Ubuntu injects keys only for `name=ubuntu` | default `linux_login` |
| `public_key` must be a raw `ssh-ed25519 AAAA…` | communicator key pair |
| Image-from-disk needs disk `available` | stop → detach → wait → POST image |
| `availability_zones[].enabled` is `extra_forbidden` | omitted |
| List pagination is `offset`/`limit` | never `page_size` |
| Immediate `DELETE` VM after FIP delete is `422` | retry until the NIC drops the address |

The baked image is `type=private`. A later VM create from it may send `cloud_init` (base64). The builder VM from a public image must not.

`image_name` must match `^[a-zA-Z][a-zA-Z0-9.\-_]*$` and be unique in the project. Evolution has no client-token header, so a name clash is a hard error, not an overwrite.

## Timeouts

- `state_timeout` (default 30m) is the deadline for waiting on VM, disk, and image.
- `poll_interval` cannot sleep past that deadline.
- HTTP calls are 30s each. Teardown is capped at 10m (or `state_timeout` if shorter).
- `ssh_timeout` is forced if unset (Packer otherwise waits forever when handshake attempts are set).
- A VM, disk, or image that enters `error`/`failed` fails the build immediately.
- An image that is created but never becomes ready is deleted on teardown so it does not consume custom-image quota.

HCL field reference: [builders/cloud-evolution.mdx](builders/cloud-evolution.mdx).
