import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { CodeBlock } from "./code-block";

describe("CodeBlock", () => {
  it("renders code text and language label", () => {
    render(<CodeBlock code="show interface status" language="bash" />);
    expect(screen.getByText("show interface status")).toBeInTheDocument();
    expect(screen.getByText("bash")).toBeInTheDocument();
  });

  it("copies code to clipboard on copy button click", async () => {
    const user = userEvent.setup({ writeToClipboard: false });
    // userEvent.setup() 会用自带的 clipboard stub 覆盖 navigator.clipboard，
    // 因此需在 setup 之后对 stub 的 writeText 做 spy
    const writeText = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
    render(<CodeBlock code="goose up" language="bash" />);
    await user.click(screen.getByRole("button", { name: "复制代码" }));
    expect(writeText).toHaveBeenCalledWith("goose up");
  });
});
