import type { IntegrationSchemaProperty, IntegrationSchemaSpec, IntegrationTypeSpec } from "../types";

export type FormFieldValue = string | boolean;

export interface FormFieldDefinition {
  key: string;
  property: IntegrationSchemaProperty;
  required: boolean;
}

export function asIntegrationTypeSpec(value: unknown): IntegrationTypeSpec {
  return value as IntegrationTypeSpec;
}

export function schemaFields(schema: IntegrationSchemaSpec | undefined): FormFieldDefinition[] {
  const properties = schema?.properties ?? {};
  const requiredKeys = new Set(schema?.required ?? []);

  return Object.entries(properties).map(([key, property]) => ({
    key,
    property,
    required: requiredKeys.has(key),
  }));
}

export function initializeSchemaValues(schema: IntegrationSchemaSpec | undefined): Record<string, FormFieldValue> {
  const values: Record<string, FormFieldValue> = {};

  for (const field of schemaFields(schema)) {
    values[field.key] = defaultValueForField(field.property);
  }

  return values;
}

export function defaultValueForField(property: IntegrationSchemaProperty): FormFieldValue {
  if (property.type === "boolean") {
    return Boolean(property.default);
  }

  if (property.type === "array") {
    if (Array.isArray(property.default)) {
      return JSON.stringify(property.default, null, 2);
    }
    return "[]";
  }

  if (typeof property.default === "number") {
    return String(property.default);
  }

  if (typeof property.default === "string") {
    return property.default;
  }

  return "";
}

export function serializeSchemaValues(
  schema: IntegrationSchemaSpec | undefined,
  values: Record<string, FormFieldValue>,
): Record<string, unknown> {
  const output: Record<string, unknown> = {};

  for (const field of schemaFields(schema)) {
    const raw = values[field.key];
    const parsed = parseFieldValue(field.property, raw);
    if (parsed !== undefined) {
      output[field.key] = parsed;
    }
  }

  return output;
}

function parseFieldValue(property: IntegrationSchemaProperty, raw: FormFieldValue | undefined): unknown {
  if (property.type === "boolean") {
    return Boolean(raw);
  }

  const value = typeof raw === "string" ? raw.trim() : "";
  if (value === "") {
    return undefined;
  }

  switch (property.type) {
    case "integer":
      return Number.parseInt(value, 10);
    case "number":
      return Number.parseFloat(value);
    case "array":
      try {
        const parsed = JSON.parse(value);
        return Array.isArray(parsed) ? parsed : undefined;
      } catch {
        return value
          .split(",")
          .map((item) => item.trim())
          .filter(Boolean);
      }
    default:
      return value;
  }
}
