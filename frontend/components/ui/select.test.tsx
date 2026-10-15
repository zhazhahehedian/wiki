import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { Field } from "@/components/hub/registry-shared";
import { Select } from "./select";

it("wires the Field label and hint and submits an empty value", () => {
  render(
    <form aria-label="筛选表单">
      <Field label="状态" hint="选择状态筛选">
        <Select
          name="status"
          value=""
          options={[
            { value: "", label: "全部状态" },
            { value: "draft", label: "草稿" },
          ]}
          onValueChange={() => {}}
        />
      </Field>
    </form>,
  );
  const trigger = screen.getByRole("combobox", { name: "状态" });
  expect(trigger).toHaveAccessibleDescription("选择状态筛选");
  expect(trigger).toHaveTextContent("全部状态");
  expect(
    new FormData(screen.getByRole("form") as HTMLFormElement).get("status"),
  ).toBe("");
});
