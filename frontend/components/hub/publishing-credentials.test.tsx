import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { PublishingCredentials } from "./publishing-credentials";
import { createPublishingCredential, listPublishingCredentials, revokePublishingCredential } from "@/lib/api/publishing";
vi.mock("@/lib/api/publishing",async importOriginal=>({...await importOriginal<typeof import("@/lib/api/publishing")>(),createPublishingCredential:vi.fn(),listPublishingCredentials:vi.fn(),revokePublishingCredential:vi.fn()}));
const credential={id:"c1",name:"CI",prefix:"chp_fixture",scopes:["capabilities:read" as const],created_at:"2026-10-27",expires_at:"2099-01-01"};
beforeEach(()=>{vi.mocked(listPublishingCredentials).mockReset().mockResolvedValue({items:[]});vi.mocked(createPublishingCredential).mockReset();vi.mocked(revokePublishingCredential).mockReset()});
function mount(){return render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><PublishingCredentials/></QueryClientProvider>)}
it("issues only selected scopes, shows the secret once and can hide it",async()=>{
  vi.mocked(createPublishingCredential).mockResolvedValue({...credential,token:"chp_one_time_secret"});
  const user=userEvent.setup();mount();await user.type(screen.getByLabelText("凭证名称"),"CI");
  await user.click(screen.getByRole("button",{name:"创建凭证"}));
  expect(await screen.findByLabelText("新 API 密钥")).toHaveTextContent("chp_one_time_secret");
  expect(createPublishingCredential).toHaveBeenCalledWith({name:"CI",scopes:["capabilities:read"],expires_at:expect.any(String)});
  await user.click(screen.getByRole("button",{name:"已保存，隐藏密钥"}));expect(screen.queryByText("chp_one_time_secret")).not.toBeInTheDocument();
});
it("revokes by credential ID and refreshes the list without exposing the secret",async()=>{
  vi.mocked(listPublishingCredentials).mockResolvedValueOnce({items:[credential]}).mockResolvedValue({items:[{...credential,revoked_at:"2026-10-27"}]});
  vi.mocked(revokePublishingCredential).mockResolvedValue(undefined);
  const user=userEvent.setup();mount();await user.click(await screen.findByRole("button",{name:"撤销 CI"}));
  await waitFor(()=>expect(revokePublishingCredential).toHaveBeenCalledWith("c1"));
  expect(await screen.findByText("已撤销")).toBeInTheDocument();expect(screen.queryByLabelText("新 API 密钥")).not.toBeInTheDocument();
});
