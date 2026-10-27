import { requireSession } from "@/lib/server-api";
import { AccountForm } from "@/components/auth/account";
export const metadata = { title: "Account" };
export default async function Account() {
  const { user } = await requireSession();
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>Your account</h1>
          <p>
            {user.name} · {user.email} · {user.role}
          </p>
        </div>
      </div>
      <AccountForm />
    </>
  );
}
