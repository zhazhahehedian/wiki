import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { IngestStatusBadge } from "./ingest-status-badge";

describe("IngestStatusBadge", () => {
  it("renders processing status with primary tone and spinner", () => {
    render(<IngestStatusBadge status="embedding" />);
    const badge = screen.getByText("向量化中").closest("[data-slot=badge]") ?? screen.getByText("向量化中");
    expect(badge.className).toContain("text-primary");
    expect(badge.className).not.toContain("bg-blue-100");
    expect(badge.querySelector(".animate-spin")).not.toBeNull();
  });

  it("renders failed status with destructive tone", () => {
    render(<IngestStatusBadge status="failed" />);
    const badge = screen.getByText("失败").closest("[data-slot=badge]") ?? screen.getByText("失败");
    expect(badge.className).toContain("text-destructive");
    expect(badge.className).not.toContain("bg-red-100");
  });
});
