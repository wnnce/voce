import type { Property, Field } from '@/types/workflow';

export const validateProperty = (upstream: Property, downstream: Property): string | null => {
  if (upstream.prefix !== downstream.prefix) {
    return `Prefix mismatch: upstream ${upstream.prefix}, downstream ${downstream.prefix}`;
  }

  // Empty downstream Name = wildcard
  if (downstream.name !== '' && upstream.name !== downstream.name) {
    return `Name mismatch: upstream ${upstream.name}, downstream ${downstream.name}`;
  }

  // Signal-only downstream
  if (!downstream.fields || downstream.fields.length === 0) {
    return null;
  }

  const upFields: Record<string, Field> = {};
  upstream.fields?.forEach((f) => {
    upFields[f.key] = f;
  });

  for (const df of downstream.fields) {
    if (!df.required) continue;

    const uf = upFields[df.key];
    if (!uf) {
      return `Required field [${downstream.prefix}:${downstream.name}.${df.key}] not provided by upstream`;
    }

    if (df.type !== '' && uf.type !== df.type) {
      return `Type mismatch for field [${downstream.prefix}:${downstream.name}.${df.key}]: upstream ${uf.type}, downstream ${df.type}`;
    }

    if (!uf.required) {
      return `Field [${downstream.prefix}:${downstream.name}.${df.key}] is required by downstream but only optionally produced by upstream`;
    }
  }

  return null;
};

export const validateProperties = (upstreams: Property[], downstreams: Property[], prefix: string): string | null => {
  const filteredDowns = downstreams.filter(d => d.prefix === prefix);
  const filteredUps = upstreams.filter(u => u.prefix === prefix);

  if (filteredDowns.length === 0) {
    // Tolerant mode: downstream ignores everything of this type
    return null;
  }

  if (filteredUps.length === 0) {
    return `Source node does not produce [${prefix}] type signals`;
  }

  let hasAnyHit = false;

  for (const up of filteredUps) {
    let matchedDown: Property | undefined;

    // 1. Try exact match (prefix:name)
    matchedDown = filteredDowns.find(d => d.name === up.name);

    // 2. Try wildcard match (prefix:*) if no exact match found
    if (!matchedDown) {
      matchedDown = filteredDowns.find(d => d.name === "");
    }

    // Skip unrecognized
    if (!matchedDown) {
      continue;
    }

    // HIT!
    hasAnyHit = true;
    const err = validateProperty(up, matchedDown);
    if (err) {
      return `Contract violation for property [${up.prefix}:${up.name}]: ${err}`;
    }
  }

  if (!hasAnyHit) {
    return `None of the source [${prefix}] properties are recognized by the target plugin`;
  }

  return null;
};
