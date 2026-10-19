import { afterEach, describe, expect, it, vi } from "vitest";
import { AuthError, useAdminApi } from "../useAdminApi";

function mockFetch(status: number, body: unknown = null) {
  const fn = vi.fn().mockResolvedValue(
    new Response(body === null ? null : JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fn);
  return fn;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("useAdminApi", () => {
  it("throws AuthError on 401", async () => {
    mockFetch(401);
    await expect(useAdminApi().hosts()).rejects.toBeInstanceOf(AuthError);
  });

  it("sends JSON bodies with a JSON content type", async () => {
    const fetchFn = mockFetch(204);
    await useAdminApi().login("secret");
    const [path, init] = fetchFn.mock.calls[0];
    expect(path).toBe("/admin/login");
    expect(init.method).toBe("POST");
    expect(init.headers["Content-Type"]).toBe("application/json");
    expect(JSON.parse(init.body)).toEqual({ token: "secret" });
  });

  it("omits an empty api_key so the daemon keeps the stored one", async () => {
    const fetchFn = mockFetch(200, { name: "h" });
    await useAdminApi().registerHost("h", "http://x:11434", "");
    expect(JSON.parse(fetchFn.mock.calls[0][1].body)).toEqual({ name: "h", base_url: "http://x:11434" });
  });

  it("surfaces the server's error text", async () => {
    mockFetch(403, "registering downstream MCP servers is disabled");
    await expect(useAdminApi().registerMCPServer("x", "y", [])).rejects.toThrow(/403/);
  });
});
