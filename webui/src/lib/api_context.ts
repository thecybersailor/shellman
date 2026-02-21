export interface APIContext {
  baseOrigin: string;
  turnUUID: string;
}

export function resolveAPIContext(input: string | URL = window.location.href): APIContext {
  const url = typeof input === "string" ? new URL(input) : input;
  const params = url.searchParams;
  const workerOrigin = params.get("worker_origin") ?? "";
  const turnUUID = params.get("turn_uuid") ?? "";
  return {
    baseOrigin: workerOrigin || url.origin,
    turnUUID
  };
}
