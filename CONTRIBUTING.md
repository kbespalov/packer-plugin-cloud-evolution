# Contributing

1. `make test` must stay green. Do not skip the race detector.
2. Cloud behavior belongs in `Client` / `Driver`. Steps must not call HTTP.
3. HTTP: never log `Authorization` or key material. Do not retry POST on
   timeout/5xx (Evolution has no idempotency key). Honor `Retry-After`.
   Lists use `offset`/`limit`, never `page_size`.
4. Live API quirks go in [docs/api-notes.md](docs/api-notes.md) and in comments
   next to the request. Fail fast on `error`/`failed` resource states; do not
   wait out `state_timeout`. A 404 on Find/Get is "missing"; any other error
   is fatal (never "name is free").
5. Never commit `CLOUDRU_EVOLUTION_KEY_*`, tokens, or project dumps.
6. Acceptance tests are opt-in (`PACKER_ACC=1`) and must clean VMs, FIPs, and
   leftover disks even on failure.
