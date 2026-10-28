import Link from "next/link";
import { api, APIError, requireSession } from "@/lib/server-api";
import type { Environment, Page } from "@/lib/contracts";
import type { CloudInventory } from "@/lib/cloud-contracts";
import { CloudInventoryView } from "@/components/cloud/inventory";
import type { DatabaseInventory } from "@/lib/database-contracts";
import { DatabaseInventoryView } from "@/components/databases/inventory";
import { ClusterManagement } from "@/components/clusters/management";
import { Refresh } from "@/components/layout/refresh";
import { EmptyState } from "@/components/shared/feedback";

export default async function ResourcesPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;
  const base = `/v1/projects/${encodeURIComponent(projectId)}`;
  const session = await requireSession();
  const [inventory, databases, environments] = await Promise.all([
    api<CloudInventory>(`${base}/cloud`).catch((error) => {
      if (error instanceof APIError && error.status === 404) return null;
      throw error;
    }),
    api<DatabaseInventory>(`${base}/databases`).catch((error) => {
      if (error instanceof APIError && error.status === 404) return null;
      throw error;
    }),
    api<Page<Environment>>(`${base}/environments`),
  ]);
  if (!inventory && !databases)
    return (
      <EmptyState title="No resource profiles are enabled">
        An operator can enable PostgreSQL or an approved cloud profile for this
        project.{" "}
        <Link href={`/projects/${projectId}`}>Return to environments</Link>.
      </EmptyState>
    );
  const choices = environments.items
    .filter((item) => !item.deleted)
    .map((item) => ({
      id: item.id,
      label: `${item.application} · ${item.pullRequest ? `PR #${item.pullRequest}` : "Staging"}`,
      generation: item.generation,
      components: item.components.map((component) => ({
        name: component.name,
        kind: component.kind,
        routed: component.routed,
      })),
    }));
  const admin = session.user.role === "admin";
  return (
    <>
      <div className="page-heading">
        <div>
          <p className="eyebrow">
            <Link href={`/projects/${projectId}`}>
              {projectId} / Environments
            </Link>
          </p>
          <h1>Resources</h1>
          <p>Environment databases, retained storage, and recovery evidence.</p>
        </div>
        <Refresh automatic />
      </div>
      {databases && (
        <DatabaseInventoryView
          project={projectId}
          inventory={databases}
          environments={choices}
          admin={admin}
          operate={admin || session.user.role === "operator"}
          now={Date.now()}
        />
      )}
      {inventory && (
        <CloudInventoryView
          project={projectId}
          inventory={inventory}
          environments={choices}
          admin={admin}
          operate={admin || session.user.role === "operator"}
          now={Date.now()}
        />
      )}
      {admin && inventory && (
        <section className="panel mt-7" aria-label="Cloud agent">
          <div className="panel-toolbar">
            <div>
              <h2 className="font-semibold">Cloud agent</h2>
              <p className="muted text-sm">
                {inventory.agent.state} · {inventory.agent.id}
              </p>
            </div>
          </div>
          <div className="p-5">
            <ClusterManagement cluster={inventory.agent} cloud />
          </div>
        </section>
      )}
    </>
  );
}
