import Link from "next/link";
import { api } from "@/lib/server-api";
import type { Environment, Page, Project } from "@/lib/contracts";
import { Overview } from "@/components/deployments/overview";
import { Refresh } from "@/components/layout/refresh";
import { Button, buttonVariants } from "@/components/ui/button";
export default async function ProjectPage({
  params,
  searchParams,
}: {
  params: Promise<{ projectId: string }>;
  searchParams: Promise<{ after?: string }>;
}) {
  const { projectId } = await params;
  const { after } = await searchParams;
  const [projects, page, installation] = await Promise.all([
    api<Page<Project>>("/v1/projects"),
    api<Page<Environment>>(
      `/v1/projects/${encodeURIComponent(projectId)}/environments?after=${encodeURIComponent(after ?? "")}`,
    ),
    api<{ cloudProjects?: string[]; databaseProjects?: string[] }>(
      "/v1/installation",
    ),
  ]);
  const project = projects.items.find((item) => item.projectId === projectId);
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>{project?.name ?? projectId}</h1>
          <p>
            Current desired revisions and evidence from{" "}
            {project?.clusterId ?? "the assigned cluster"}.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {(installation.cloudProjects?.includes(projectId) ||
            installation.databaseProjects?.includes(projectId)) && (
            <Link
              className={buttonVariants({ variant: "outline" })}
              href={`/projects/${projectId}/resources`}
            >
              Resources
            </Link>
          )}
          <Refresh automatic />
        </div>
      </div>
      <Overview
        environments={page.items}
        projects={projects.items}
        more={!!page.nextCursor}
      />
      {page.nextCursor && (
        <div className="mt-6">
          <Button
            variant="outline"
            nativeButton={false}
            render={
              <Link
                href={`/projects/${projectId}?after=${encodeURIComponent(page.nextCursor)}`}
              />
            }
          >
            Next environments
          </Button>
        </div>
      )}
      {after && (
        <div className="mt-4">
          <Link href={`/projects/${projectId}`}>Back to first page</Link>
        </div>
      )}
    </>
  );
}
