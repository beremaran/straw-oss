// Straw Management UI Shared Validation Rules

export function parseTag(tagStr) {
  const trimmed = (tagStr || '').trim();
  if (!trimmed) throw new Error('Tag cannot be empty');
  
  if (trimmed === '*') {
    throw new Error('Bare wildcard * is not allowed as a routing rule tag');
  }

  let parts = trimmed.split(':');
  if (parts.length < 2) {
    parts = trimmed.split('=');
  }
  
  if (parts.length < 2 || !parts[0].trim()) {
    throw new Error("Tag must follow 'key:value' or 'key=value' format");
  }

  const key = parts[0].trim();
  const value = parts.slice(1).join(':').trim(); // preserve inner colons

  if (!value) {
    // Return key-only tag but indicate warning
    return { key, value: '', warning: 'Tag has empty value' };
  }

  return { key, value };
}

export function validateScope(scopeStr) {
  const trimmed = (scopeStr || '').trim();
  if (!trimmed) throw new Error('Scope cannot be empty');
  if (trimmed === '*') return true; // match all
  
  if (trimmed.includes('*')) {
    // Wildcard prefix key:* or suffix *:value
    const parts = trimmed.split(':');
    if (parts.length === 2) {
      if (parts[0] === '*' && parts[1] !== '*' && parts[1].trim()) return true;
      if (parts[1] === '*' && parts[0] !== '*' && parts[0].trim()) return true;
    }
    throw new Error(`Invalid scope wildcard format '${trimmed}'. Only key:* or *:value wildcards are supported`);
  }

  // Check if standard key:value tag format without wildcards
  try {
    const parsed = parseTag(trimmed);
    return true;
  } catch (err) {
    throw new Error(`Invalid scope format '${trimmed}'. Must be *, prefix wildcard (e.g. target:*), suffix wildcard (e.g. *:us), or exact tag (e.g. region:us)`);
  }
}

export function validateDuration(durationStr) {
  const trimmed = (durationStr || '').trim();
  if (!trimmed) return true; // empty is allowed (optional field)
  
  // Go duration regex: standard numbers with units ns, us, µs, ms, s, m, h
  const goDurationRegex = /^([0-9]+(?:\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$/;
  if (!goDurationRegex.test(trimmed)) {
    throw new Error("Invalid Go duration (e.g. '500ms', '30s', '1m', '2h45m'). Natural language values like '30 seconds' are rejected.");
  }
  return true;
}

export function validateDate(dateStr) {
  const trimmed = (dateStr || '').trim();
  if (!trimmed) return true; // optional
  
  const dateRegex = /^\d{4}-\d{2}-\d{2}$/;
  if (!dateRegex.test(trimmed)) {
    throw new Error("Date must use YYYY-MM-DD format");
  }
  
  const timestamp = Date.parse(trimmed);
  if (isNaN(timestamp)) {
    throw new Error("Invalid calendar date");
  }
  return true;
}

export function validateJsonObject(jsonStr) {
  const trimmed = (jsonStr || '').trim();
  if (!trimmed) throw new Error("JSON configuration is required");
  
  try {
    const parsed = JSON.parse(trimmed);
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
      throw new Error("Configuration must be a valid JSON object");
    }
    return parsed;
  } catch (err) {
    throw new Error(`Invalid JSON syntax: ${err.message}`);
  }
}

export function validatePositiveInteger(val, fieldName) {
  const num = Number(val);
  if (!Number.isInteger(num) || num <= 0) {
    throw new Error(`${fieldName} must be a positive integer`);
  }
  return num;
}

export function validateNonNegativeInteger(val, fieldName) {
  const num = Number(val);
  if (!Number.isInteger(num) || num < 0) {
    throw new Error(`${fieldName} must be a non-negative integer`);
  }
  return num;
}
