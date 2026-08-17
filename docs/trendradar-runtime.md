# TrendRadar WeChat Channels runtime contract

`scripts/Invoke-LtaooTrendRadarBatch.ps1` is the Windows-only provider entry
point for configured WeChat Channels share links. TrendRadar calls it with
literal absolute paths to a private run directory, `request.json`, a one-shot
`grant.json`, the reviewed ltaoo and batch executables, and one authorized
proxy-router backend. Runtime protocol v2 currently accepts only
`-RouterKind mihomo` with `-RouterExePath`, `-RouterConfigPath`, and the
authorized capability fingerprint.

The entry point is intentionally noninteractive. A human authorizes this mode
in TrendRadar first; each manual or scheduled run then receives a five-minute,
owner-only grant bound to the Windows SID, run and request hashes, canonical
runtime paths, ltaoo/batch/router hashes, router kind, config-path hash, and
capability fingerprint. The provider deletes that grant before changing the
certificate store or proxy router. There is no
`Read-Host`, elevation, or `LocalMachine` certificate operation.

## Machine requirements

- Windows PowerShell 5.1 or PowerShell 7.
- A reviewed `wx_video_download.exe` and `wx_channel_ltaoo_batch.exe`.
- A running Mihomo-compatible core whose active YAML has `proxies:`, `rules:`, and a
  literal loopback `external-controller`. If `secret` is configured, it is read
  only in memory for the local reload request.
- Permission for the current user to add and remove a certificate in
  `Cert:\CurrentUser\Root` and to replace the selected Mihomo YAML.
- API/proxy ports (2022/2023 by default) free before the run.

The proxy client remains running. `MihomoBackend` inserts one marked loopback HTTP proxy and
ordered process rules, validates the candidate with the configured Mihomo core,
atomically replaces the YAML, and reloads the core through its loopback API.
`wx_video_download.exe` remains `DIRECT`; WeChat processes use only the
temporary loopback proxy.

The old `-ClashExePath`/`-ClashConfigPath` pair remains a closed compatibility
adapter for reviewed v1 POC callers. It cannot be mixed with v2 router
arguments. New TrendRadar versions never emit the old pair, and unknown router
kinds fail before the grant is consumed or the machine is modified.

## Lifecycle and recovery

Every run uses a fresh one-day CA and private key beneath the private run root.
The CA is installed only in `CurrentUser`, ltaoo starts hidden with system proxy
and TUN changes disabled, and the verified batch command performs bounded
collection. A stable `runtime-journal.json` is updated after each phase.

Cleanup always attempts to:

1. stop ltaoo only when PID, start time, path, and executable hash all match;
2. remove only the recorded CurrentUser CA thumbprint;
3. restore the byte-for-byte router backup when no external edit occurred, or
   remove only the run's marked blocks when external edits are detected;
4. delete private keys and temporary configuration;
5. verify both runtime ports are no longer listening;
6. write a closed cleanup receipt and finalize any closed collection draft.

If safe restoration cannot be proven, the backup and journal remain and the
published batch is `needs_verification`; already collected valid comments are
not discarded. The next invocation recovers that journal before starting a new
run and finalizes any prior closed draft. Operators must not delete a retained
journal or backup manually before inspecting the machine state.

Exit codes are `0=succeeded`, `2=partial`, `3=needs_verification/runtime_busy`,
`4=failed`, and `64=request_invalid`. Output batches never contain the grant,
request, certificate, private key, router configuration, controller secret,
absolute local paths, cursors, cookies, or raw response bodies.

The older `prepare-ltaoo-probe.ps1` and `cleanup-ltaoo-probe.ps1` remain the
explicitly interactive diagnostic path. They are not used by TrendRadar manual
or scheduled collection.
