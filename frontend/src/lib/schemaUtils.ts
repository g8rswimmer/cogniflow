export function getTemplateFields(schema: Record<string, unknown>): string[] {
  const properties =
    (schema.properties as Record<string, Record<string, unknown>> | undefined) ?? {}
  // Exclude x-textarea fields — they carry their own inline variable picker
  return Object.entries(properties)
    .filter(([, p]) => p['x-template'] && !p['x-textarea'])
    .map(([k]) => k)
}
