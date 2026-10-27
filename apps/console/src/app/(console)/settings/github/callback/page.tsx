import { requireAdmin } from "@/lib/server-api";
import { GitHubCallback } from "@/components/github/callback";
export const metadata = { title: "Complete GitHub setup" };
export default async function CallbackPage({
  searchParams,
}: {
  searchParams: Promise<{ code?: string; state?: string }>;
}) {
  await requireAdmin();
  const query = await searchParams;
  return <GitHubCallback code={query.code} state={query.state} />;
}
