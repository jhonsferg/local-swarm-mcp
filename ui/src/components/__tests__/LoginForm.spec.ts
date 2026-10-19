import { describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";
import LoginForm from "../LoginForm.vue";

describe("LoginForm", () => {
  it("emits the trimmed token and clears the field", async () => {
    const wrapper = mount(LoginForm);
    const input = wrapper.find("input[type='password']");
    await input.setValue("  secret  ");
    await wrapper.find("form").trigger("submit");
    expect(wrapper.emitted("submit")).toEqual([["secret"]]);
    expect((input.element as HTMLInputElement).value).toBe("");
  });

  it("does not submit an empty token", async () => {
    const wrapper = mount(LoginForm);
    await wrapper.find("form").trigger("submit");
    expect(wrapper.emitted("submit")).toBeUndefined();
  });

  it("shows an error", () => {
    const wrapper = mount(LoginForm, { props: { error: "That key was not accepted." } });
    expect(wrapper.text()).toContain("That key was not accepted.");
  });
});
