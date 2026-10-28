import Link from "next/link";
import { Database, Archive, ShieldCheck } from "lucide-react";
import type { DatabaseInventory } from "@/lib/database-contracts";
import { date } from "@/lib/format";
import { Status } from "@/components/shared/status";
import { EmptyState } from "@/components/shared/feedback";
import { ClusterManagement } from "@/components/clusters/management";
import { DatabaseActions } from "./actions";

export function DatabaseInventoryView({
  project,
  inventory,
  admin,
  operate,
  now,
  environments,
}: {
  project: string;
  inventory: DatabaseInventory;
  admin: boolean;
  operate: boolean;
  now: number;
  environments: { id: string; label: string }[];
}) {
  return (
    <section className="flex flex-col gap-5" aria-label="PostgreSQL resources">
      <div className="panel">
        <div className="panel-toolbar">
          <div>
            <h2 className="flex items-center gap-2 font-semibold">
              <Database className="size-4" />
              PostgreSQL
            </h2>
            <p className="muted text-sm">
              Environment databases · {inventory.items.length} of{" "}
              {inventory.configuration.limit} retained allocation slots
            </p>
          </div>
          <Status state={inventory.agent.state} />
        </div>
        {!inventory.items.length ? (
          <EmptyState title="No databases allocated yet">
            Declare a PostgreSQL resource using an approved profile in your
            application and bind its components. A new preview receives an empty
            database.
          </EmptyState>
        ) : (
          <div className="divide-y divide-border">
            {inventory.items.map((item) => {
              const observed =
                item.observation?.generation === item.generation
                  ? item.observation
                  : null;
              const stale =
                observed && now - Date.parse(observed.observedAt) > 90_000;
              const backup = observed?.backup;
              return (
                <article className="flex flex-col gap-5 p-5" key={item.id}>
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <h3 className="font-semibold">{item.alias}</h3>
                      <Link
                        className="muted text-sm"
                        href={`/projects/${project}/environments/${item.environmentId}`}
                      >
                        {environments.find(
                          (environment) =>
                            environment.id === item.environmentId,
                        )?.label ?? item.environmentId}
                      </Link>
                    </div>
                    <Status
                      state={
                        item.action === "retain"
                          ? "retained"
                          : stale
                            ? "stale"
                            : (observed?.state ?? "pending")
                      }
                    />
                  </div>
                  <div className="grid gap-5 md:grid-cols-3">
                    <div>
                      <p className="muted text-xs">Profile & storage</p>
                      <p className="mt-1 text-sm">
                        {item.profile} · {item.configuration.storageSize}
                      </p>
                      <p className="muted text-xs">
                        {item.configuration.storageClass}
                      </p>
                    </div>
                    <div>
                      <p className="muted text-xs">Authorized consumers</p>
                      <p className="mt-1 text-sm">
                        {item.action === "retain"
                          ? "Disconnected"
                          : item.components.join(", ") ||
                            "No component bindings"}
                      </p>
                      <p className="muted text-xs">
                        Generation {item.generation}
                      </p>
                    </div>
                    <div>
                      <p className="muted text-xs">Backup & WAL</p>
                      <p className="mt-1 flex items-center gap-2 text-sm">
                        <Archive className="size-3.5" />
                        {backup && stale
                          ? "Backup observation is stale"
                          : backup?.state === "ready"
                            ? "Base backup and WAL observed"
                            : (backup?.reason?.replaceAll("_", " ") ??
                              "No backup configured")}
                      </p>
                      {backup?.lastBaseBackupAt && (
                        <p className="muted text-xs">
                          Base completed {date(backup.lastBaseBackupAt)}
                        </p>
                      )}
                      {backup?.lastArchivedAt && (
                        <p className="muted text-xs">
                          Archive updated {date(backup.lastArchivedAt)}
                        </p>
                      )}
                    </div>
                  </div>
                  {backup?.restoreNamespace && (
                    <div className="flex items-start gap-2 text-sm">
                      <ShieldCheck className="mt-0.5 size-4 shrink-0" />
                      <div>
                        <p>
                          {backup.restoreReady
                            ? "Isolated restore was observed ready; verify recovered data"
                            : "Isolated restore is starting"}
                        </p>
                        <p className="muted break-words text-xs">
                          {backup.restoreNamespace} / {backup.restoreName} · No
                          application binding
                        </p>
                      </div>
                    </div>
                  )}
                  {observed && (
                    <p className="muted text-xs">
                      {observed.reason.replaceAll("_", " ")} · Observed{" "}
                      {date(observed.observedAt)}
                    </p>
                  )}
                  <DatabaseActions
                    project={project}
                    allocation={item}
                    inventory={inventory}
                    admin={admin}
                    operate={operate}
                    fresh={Boolean(observed && !stale)}
                    reconnectable={environments.some(
                      (environment) => environment.id === item.environmentId,
                    )}
                  />
                </article>
              );
            })}
          </div>
        )}
      </div>
      {admin && (
        <div className="panel">
          <div className="panel-toolbar">
            <div>
              <h2 className="font-semibold">Database provisioner</h2>
              <p className="muted text-sm">
                Independent enrollment and authority for approved PostgreSQL
                profiles.
              </p>
            </div>
          </div>
          <div className="p-5">
            <ClusterManagement cluster={inventory.agent} database />
          </div>
        </div>
      )}
      <p className="muted text-xs">
        Removing a preview retains its database and disconnects consumers.
        Application rollback does not reverse migrations. An isolated restore
        requires data verification before an operator plans any cutover.
      </p>
    </section>
  );
}
