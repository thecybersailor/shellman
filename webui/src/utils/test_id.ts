export function toStableTestId(raw: string): string {
  return raw.replace(/[^a-zA-Z0-9_-]/g, "_");
}
