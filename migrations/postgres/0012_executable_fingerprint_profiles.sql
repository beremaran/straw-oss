-- Preserve the original seeded rows as audit history while correcting the
-- executable profile catalog. The compatibility alias is resolved before
-- catalog lookup and is never advertised as a named browser profile.

INSERT INTO fingerprint_profiles (tenant_id, name, scope_type, supported_by_worker, enabled, profile_jsonb)
SELECT NULL, 'chrome_120', 'global', true, true, jsonb_build_object(
    'executor_type', 'egress',
    'profile_ref', 'tls-client/v1.15.1:profiles.Chrome_120',
    'contract_revision', 'chrome_120_v1_15_1'
)
WHERE NOT EXISTS (
    SELECT 1 FROM fingerprint_profiles WHERE scope_type = 'global' AND name = 'chrome_120'
);

UPDATE fingerprint_profiles
SET supported_by_worker = true,
    enabled = true,
    profile_jsonb = profile_jsonb || jsonb_build_object(
        'executor_type', 'egress',
        'profile_ref', 'tls-client/v1.15.1:profiles.Chrome_120',
        'contract_revision', 'chrome_120_v1_15_1'
    ),
    updated_at = now()
WHERE scope_type = 'global' AND name = 'chrome_120';

UPDATE fingerprint_profiles
SET supported_by_worker = false,
    enabled = false,
    updated_at = now()
WHERE scope_type = 'global' AND name IN ('default', 'firefox_121', 'safari_17');
