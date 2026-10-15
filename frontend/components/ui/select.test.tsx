import { useState } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { Field } from "@/components/hub/registry-shared";
import { Select } from "./select";

const options = [
  { value: "", label: "全部状态" },
  { value: "draft", label: "草稿" },
  { value: "published", label: "已上线" },
];

it("supports keyboard selection and clearing a filter through its visible label", async () => {
  const user = userEvent.setup();
  function Form() {
    const [value, setValue] = useState("draft");
    return (
      <form aria-label="筛选表单">
        <Field label="状态" hint="选择状态筛选">
          <Select
            name="status"
            value={value}
            options={options}
            onValueChange={setValue}
          />
        </Field>
      </form>
    );
  }
  render(<Form />);
  const trigger = screen.getByRole("combobox", { name: "状态" });
  expect(trigger).toHaveAccessibleDescription("选择状态筛选");
  trigger.focus();
  await user.keyboard("{ArrowDown}{End}{Enter}");
  await waitFor(() => expect(trigger).toHaveTextContent("已上线"));
  expect(
    new FormData(screen.getByRole("form") as HTMLFormElement).get("status"),
  ).toBe("published");
  await user.click(trigger);
  await user.click(await screen.findByRole("option", { name: "全部状态" }));
  expect(trigger).toHaveTextContent("全部状态");
  expect(
    new FormData(screen.getByRole("form") as HTMLFormElement).get("status"),
  ).toBe("");
});

it("keeps the current value on Escape and returns focus to the trigger", async () => {
  const changed = vi.fn();
  render(
    <Select
      aria-label="状态"
      value="draft"
      options={options}
      onValueChange={changed}
    />,
  );
  const user = userEvent.setup();
  const trigger = screen.getByRole("combobox", { name: "状态" });
  await user.click(trigger);
  await user.keyboard("{ArrowDown}{Escape}");
  expect(changed).not.toHaveBeenCalled();
  expect(trigger).toHaveFocus();
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
});

it("shows a disabled placeholder when there are no models", async () => {
  const changed = vi.fn();
  render(
    <Select
      aria-label="默认模型"
      value=""
      options={[]}
      placeholder="先选择模型"
      disabled
      onValueChange={changed}
    />,
  );
  const trigger = screen.getByRole("combobox", { name: "默认模型" });
  expect(trigger).toBeDisabled();
  expect(trigger).toHaveTextContent("先选择模型");
  await userEvent.click(trigger);
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  expect(changed).not.toHaveBeenCalled();
});
