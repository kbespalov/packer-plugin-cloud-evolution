# Contributing

1. `make test` must stay green. Do not skip the race detector.
2. Cloud behavior belongs in `Client` / `Driver`. Steps must not call HTTP.
3. Live API quirks go in the README table and in comments next to the request.
4. Never commit `EVOLUTION_KEY_*`, tokens, or project dumps.
5. Acceptance tests are opt-in (`PACKER_ACC=1`) and must clean VMs, FIPs, and
   leftover disks even on failure.
