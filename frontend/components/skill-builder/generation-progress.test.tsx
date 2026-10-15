import { act, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { GenerationProgress } from "./generation-progress";

afterEach(() => {
  vi.useRealTimers();
});

it("reports elapsed time and releases its timer", () => {
  vi.useFakeTimers();
  const view = render(<GenerationProgress updating={false} />);
  expect(screen.getByText("已等待 0 秒")).toBeInTheDocument();
  act(() => vi.advanceTimersByTime(46000));
  expect(screen.getByText("已等待 46 秒")).toBeInTheDocument();
  expect(screen.getByRole("status")).toHaveTextContent("仍在等待模型完成");
  view.unmount();
  expect(vi.getTimerCount()).toBe(0);
});
