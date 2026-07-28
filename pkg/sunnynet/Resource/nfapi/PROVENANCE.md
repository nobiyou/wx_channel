# SunnyNet NFAPI binary provenance

The clean `nobiyou/wx_channel` commit `11d49cee1da9032230dc5c0eece79bcf03da3e82`
references two `nfapi.dll` files with `go:embed`, but does not track them. They are
required to compile the vendored SunnyNet Windows backend.

These two files were retrieved without execution from the upstream SunnyNet repository:

- Repository: `https://github.com/qtgolang/SunnyNet`
- Pinned commit: `505b77b76da5872e8466a327dc6e574c42f7700c`
- License: MIT; the corresponding license is retained at `pkg/sunnynet/LICENSE`

| Path | Bytes | Git blob SHA-1 | SHA-256 | Authenticode |
| --- | ---: | --- | --- | --- |
| `dll/win32/nfapi.dll` | 251904 | `4babd31cefabef6cb74c5d3c004ac8c8b5a24280` | `B6AD927CE7A5281F1B71BE347B6EE4B920A8EF90F104C6A5CC56082FBA0C3528` | Not signed |
| `dll/x64/nfapi.dll` | 292864 | `e68fd1eb32a40875df3f56ef33047cfad4e33d67` | `1D6F3487D3AA707B978E1A81F8E98250D334120B856B89780408EB98DBBD0910` | Not signed |

The four `netfilter2.sys` files already tracked by `wx_channel` have Git blob IDs
identical to the same pinned SunnyNet commit. Windows reports valid Authenticode
signatures from Microsoft Windows Hardware Compatibility Publisher:

| Path | SHA-256 |
| --- | --- |
| `sys/tdi/amd64/netfilter2.sys` | `8B24C85B5325E2CEFF531651A74274409518FB2FF11EF258D2675377B0C9B5A2` |
| `sys/tdi/i386/netfilter2.sys` | `3ED01B98D86E0E85F71A8EC2856E7FA170DB6B34D0955A508F116B5EC1172B16` |
| `sys/wfp/amd64/netfilter2.sys` | `D5E68A1C65280CB8497E7CB95BD0013D79CB728C30FE7821315915946B88251C` |
| `sys/wfp/i386/netfilter2.sys` | `01A7D8088C631988C03430795E80EDF07123047294EE4A6FC260EB29B7515346` |

The DLLs are unsigned third-party native dependencies. Their exact hashes are enforced
by `internal/pocaudit`; build and run procedures must fail closed on any mismatch. They
must not be loaded outside the approved disposable Windows VM or before the real-run
checkpoint.
