import { describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";
import BaseButton from "../BaseButton.vue";

describe("BaseButton", () => {
  it("renders slot content", () => {
    const wrapper = mount(BaseButton, { slots: { default: "Click me" } });
    expect(wrapper.text()).toBe("Click me");
  });

  it("emits click when not disabled", async () => {
    const wrapper = mount(BaseButton);
    await wrapper.trigger("click");
    expect(wrapper.emitted("click")).toHaveLength(1);
  });

  it("does not emit click when disabled", async () => {
    const wrapper = mount(BaseButton, { props: { disabled: true } });
    await wrapper.trigger("click");
    expect(wrapper.emitted("click")).toBeUndefined();
  });

  it("applies the variant class", () => {
    const wrapper = mount(BaseButton, { props: { variant: "danger" } });
    expect(wrapper.classes()).toContain("base-btn--danger");
  });
});
