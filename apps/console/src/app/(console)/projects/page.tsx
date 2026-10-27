import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { api } from "@/lib/server-api";
import type { Page, Project } from "@/lib/contracts";
import { EmptyState } from "@/components/shared/feedback";
import { Button } from "@/components/ui/button";
export const metadata = { title: "Projects" };
export default async function Projects() {
  const page = await api<Page<Project>>("/v1/projects");
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>Projects</h1>
          <p>
            Repository ownership, environments, and delivery on your cluster.
          </p>
        </div>
      </div>
      <div className="panel data-list">
        {page.items.length ? (
          page.items.map((project) => (
            <div className="data-row" key={project.projectId}>
              <div>
                <h2 className="font-semibold">{project.name}</h2>
                <p>
                  {project.environments} environments · {project.ready} last
                  reported ready · {project.attention} need attention
                </p>
                <p>
                  Repository #{project.repositoryId} · {project.clusterId}
                </p>
              </div>
              <Button
                variant="outline"
                nativeButton={false}
                render={<Link href={`/projects/${project.projectId}`} />}
              >
                View environments
                <ArrowRight data-icon="inline-end" />
              </Button>
            </div>
          ))
        ) : (
          <EmptyState title="No accessible projects">
            Ask an administrator to grant project access. Projects are
            registered in the installation configuration.
          </EmptyState>
        )}
      </div>
    </>
  );
}
