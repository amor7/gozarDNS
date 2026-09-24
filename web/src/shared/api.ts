export class ApiError extends Error {
  constructor(
    public code: string,
    public status: number,
    message?: string,
  ) {
    super(message || code);
  }
}

type Opts = { method?: string; body?: unknown; form?: FormData };

let onUnauthorized: (() => void) | null = null;
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn;
}

/** JSON call to the app's own /api. Errors come back as ApiError with a code
 *  the UI translates. */
export async function api<T = unknown>(path: string, opts: Opts = {}): Promise<T> {
  const headers: Record<string, string> = { "X-Requested-With": "gozar" };
  let body: BodyInit | undefined;
  if (opts.form) {
    body = opts.form;
  } else if (opts.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(opts.body);
  }
  let res: Response;
  try {
    res = await fetch("/api" + path, {
      method: opts.method || (body ? "POST" : "GET"),
      headers,
      body,
      credentials: "same-origin",
    });
  } catch {
    throw new ApiError("network", 0);
  }
  if (res.status === 401 && onUnauthorized && !path.startsWith("/login")) onUnauthorized();
  const text = await res.text();
  let data: any = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = null;
  }
  if (!res.ok) throw new ApiError(data?.error || (res.status >= 500 ? "internal" : "bad_request"), res.status, data?.message);
  return data as T;
}

export const errCode = (e: unknown) => (e instanceof ApiError ? e.code : "internal");
