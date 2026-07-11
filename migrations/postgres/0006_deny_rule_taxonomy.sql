-- Widen deny_rules to the docs/planning/26 P0 taxonomy (docs/implementation-history.md#p0-43):
-- rule_type in ('cidr','host','host_suffix','cname_suffix','metadata_ip',
-- 'private_range'), action in ('deny','allow_override'), plus a reason field.
-- Existing rows are remapped so their effective behavior is unchanged:
-- 'ip' -> 'cidr' (normalized_cidr populated as a /32 from normalized_ip),
-- 'cname' -> 'cname_suffix', 'allow' -> 'allow_override'.
ALTER TABLE deny_rules ADD COLUMN IF NOT EXISTS reason text;

ALTER TABLE deny_rules DROP CONSTRAINT IF EXISTS deny_rules_rule_type_check;
ALTER TABLE deny_rules DROP CONSTRAINT IF EXISTS deny_rules_action_check;

UPDATE deny_rules
   SET normalized_cidr = (host(normalized_ip) || '/' || CASE WHEN family(normalized_ip) = 6 THEN '128' ELSE '32' END)::cidr
 WHERE rule_type = 'ip' AND normalized_ip IS NOT NULL AND normalized_cidr IS NULL;

UPDATE deny_rules SET rule_type = 'cidr' WHERE rule_type = 'ip';
UPDATE deny_rules SET rule_type = 'cname_suffix' WHERE rule_type = 'cname';
UPDATE deny_rules SET action = 'allow_override' WHERE action = 'allow';

ALTER TABLE deny_rules ADD CONSTRAINT deny_rules_rule_type_check
  CHECK (rule_type IN ('cidr', 'host', 'host_suffix', 'cname_suffix', 'metadata_ip', 'private_range'));
ALTER TABLE deny_rules ADD CONSTRAINT deny_rules_action_check
  CHECK (action IN ('deny', 'allow_override'));
