# Changelog

## 0.2.0 (2026-08-29)

- Fix `Retry-After` handling: a 429/503 carrying the header was wrapped in an
  error that no longer matched `AsAPIError` and was never retried.
- Recover `CreateInstance` after an ambiguous POST `/vms` failure (5xx or
  transport timeout): the VM name is the idempotency key, so poll the list
  briefly and adopt the VM instead of failing the build. Scoped to the `/vms`
  request itself; an IAM failure inside the same call is never "recovered".
- Track resources for cleanup even on failure: a floating IP allocated
  without an address, and the boot disk when the instance wait fails.
- Retry transient transport errors (dial, TLS, timeout) on IAM token fetch.
- `ssh_host` now overrides the discovered address (bastion / DNAT setups).
- Reset floating-IP fields when the primary NIC replaces an earlier one in
  the instance view.
- Tests are race-clean: atomic request counters, no `t.Fatal` inside HTTP
  handlers, `go test -race` passes.

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
