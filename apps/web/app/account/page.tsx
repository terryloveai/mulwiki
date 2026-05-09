import { AccountView } from "@mulwiki/views/auth/AccountView";
import { AuthGuard } from "@mulwiki/views/auth/AuthGuard";

export default function AccountPage() {
  return (
    <AuthGuard>
      <AccountView />
    </AuthGuard>
  );
}
