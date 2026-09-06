import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import {
  AuthenticatedPlaygroundProvider,
  useModelConnection,
} from "./playground-provider";
import { playgroundApi } from "@/lib/api/playground";
const auth = vi.hoisted(() => ({
  id: "alice",
  isError: false,
  isFetching: false,
  expired: false,
}));
vi.mock("@/lib/hooks/use-auth", () => ({
  useAuth: () => ({
    data: { id: auth.id },
    isError: auth.isError,
    isFetching: auth.isFetching,
  }),
}));
vi.mock("@/components/auth/auth-session-boundary", () => ({
  useSessionExpired: () => auth.expired,
}));
vi.mock("@/lib/api/playground", () => ({
  playgroundApi: { get: vi.fn(), save: vi.fn(), delete: vi.fn() },
}));
beforeEach(() => {
  vi.resetAllMocks();
  Object.assign(auth, {
    id: "alice",
    isError: false,
    isFetching: false,
    expired: false,
  });
  vi.mocked(playgroundApi.get).mockImplementation(async () => ({
    protocol: "openai",
    baseUrl: "https://models.example/v1",
    models: [auth.id],
    defaultModel: auth.id,
    hasKey: true,
    version: 1,
  }));
});
function Probe() {
  const { connection } = useModelConnection();
  return <span>{connection?.defaultModel ?? "empty"}</span>;
}
it.each(["identity", "error", "expiry"])(
  "retains metadata on background refresh and clears it on %s",
  async (reason) => {
    const view = render(
      <AuthenticatedPlaygroundProvider>
        <Probe />
      </AuthenticatedPlaygroundProvider>,
    );
    await screen.findByText("alice");
    auth.isFetching = true;
    view.rerender(
      <AuthenticatedPlaygroundProvider>
        <Probe />
      </AuthenticatedPlaygroundProvider>,
    );
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(playgroundApi.get).toHaveBeenCalledTimes(1);
    if (reason === "identity") auth.id = "bob";
    if (reason === "error") auth.isError = true;
    if (reason === "expiry") auth.expired = true;
    view.rerender(
      <AuthenticatedPlaygroundProvider>
        <Probe />
      </AuthenticatedPlaygroundProvider>,
    );
    await waitFor(() =>
      expect(screen.queryByText("alice")).not.toBeInTheDocument(),
    );
    if (reason === "identity") await screen.findByText("bob");
  },
);
