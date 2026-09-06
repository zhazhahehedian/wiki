import { HubShell } from "@/components/hub/hub-shell";
import { AuthenticatedPlaygroundProvider } from "@/components/playground/playground-provider";

export default function HubLayout({ children }: { children: React.ReactNode }) {
  return (
    <AuthenticatedPlaygroundProvider>
      <HubShell>{children}</HubShell>
    </AuthenticatedPlaygroundProvider>
  );
}
