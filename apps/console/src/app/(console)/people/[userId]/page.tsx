import { api, requireAdmin } from "@/lib/server-api";
import type { User, Project, Page } from "@/lib/contracts";
import { EditPerson } from "@/components/people/edit";
export default async function Person({
  params,
}: {
  params: Promise<{ userId: string }>;
}) {
  const session = await requireAdmin();
  const { userId } = await params;
  const [user, projects] = await Promise.all([
    api<User>(`/v1/users/${encodeURIComponent(userId)}`),
    api<Page<Project>>("/v1/projects"),
  ]);
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>{user.name}</h1>
          <p>Manage account status, role, and project grants.</p>
        </div>
      </div>
      <EditPerson
        user={user}
        projects={projects.items}
        self={user.id === session.user.id}
      />
    </>
  );
}
