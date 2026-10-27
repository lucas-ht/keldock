import type { Environment, Page, Project } from "@/lib/contracts";
import { api } from "@/lib/server-api";
import { Overview } from "@/components/deployments/overview";
import { Refresh } from "@/components/layout/refresh";
export const metadata = { title: "Overview" };
export default async function OverviewPage() {
  const projects = await api<Page<Project>>("/v1/projects");
  const pages = await Promise.all(
    projects.items.map((project) =>
      api<Page<Environment>>(`/v1/projects/${project.projectId}/environments`),
    ),
  );
  const environments = pages.flatMap((page) => page.items);
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>Deployment health</h1>
          <p>Selected releases and their latest observed state.</p>
        </div>
        <Refresh automatic />
      </div>
      <Overview
        environments={environments}
        projects={projects.items}
        more={pages.some((page) => !!page.nextCursor)}
      />
    </>
  );
}
