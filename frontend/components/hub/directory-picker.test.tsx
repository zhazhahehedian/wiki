import { useState } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { DirectoryPicker } from "./directory-picker";
import { listDirectoryDepartments, listDirectoryUsers } from "@/lib/api/directory";
import { APIError } from "@/lib/api/client";
vi.mock("@/lib/api/directory",()=>({listDirectoryDepartments:vi.fn(),listDirectoryUsers:vi.fn()}));
function mount(node:React.ReactNode){return render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}>{node}</QueryClientProvider>)}
beforeEach(()=>{vi.mocked(listDirectoryDepartments).mockReset();vi.mocked(listDirectoryUsers).mockReset()});
it("loads lazily, paginates, and returns the stable department ID",async()=>{
  const change=vi.fn();const user=userEvent.setup();
  vi.mocked(listDirectoryDepartments).mockResolvedValueOnce({items:[{open_department_id:"od-one",name:"同名部门"}],has_more:true,page_token:"next"}).mockResolvedValueOnce({items:[{open_department_id:"od-two",name:"同名部门"}],has_more:false,page_token:""});
  mount(<DirectoryPicker kind="department" value="" onChange={change}/>);
  expect(listDirectoryDepartments).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button",{name:"选择飞书部门"}));
  await user.click(await screen.findByRole("button",{name:"下一页部门"}));
  await waitFor(()=>expect(listDirectoryDepartments).toHaveBeenCalledWith("0","next",expect.any(AbortSignal)));
  await user.click(await screen.findByRole("button",{name:"选择 同名部门"}));
  expect(change).toHaveBeenCalledWith("od-two","同名部门");
});
it("selects users by open_id and preserves selection when browsing",async()=>{
  vi.mocked(listDirectoryDepartments).mockResolvedValue({items:[],has_more:false,page_token:""});
  vi.mocked(listDirectoryUsers).mockResolvedValue({items:[{open_id:"ou-bob",name:"Bob"}],has_more:false,page_token:""});
  function Users(){const [ids,setIds]=useState<string[]>([]);return <DirectoryPicker kind="users" value={ids} onChange={setIds}/>}
  const user=userEvent.setup();mount(<Users/>);await user.click(screen.getByRole("button",{name:"选择飞书用户"}));
  await user.click(await screen.findByRole("checkbox",{name:/Bob/}));expect(screen.getByText("已选 1 / 200 人")).toBeInTheDocument();
  await user.click(screen.getByRole("button",{name:"移除 Bob"}));expect(screen.getByRole("checkbox",{name:/Bob/})).not.toBeChecked();
});
it("shows reauthorization instead of an empty successful directory",async()=>{
  vi.mocked(listDirectoryDepartments).mockRejectedValue(new APIError(403,"directory_authorization_required","需要重新授权"));
  const user=userEvent.setup();mount(<DirectoryPicker kind="department" value="" onChange={()=>{}}/>);await user.click(screen.getByRole("button",{name:"选择飞书部门"}));
  expect(await screen.findByRole("alert")).toHaveTextContent("需要重新授权");
  expect(screen.getByRole("link",{name:"重新授权飞书"})).toHaveAttribute("href",expect.stringContaining("/auth/feishu/start"));
});

it("navigates from an accessible own department when the root is hidden",async()=>{
  vi.mocked(listDirectoryDepartments).mockImplementation(async(parent)=>parent==="0"
    ? {items:[{open_department_id:"od-own",name:"本人部门"}],has_more:false,page_token:"",limited_root:true}
    : {items:[],has_more:false,page_token:""});
  vi.mocked(listDirectoryUsers).mockImplementation(async(parent)=>parent==="0"
    ? {items:[],has_more:false,page_token:"",limited_root:true}
    : {items:[{open_id:"ou-bob",name:"Bob"}],has_more:false,page_token:""});
  const change=vi.fn(); const user=userEvent.setup();
  mount(<DirectoryPicker kind="users" value={[]} onChange={change}/>);
  await user.click(screen.getByRole("button",{name:"选择飞书用户"}));
  expect(await screen.findByText(/飞书限制了组织根节点访问/)).toBeInTheDocument();
  await user.click(await screen.findByRole("button",{name:"查看下级与人员"}));
  await user.click(await screen.findByRole("checkbox",{name:/Bob/}));
  expect(change).toHaveBeenCalledWith(["ou-bob"]);
  expect(listDirectoryUsers).toHaveBeenCalledWith("od-own","",expect.any(AbortSignal));
});
