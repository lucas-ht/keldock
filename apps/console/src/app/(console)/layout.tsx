import { api, requireSession } from "@/lib/server-api";
import { Shell } from "@/components/layout/shell";
export default async function ConsoleLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const session = await requireSession();
  const installation = await api<{ notice: string }>("/v1/installation");
  return (
    <Shell user={session.user} notice={installation.notice}>
      {children}
    </Shell>
  );
}
