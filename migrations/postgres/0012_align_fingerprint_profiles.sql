-- straw P0 config store: align seeded built-in global fingerprint profiles.
-- Delete unsupported legacy profiles: firefox_121, safari_17
-- Seed new valid profiles: firefox_120, safari_16_0

DELETE FROM fingerprint_profiles 
WHERE scope_type = 'global' AND name IN ('firefox_121', 'safari_17');

INSERT INTO fingerprint_profiles (tenant_id, name, scope_type, supported_by_worker, enabled, profile_jsonb)
SELECT NULL, v.name, 'global', true, true, jsonb_build_object('profile_ref', 'builtin:' || v.name)
FROM (VALUES ('firefox_120'), ('safari_16_0')) AS v(name)
WHERE NOT EXISTS (
    SELECT 1 FROM fingerprint_profiles fp
    WHERE fp.scope_type = 'global' AND fp.name = v.name
);
