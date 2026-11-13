// Client tests against the frozen v1 envelope: success decoding, typed
// error mapping, the in-memory CSRF token and the session endpoints. The
// global fetch is stubbed; no network or browser storage is involved.
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  del,
  get,
  getCSRFToken,
  inspectSession,
  login,
  newRequestID,
  post,
  setCSRFToken,
} from "./client";

function jsonResponse(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as Response;
}

function jsonResponseText(status: number, text: string): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => {
      throw new SyntaxError("not json");
    },
    text: async () => text,
  } as unknown as Response;
}

afterEach(() => {
  vi.unstubAllGlobals();
  setCSRFToken(null);
});

describe("envelope decoding", () => {
  it("returns the data of a success envelope", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { schema_version: "1.0", ok: true, data: { id: "w-1" } }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(get("/works/w-1")).resolves.toEqual({ id: "w-1" });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/works/w-1");
    expect(init.credentials).toBe("same-origin");
  });

  it("maps a failure envelope to a typed ApiError with its code and message", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        jsonResponse(409, {
          schema_version: "1.0",
          ok: false,
          error: { code: "conflict", message: "record changed; reload before retrying", retryable: false },
        }),
      ),
    );
    await expect(get("/works")).rejects.toMatchObject({
      code: "conflict",
      status: 409,
      message: "record changed; reload before retrying",
      retryable: false,
    } satisfies Partial<ApiError>);
  });

  it("distinguishes retryable, conflict and unauthorized errors", async () => {
    const make = (code: string, status: number, retryable: boolean) =>
      jsonResponse(status, { schema_version: "1.0", ok: false, error: { code, message: code, retryable } });
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(make("locked", 503, true))
        .mockResolvedValueOnce(make("cycle_detected", 409, false))
        .mockResolvedValueOnce(make("unauthorized", 401, false)),
    );
    const locked = await get("/works").catch((e: unknown) => e);
    expect((locked as ApiError).isRetryable).toBe(true);
    const cycle = await get("/works").catch((e: unknown) => e);
    expect((cycle as ApiError).isConflict).toBe(true);
    const unauthorized = await get("/works").catch((e: unknown) => e);
    expect((unauthorized as ApiError).isUnauthorized).toBe(true);
  });

  it("treats a non-JSON failure body as a transport error", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponseText(500, "boom")));
    const err = await get("/works").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).code).toBe("internal_error");
  });

  it("treats a network failure as a retryable transport error", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("failed to fetch")));
    const err = await get("/works").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).isRetryable).toBe(true);
  });
});

describe("CSRF handling", () => {
  it("sends the in-memory CSRF header on mutations and not on reads", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(200, { schema_version: "1.0", ok: true, data: {} }))
      .mockResolvedValueOnce(jsonResponse(200, { schema_version: "1.0", ok: true, data: {} }));
    vi.stubGlobal("fetch", fetchMock);
    setCSRFToken("token-1");
    await get("/works");
    await post("/works", { request_id: "r-1" });
    const [, getInit] = fetchMock.mock.calls[0];
    const [, postInit] = fetchMock.mock.calls[1];
    expect(getInit.headers["X-CSRF-Token"]).toBeUndefined();
    expect(postInit.headers["X-CSRF-Token"]).toBe("token-1");
    expect(postInit.body).toBe(JSON.stringify({ request_id: "r-1" }));
    expect(postInit.headers["Content-Type"]).toBe("application/json");
  });

  it("never sends a CSRF header before a token is known", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(200, { schema_version: "1.0", ok: true, data: {} }));
    vi.stubGlobal("fetch", fetchMock);
    await post("/works", {});
    const [, init] = fetchMock.mock.calls[0];
    expect(init.headers["X-CSRF-Token"]).toBeUndefined();
  });
});

describe("session endpoints", () => {
  it("login stores the CSRF token in memory only", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse(200, { schema_version: "1.0", ok: true, data: { csrf_token: "csrf-1" } }),
    );
    vi.stubGlobal("fetch", fetchMock);
    await login("owner-secret");
    expect(getCSRFToken()).toBe("csrf-1");
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/session");
    expect(JSON.parse(init.body)).toEqual({ owner_secret: "owner-secret" });
  });

  it("inspectSession refreshes the token and reports a missing session as false", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(jsonResponse(200, { schema_version: "1.0", ok: true, data: { csrf_token: "csrf-2" } }))
        .mockResolvedValueOnce(
          jsonResponse(401, {
            schema_version: "1.0",
            ok: false,
            error: { code: "unauthorized", message: "not authenticated", retryable: false },
          }),
        ),
    );
    await expect(inspectSession()).resolves.toBe(true);
    expect(getCSRFToken()).toBe("csrf-2");
    await expect(inspectSession()).resolves.toBe(false);
    expect(getCSRFToken()).toBeNull();
  });

  it("logout sends DELETE with the CSRF token and clears it", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(204, { schema_version: "1.0", ok: true, data: null }));
    vi.stubGlobal("fetch", fetchMock);
    setCSRFToken("csrf-3");
    await del("/session");
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/v1/session");
    expect(init.method).toBe("DELETE");
    expect(init.headers["X-CSRF-Token"]).toBe("csrf-3");
  });
});

describe("idempotency keys", () => {
  it("mints distinct request ids", () => {
    const ids = new Set(Array.from({ length: 50 }, () => newRequestID()));
    expect(ids.size).toBe(50);
  });
});
