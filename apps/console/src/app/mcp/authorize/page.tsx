import { redirect } from "next/navigation";
import { api, getSession } from "@/lib/server-api";
import type { Authorization, Project, Page } from "@/lib/contracts";
import { AuthFrame } from "@/components/auth/frame";
import { Consent } from "@/components/mcp/consent";
import { ErrorNotice } from "@/components/shared/feedback";
export const metadata = { title: "Authorize MCP access" };
export default async function Authorize({
  searchParams,
}: {
  searchParams: Promise<{ request?: string }>;
}) {
  const { request } = await searchParams;
  if (!request)
    return (
      <AuthFrame>
        <h1>Authorization unavailable</h1>
        <ErrorNotice message="No authorization request was supplied. Restart the connection in your MCP client." />
      </AuthFrame>
    );
  if (!(await getSession()))
    redirect(
      `/sign-in?next=${encodeURIComponent(`/mcp/authorize?request=${encodeURIComponent(request)}`)}`,
    );
  let content;
  try {
    const [authorization, projects] = await Promise.all([
      api<Authorization>(
        `/v1/mcp/authorizations/${encodeURIComponent(request)}`,
      ),
      api<Page<Project>>("/v1/projects"),
    ]);
    content = (
      <Consent authorization={authorization} projects={projects.items} />
    );
  } catch {
    content = (
      <ErrorNotice message="This request expired, was already used, or MCP access is disabled. Restart the connection in your MCP client." />
    );
  }
  return (
    <AuthFrame>
      <h1>Authorize MCP access</h1>
      {content}
    </AuthFrame>
  );
}
