# Compatibility and versioning

Straw is pre-1.0. Patch releases preserve documented behavior. Minor releases may make incompatible protocol or
configuration changes only when release notes identify the change, migration, and rollback path.

## Supported matrix

| Component | Supported version | Compatibility rule |
| --- | --- | --- |
| Straw Control/Egress/CLI | same release | Upgrade Egress, then Control, then CLI; mixed minors are unsupported unless release notes say otherwise |
| worker protocol and Go binding | `v0.3.0` | Exact tag in `go.mod`; negotiation rejects unsupported revisions |
| Go SDK | `v0.1.0` | Exact tag in `go.mod` |
| Python SDK and binding | `v0.1.0` / `v0.3.0` | Exact immutable Git tags in `uv.lock` |
| container images | release tag or digest | Never depend on a moving tag for production rollback |
| Go / Python / Node | 1.26.5 / 3.13 / 20+ | Development and CI toolchains |

## Fingerprint profile catalogue

The built-in fingerprint contract revision is `tls-client-v1.15.1-http1-http2`. Straw adapts the complete set of 79
profile definitions from `tls-client` v1.15.1, but does not depend on either `tls-client` or `fhttp` at runtime. The
contract covers the TLS ClientHello and the profile's HTTP/2 settings order, connection flow window, pseudo-header
order, stream priority, and priority frames. HTTP/3 fields are deliberately ignored.

Names are exact and case-sensitive:

```text
brave_146 brave_146_PSK
chrome_103 chrome_104 chrome_105 chrome_106 chrome_107 chrome_108 chrome_109 chrome_110 chrome_111 chrome_112
chrome_116_PSK chrome_116_PSK_PQ chrome_117 chrome_120 chrome_124 chrome_130_PSK chrome_131 chrome_131_PSK
chrome_133 chrome_133_PSK chrome_144 chrome_144_PSK chrome_146 chrome_146_PSK
cloudscraper confirmed_android confirmed_ios
firefox_102 firefox_104 firefox_105 firefox_106 firefox_108 firefox_110 firefox_117 firefox_120 firefox_123
firefox_132 firefox_133 firefox_135 firefox_146_PSK firefox_147 firefox_147_PSK firefox_148
mesh_android mesh_android_1 mesh_android_2 mesh_ios mesh_ios_1 mesh_ios_2
mms_ios mms_ios_1 mms_ios_2 mms_ios_3
nike_android_mobile nike_ios_mobile
okhttp4_android_7 okhttp4_android_8 okhttp4_android_9 okhttp4_android_10 okhttp4_android_11 okhttp4_android_12
okhttp4_android_13
opera_89 opera_90 opera_91
safari_15_6_1 safari_16_0 safari_ios_15_5 safari_ios_15_6 safari_ios_16_0 safari_ios_17_0 safari_ios_18_0
safari_ios_18_5 safari_ios_26_0 safari_ipad_15_6
zalando_android_mobile zalando_ios_mobile
```

Profiles ending in `_PSK` (including `_PSK_PQ`) maintain an isolated bounded TLS session cache per profile and
executor. The first connection omits an empty pre-shared-key extension; a later connection may offer PSK only after
the destination has supplied a valid session ticket. This avoids cross-profile session leakage while preserving
resumption behavior. The catalogue does not provide browser headers, cookies, JavaScript execution, or browser state.

The adapted source provenance and upstream BSD-4-Clause text are shipped in
[`THIRD_PARTY_NOTICES.md`](https://github.com/beremaran/straw-oss/blob/main/THIRD_PARTY_NOTICES.md).

REST paths and stable error codes are additive within a minor line. JSON clients must ignore unknown fields. Removing
or changing a field, route, error meaning, config default, metric, or CLI output contract requires a minor release,
an `Unreleased` changelog entry, migration guidance, and a deprecation period where practical. Runtime snapshots are
validated as a whole and use `config_version`; protobuf/NATS changes require negotiation fixtures and coordinated
binding tags.

Before 1.0, deprecation normally lasts one minor release. Security fixes may remove unsafe behavior immediately.
Rollback restores the previous binaries/images and, for stateful profiles, the backup taken before upgrade.
