export function parseObjectInput(
  value: string,
  label: string,
): Record<string, unknown> {
  const trimmed = value.trim();
  if (!trimmed) {
    return {};
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch (error) {
    throw new Error(`${label} must be valid JSON.`);
  }

  if (!isPlainObject(parsed)) {
    throw new Error(`${label} must be a JSON object.`);
  }

  return parsed;
}

export function parseStringMapInput(
  value: string,
  label: string,
): Record<string, string> {
  const parsed = parseObjectInput(value, label);
  const output: Record<string, string> = {};

  for (const [key, entry] of Object.entries(parsed)) {
    if (typeof entry !== "string") {
      throw new Error(`${label} must contain only string values.`);
    }
    output[key] = entry;
  }

  return output;
}

export function prettyJSON(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}
