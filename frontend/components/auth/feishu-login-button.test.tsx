import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { FeishuLoginButton } from "./feishu-login-button";

describe("FeishuLoginButton", () => {
  it("opens the server-owned Feishu OAuth flow", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const user = userEvent.setup();
    render(<FeishuLoginButton />);

    const link = screen.getByRole("link", { name: "使用飞书登录" });
    expect(link).toHaveAttribute(
      "href",
      "http://localhost:8080/api/v1/auth/feishu/start",
    );

    await user.tab();
    expect(link).toHaveFocus();
    expect(consoleError).not.toHaveBeenCalled();
    consoleError.mockRestore();
  });
});
