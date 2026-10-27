import { api, requireAdmin } from "@/lib/server-api";
import type { Project, Page } from "@/lib/contracts";
import { InvitePerson } from "@/components/people/invite";
export const metadata = { title: "Invite person" };
export default async function Invite() {
  await requireAdmin();
  const projects = await api<Page<Project>>("/v1/projects");
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>Invite a person</h1>
          <p>Create a one-time activation link and choose their access.</p>
        </div>
      </div>
      <InvitePerson projects={projects.items} />
    </>
  );
}
