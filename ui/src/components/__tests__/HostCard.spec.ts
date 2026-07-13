import { describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";
import HostCard from "../HostCard.vue";
import type { HostStatus } from "@/types";

const host: HostStatus = {
  name: "rx9070",
  base_url: "http://192.168.18.29:11434",
  models: [{ name: "gemma4:12b", capabilities: ["tools"] }],
  up: true,
  last_seen: "2026-07-13T00:00:00Z",
};

describe("HostCard", () => {
  it("renders host name, url, and discovered models", () => {
    const wrapper = mount(HostCard, { props: { host } });
    expect(wrapper.text()).toContain("rx9070");
    expect(wrapper.text()).toContain("http://192.168.18.29:11434");
    expect(wrapper.text()).toContain("gemma4:12b");
  });

  it("shows offline status and last error when down", () => {
    const downHost: HostStatus = { ...host, up: false, last_error: "connection refused" };
    const wrapper = mount(HostCard, { props: { host: downHost } });
    expect(wrapper.text()).toContain("offline");
    expect(wrapper.text()).toContain("connection refused");
  });

  it("emits remove", async () => {
    const wrapper = mount(HostCard, { props: { host } });
    const buttons = wrapper.findAll("button");
    const removeBtn = buttons.find((b) => b.text() === "Remove")!;
    await removeBtn.trigger("click");
    expect(wrapper.emitted("remove")).toEqual([["rx9070"]]);
  });

  it("toggles the edit form and emits update on submit", async () => {
    const wrapper = mount(HostCard, { props: { host } });
    const editBtn = wrapper.findAll("button").find((b) => b.text() === "Edit")!;
    await editBtn.trigger("click");

    const form = wrapper.find("form");
    expect(form.exists()).toBe(true);
    await form.trigger("submit");
    // Empty submit (fields already pre-filled from host, so this should
    // actually emit update since name/baseUrl aren't empty).
    expect(wrapper.emitted("update")).toBeTruthy();
  });
});
