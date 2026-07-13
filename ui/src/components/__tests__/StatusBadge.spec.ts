import { describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";
import StatusBadge from "../StatusBadge.vue";

describe("StatusBadge", () => {
  it("shows online label and class when online", () => {
    const wrapper = mount(StatusBadge, { props: { online: true } });
    expect(wrapper.text()).toContain("online");
    expect(wrapper.classes()).toContain("status-badge--online");
  });

  it("shows offline label and class when offline", () => {
    const wrapper = mount(StatusBadge, { props: { online: false } });
    expect(wrapper.text()).toContain("offline");
    expect(wrapper.classes()).toContain("status-badge--offline");
  });

  it("supports custom labels", () => {
    const wrapper = mount(StatusBadge, {
      props: { online: true, onLabel: "connected", offLabel: "disconnected" },
    });
    expect(wrapper.text()).toContain("connected");
  });
});
