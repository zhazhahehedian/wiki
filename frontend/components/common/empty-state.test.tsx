import { render, screen } from "@testing-library/react";
import { FileText } from "lucide-react";
import { describe, expect, it } from "vitest";

import { EmptyState } from "./empty-state";

describe("EmptyState", () => {
  it("renders icon, title, description and action", () => {
    render(
      <EmptyState
        icon={FileText}
        title="还没有文档"
        description="拖入文件开始构建知识库"
        action={<button>上传</button>}
      />,
    );
    expect(screen.getByText("还没有文档")).toBeInTheDocument();
    expect(screen.getByText("拖入文件开始构建知识库")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "上传" })).toBeInTheDocument();
  });

  it("renders without optional description and action", () => {
    render(<EmptyState icon={FileText} title="空空如也" />);
    expect(screen.getByText("空空如也")).toBeInTheDocument();
  });
});
