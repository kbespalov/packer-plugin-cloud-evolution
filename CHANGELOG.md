# Changelog

## 0.1.0 (2026-08-22)

- Initial `cloud-evolution` builder: IAM key/token, VM `[one]`, floating IP after NIC,
  stop → detach → private image from disk.
- After image create, delete FIP first then retry VM delete: Evolution returns
  422 `floating_ip_can_not_be_detached_from_vm_in_current_state` if the VM is
  removed immediately after the FIP.
- Fail fast when a VM, disk, or image enters `error`/`failed`. Treat GET 404
  right after create as eventual consistency; a later 404 is "disappeared".
- Do not treat a failed `FindImage` as "name is free".
- Delete a catalog entry that never becomes ready so it does not consume
  custom-image quota. Bound teardown to 10m (or `state_timeout` if smaller).
- Idempotent stop/detach; retry disk delete while the volume is still `in_use`.
- Every wait has a deadline: poll caps sleep to the remaining time (a huge
  `poll_interval` cannot sleep past `state_timeout`), HTTP clients without
  `Timeout` get 30s, each request gets a deadline if the caller passed
  `context.Background()`, FIP address wait uses `poll`, teardown and
  `Artifact.Destroy` use bounded contexts, and `ssh_timeout` is forced when
  the communicator would otherwise wait forever.
- Validate disk type, disk size vs source `min_disk`, zone UUID shape, and
  reject SSH private-key material in `public_key`.
- Environment variables are `CLOUDRU_EVOLUTION_*` only. Bare `EVOLUTION_*` is
  no longer read.
- Unit tests for config, HTTP client, bake steps, and artifact.
