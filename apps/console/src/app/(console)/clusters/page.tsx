import { api, requireSession } from "@/lib/server-api";
import type { Cluster, Page } from "@/lib/contracts";
import { date } from "@/lib/format";
import { ClusterManagement } from "@/components/clusters/management";
import { Status } from "@/components/shared/status";
import { EmptyState } from "@/components/shared/feedback";
import { Refresh } from "@/components/layout/refresh";
export const metadata = { title: "Clusters" };
export default async function Clusters() {
  const [session, page] = await Promise.all([
    requireSession(),
    api<Page<Cluster>>("/v1/clusters"),
  ]);
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>Clusters</h1>
          <p>
            Enroll and observe the outbound agent on your customer-controlled
            infrastructure.
          </p>
        </div>
        <Refresh automatic />
      </div>
      <div className="panel data-list">
        {page.items.length ? (
          page.items.map((cluster) => (
            <section className="flex flex-col gap-6 p-6" key={cluster.id}>
              <div className="section-heading">
                <div>
                  <h2>{cluster.name}</h2>
                  <p>
                    {cluster.id} · Identity generation {cluster.revision}
                  </p>
                </div>
                <Status state={cluster.state} />
              </div>
              <dl className="grid gap-5 sm:grid-cols-3">
                <div>
                  <dt className="text-xs text-muted-foreground">Last seen</dt>
                  <dd className="mt-2">{date(cluster.lastSeenAt)}</dd>
                </div>
                <div>
                  <dt className="text-xs text-muted-foreground">
                    Agent version
                  </dt>
                  <dd className="mt-2">
                    {cluster.agentVersion || "Not reported"}
                  </dd>
                </div>
                <div>
                  <dt className="text-xs text-muted-foreground">
                    Credential expiry
                  </dt>
                  <dd className="mt-2">{date(cluster.expiresAt)}</dd>
                </div>
              </dl>
              {session.user.role === "admin" && (
                <ClusterManagement cluster={cluster} />
              )}
            </section>
          ))
        ) : (
          <EmptyState title="No accessible cluster">
            Cluster visibility follows your current project access.
          </EmptyState>
        )}
      </div>
    </>
  );
}
