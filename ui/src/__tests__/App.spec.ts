import { afterEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import App from "../App.vue";

// A fake daemon: every /admin data route answers 401 until /admin/login
// succeeds with the right token.
function fakeDaemon(token: string) {
  let authed = false;
  const fetchFn = vi.fn(async (path: string, init?: RequestInit) => {
    const json = (status: number, body: unknown) =>
      new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
    if (path === "/admin/login") {
      const sent = JSON.parse(String(init?.body)).token;
      if (sent !== token) return json(401, "invalid token");
      authed = true;
      return new Response(null, { status: 204 });
    }
    if (!authed) return json(401, "missing bearer token");
    switch (path) {
      case "/admin/health":
        return json(200, { service: "local-swarm-mcp", version: "v-test", command_registration: false, auth_required: true });
      case "/admin/backends":
        return json(200, []);
      case "/admin/hosts":
        return json(200, []);
      case "/admin/mcp-servers":
        return json(200, []);
      default:
        return json(404, "not found");
    }
  });
  vi.stubGlobal("fetch", fetchFn);
  return fetchFn;
}

class FakeEventSource {
  onmessage: ((ev: MessageEvent) => void) | null = null;
  close() {}
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("App", () => {
  it("asks for the API key on 401, then loads the dashboard after signing in", async () => {
    fakeDaemon("secret");
    vi.stubGlobal("EventSource", FakeEventSource);
    const wrapper = mount(App);
    await flushPromises();

    expect(wrapper.text()).toContain("Sign in");
    expect(wrapper.text()).not.toContain("Inference hosts");

    const input = wrapper.find("input[type='password']");
    await input.setValue("wrong");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(wrapper.text()).toContain("That key was not accepted.");

    await input.setValue("secret");
    await wrapper.find("form").trigger("submit");
    await flushPromises();
    expect(wrapper.text()).toContain("Inference hosts");
    expect(wrapper.text()).toContain("v-test");
    expect(wrapper.text()).toContain("Sign out");
    // command_registration=false hides the register-server form.
    expect(wrapper.text()).toContain("Registering new servers is disabled");
    expect(wrapper.text()).not.toContain("+ Register server");
    wrapper.unmount();
  });
});
