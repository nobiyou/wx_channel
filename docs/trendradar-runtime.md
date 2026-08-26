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

## Page bridge readiness

The ltaoo HTTP listener becoming available does not prove that a PC WeChat
Channels page has been injected. The page bridge is established only after a
Channels page is loaded through the running proxy and its injected client
registers the required API methods. For a manual run:

1. Start the collection run and wait until the runtime has opened its API
   listener (2022 by default).
2. Refresh the already-open Channels video once, or leave and re-enter it.
3. Keep that page open while the batch performs the shared, bounded profile
   readiness probe. The first configured link proves the bridge; the remaining
   links are collected through the same page client and do not need to be
   opened individually.

The shared profile readiness window is 60 seconds for the whole batch, not 60
seconds per link. TrendRadar runs may opt in to one startup page refresh through
`-AutoRefreshWechatPage`. The helper only sends F5 to an unambiguous visible
WeChat window; when both the PC host and a titled `WeChatAppEx` page window are
visible, it prefers the single visible `WeChatAppEx` page process. The helper
restores and foregrounds the selected window before sending one F5; it never
types a URL, searches, reads page data, or refreshes during collection.
If no unambiguous window exists, it fails with a closed error code and the
runtime performs normal cleanup. After the one refresh, the batch waits for the
same shared profile readiness window. If the bridge is not observed within that
window, the batch records `profile_not_ready` and does not start comment
pagination. This is a page-context failure, not a pagination failure.

Runs may additionally opt in to `-AutoOpenFirstShareUrl`. This mode restores
and foregrounds the PC WeChat host, checks the left navigation rail with a
local butterfly-icon template plus normalized geometry, and performs one
controlled click only after both checks pass. It then waits for a titled
`WeChatAppEx` page, writes the first configured `content_urls` entry through
the page's `OmniboxViewViews` value pattern, and submits Enter. The remaining
configured links are still handled by the same batch/profile bridge; they are
not opened one by one. The mode is mutually exclusive with
`-AutoRefreshWechatPage`, and all navigation failures are closed errors that
enter normal cleanup. No OCR, clipboard, browser automation, or model API is
used. Window activation is DPI-aware and uses a temporary foreground-input
attachment when Windows would otherwise reject `SetForegroundWindow`; it does
not set the window permanently topmost.

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
