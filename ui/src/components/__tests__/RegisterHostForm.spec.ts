import { describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";
import RegisterHostForm from "../RegisterHostForm.vue";

describe("RegisterHostForm", () => {
  it("emits submit with trimmed field values", async () => {
    const wrapper = mount(RegisterHostForm);
    await wrapper.find("input[placeholder='rx9070']").setValue("  rx9070  ");
    await wrapper.find("input[placeholder='http://192.168.18.29:11434']").setValue("  http://192.168.18.29:11434  ");
    await wrapper.find("form").trigger("submit");

    const submitted = wrapper.emitted("submit");
    expect(submitted).toHaveLength(1);
    expect(submitted![0][0]).toEqual({ name: "rx9070", baseUrl: "http://192.168.18.29:11434", apiKey: "" });
  });

  it("does not emit submit when name or base URL is empty", async () => {
    const wrapper = mount(RegisterHostForm);
    await wrapper.find("form").trigger("submit");
    expect(wrapper.emitted("submit")).toBeUndefined();
  });

  it("pre-fills fields from initial props and locks the name when editing", () => {
    const wrapper = mount(RegisterHostForm, {
      props: { initialName: "rx9070", initialBaseUrl: "http://x:11434", lockName: true, submitLabel: "Save" },
    });
    const nameInput = wrapper.find("input[placeholder='rx9070']");
    expect((nameInput.element as HTMLInputElement).value).toBe("rx9070");
    expect((nameInput.element as HTMLInputElement).disabled).toBe(true);
    expect(wrapper.text()).toContain("Save");
  });

  it("emits cancel", async () => {
    const wrapper = mount(RegisterHostForm);
    const buttons = wrapper.findAll("button");
    await buttons[buttons.length - 1].trigger("click");
    expect(wrapper.emitted("cancel")).toHaveLength(1);
  });
});
