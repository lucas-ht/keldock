import { redirect } from "next/navigation";
import { getSession } from "@/lib/server-api";
import { AuthFrame } from "@/components/auth/frame";
import { SignInForm } from "@/components/auth/forms";
export const metadata = { title: "Sign in" };
export default async function SignIn({
  searchParams,
}: {
  searchParams: Promise<{ next?: string }>;
}) {
  const query = await searchParams;
  const next =
    query.next?.startsWith("/") &&
    !query.next.startsWith("//") &&
    !query.next.includes("\\") &&
    !/[\r\n]/.test(query.next)
      ? query.next
      : "/";
  if (await getSession()) redirect(next);
  return (
    <AuthFrame>
      <h1>Welcome back</h1>
      <p>Sign in to your Keldock installation.</p>
      <SignInForm next={next} />
    </AuthFrame>
  );
}
