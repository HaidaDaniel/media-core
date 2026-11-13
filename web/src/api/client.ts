// The typed API client for the frozen v1 application contract
// (docs/application-contract.md): the common envelope, the stable error
// codes, same-origin cookie sessions and the in-memory CSRF token. The
// client translates transport failures into typed errors; it re-implements
// no domain rule.

// One stable v1 error code (internal/app/contracts.go).
export type ErrorCode =
  | "invalid_argument"
  | "not_found"
  | "conflict"
  | "identity_conflict"
  | "duplicate_candidate"
  | "cycle_detected"
  | "unauthorized"
  | "forbidden"
  | "rate_limited"
  | "locked"
  | "schema_mismatch"
  | "storage_error"
  | "config_error"
  | "internal_error";

export interface ApiErrorBody {
  code: ErrorCode;
  message: string;
  retryable: boolean;
}

// The common envelope of every API response.
export interface Envelope<T> {
  schema_version: string;
  ok: boolean;
  data?: T;
  error?: ApiErrorBody;
}

// ApiError is a typed API failure. Forms keep their input on any ApiError
// and show the message; retryable errors (locked, rate_limited) and
// conflicts (stale revisions) are distinguished from input errors.
export class ApiError extends Error {
  readonly code: ErrorCode;
  readonly status: number;
  readonly retryable: boolean;

  constructor(code: ErrorCode, status: number, message: string, retryable: boolean) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
    this.retryable = retryable;
  }

  get isUnauthorized(): boolean {
    return this.code === "unauthorized" || this.code === "forbidden";
  }

  get isConflict(): boolean {
    return (
      this.code === "conflict" ||
      this.code === "identity_conflict" ||
      this.code === "duplicate_candidate" ||
      this.code === "cycle_detected"
    );
  }

  get isRetryable(): boolean {
    return this.code === "locked" || this.code === "rate_limited" || this.retryable;
  }
}

// The per-session CSRF header, required on every authenticated mutation.
const CSRF_HEADER = "X-CSRF-Token";

// The CSRF token lives in memory only, never in browser storage; it is
// set on login and refreshed through the session inspection endpoint.
let csrfToken: string | null = null;

export function setCSRFToken(token: string | null): void {
  csrfToken = token;
}

export function getCSRFToken(): string | null {
  return csrfToken;
}

// newRequestID mints the idempotency key of one mutation; the same key
// must be reused when a request is retried after a lost response.
export function newRequestID(): string {
  return crypto.randomUUID();
}

const API_PREFIX = "/api/v1";

interface RequestOptions {
  method: string;
  body?: unknown;
}

// request performs one same-origin API call and returns the envelope data.
// Sessions travel in the HttpOnly cookie; mutations carry the in-memory
// CSRF header. A failed envelope is a typed ApiError, never a throwaway
// rejection.
async function request<T>(path: string, options: RequestOptions): Promise<T> {
  const headers: Record<string, string> = {};
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (options.method !== "GET" && options.method !== "HEAD" && csrfToken !== null) {
    headers[CSRF_HEADER] = csrfToken;
  }
  let response: Response;
  try {
    response = await fetch(API_PREFIX + path, {
      method: options.method,
      headers,
      credentials: "same-origin",
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    });
  } catch {
    throw new ApiError("internal_error", 0, "the API is unreachable", true);
  }
  let parsed: Envelope<T>;
  try {
    parsed = (await response.json()) as Envelope<T>;
  } catch {
    throw new ApiError("internal_error", response.status, "the API returned a non-JSON response", false);
  }
  if (!response.ok || !parsed.ok || parsed.error !== undefined) {
    const body = parsed.error ?? { code: "internal_error", message: "the API returned an error", retryable: false };
    if (!body.code) {
      throw new ApiError("internal_error", response.status, body.message, body.retryable);
    }
    throw new ApiError(body.code, response.status, body.message, body.retryable);
  }
  if (parsed.data === undefined) {
    throw new ApiError("internal_error", response.status, "the API returned no data", false);
  }
  return parsed.data;
}

export function get<T>(path: string): Promise<T> {
  return request<T>(path, { method: "GET" });
}

export function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, { method: "POST", body });
}

export function patch<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, { method: "PATCH", body });
}

export function del<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, { method: "DELETE", body });
}

// The session payload of POST and GET /session: the per-session CSRF token.
interface SessionPayload {
  csrf_token: string;
}

// login exchanges the owner secret for a single-owner session. The secret
// is sent once, travels nowhere else and is never stored.
export async function login(ownerSecret: string): Promise<void> {
  const payload = await post<SessionPayload>("/session", { owner_secret: ownerSecret });
  setCSRFToken(payload.csrf_token);
}

// logout invalidates the session and clears the in-memory CSRF token.
export async function logout(): Promise<void> {
  await del("/session");
  setCSRFToken(null);
}

// inspectSession reports whether a live session exists and refreshes the
// in-memory CSRF token; a missing or expired session is not an error.
export async function inspectSession(): Promise<boolean> {
  try {
    const payload = await get<SessionPayload>("/session");
    setCSRFToken(payload.csrf_token);
    return true;
  } catch (err) {
    if (err instanceof ApiError && err.code === "unauthorized") {
      setCSRFToken(null);
      return false;
    }
    throw err;
  }
}
