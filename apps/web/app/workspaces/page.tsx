import { AuthGuard } from "@mulwiki/views/auth/AuthGuard";
import { WorkspacesView } from "@mulwiki/views/workspaces/WorkspacesView";

export default function WorkspacesPage() {
  return (
    <AuthGuard>
      <WorkspacesView />
    </AuthGuard>
  );
}
