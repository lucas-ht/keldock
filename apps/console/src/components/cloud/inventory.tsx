"use client";
import { useState } from "react";
import type {
  CloudEnvironment,
  CloudIntent,
  CloudInventory,
} from "@/lib/cloud-contracts";
import { cloudKinds } from "@/lib/cloud-contracts";
import { CloudState, cloudReason } from "./status";
import { CloudFacts } from "./details";
import { CloudActions } from "./actions";
import { ConnectionForm } from "./forms/connection";
import { StorageForm } from "./forms/storage";
import { BindingForm } from "./forms/binding";
import { BackupForm } from "./forms/backup";
import { DomainForm } from "./forms/domain";
import { EmptyState } from "@/components/shared/feedback";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableHeader,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
} from "@/components/ui/table";

function title(item: CloudIntent) {
  return item.domain?.request.hostname ?? item.id;
}
function kind(item: CloudIntent) {
  return item.resource
    ? cloudKinds[item.resource.kind]
    : item.kind === "binding"
      ? "Consumer grant"
      : item.kind === "backup"
        ? "Database backup"
        : "Domain";
}

export function CloudInventoryView({
  project,
  inventory,
  environments,
  admin,
  operate,
  now,
}: {
  project: string;
  inventory: CloudInventory;
  environments: CloudEnvironment[];
  admin: boolean;
  operate: boolean;
  now: number;
}) {
  const { items, configuration } = inventory;
  const connections = items.filter((item) => item.connection);
  const usable = connections.filter(
    (item) =>
      item.connection?.enabled &&
      item.action === "reconcile" &&
      item.state === "ready" &&
      item.observation &&
      now - Date.parse(item.observation.observedAt) <= 120_000,
  );
  const resources = items.filter((item) => !item.connection);
  const [selected, setSelected] = useState(resources[0]?.id ?? "");
  const current =
    resources.find((item) => item.id === selected) ?? resources[0];
  const bound =
    current &&
    items.some(
      (item) =>
        item.action !== "disconnected" &&
        item.action !== "disconnect" &&
        (item.binding?.resourceId === current.id ||
          item.backup?.resourceId === current.id),
    );
  const ready = current?.state === "ready" && current.action === "reconcile";
  return (
    <div className="flex flex-col gap-7">
      <section className="panel" aria-label="Cloud connections">
        <div className="panel-toolbar">
          <h2 className="font-semibold">Approved connections</h2>
          {admin && (
            <ConnectionForm project={project} catalog={configuration} />
          )}
        </div>
        {connections.length ? (
          <div className="data-list">
            {connections.map((item) => (
              <div className="data-row" key={item.id}>
                <div>
                  <strong>{item.id}</strong>
                  <p>
                    {item.connection?.account} · {item.connection?.region}
                  </p>
                  <p>{cloudReason(item)}</p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <CloudState item={item} now={now} />
                  {admin && (
                    <ConnectionForm
                      project={project}
                      catalog={configuration}
                      current={item}
                    />
                  )}{" "}
                  {admin && (
                    <CloudActions project={project} item={item} admin />
                  )}
                </div>
              </div>
            ))}
          </div>
        ) : (
          <EmptyState title="No cloud account connected">
            An administrator can select an account approved by the installation
            operator.
          </EmptyState>
        )}
      </section>
      <section
        className="panel"
        aria-label="Retained resources and consumer grants"
      >
        <div className="panel-toolbar">
          <div>
            <h2 className="font-semibold">Resources and access</h2>
            <p className="muted text-xs">
              Storage is retained independently of environments.
            </p>
          </div>
          {operate && (
            <div className="flex flex-wrap gap-2">
              <StorageForm
                project={project}
                catalog={configuration}
                connections={usable}
              />
              <DomainForm
                project={project}
                catalog={configuration}
                connections={usable}
                environments={environments}
              />
            </div>
          )}
        </div>
        {resources.length ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Resource</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Revision</TableHead>
                <TableHead>
                  <span className="sr-only">Inspect</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {resources.map((item) => (
                <TableRow
                  key={item.id}
                  data-selected={item.id === current?.id || undefined}
                >
                  <TableCell className="max-w-72 whitespace-normal">
                    <strong className="break-words">{title(item)}</strong>
                    {item.resource && (
                      <p className="muted mt-1 break-all text-xs">
                        {item.resource.profile}
                      </p>
                    )}
                  </TableCell>
                  <TableCell>{kind(item)}</TableCell>
                  <TableCell>
                    <CloudState item={item} now={now} />
                  </TableCell>
                  <TableCell>{item.generation}</TableCell>
                  <TableCell>
                    <Button
                      size="sm"
                      variant={item.id === current?.id ? "secondary" : "ghost"}
                      onClick={() => setSelected(item.id)}
                      aria-label={`Inspect ${title(item)}`}
                    >
                      Inspect
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <EmptyState title="No retained resources yet">
            Verify a connection, then add storage or claim a domain for a
            deployed environment.
          </EmptyState>
        )}
        {!usable.length && (
          <div className="panel-footer">
            Resource creation becomes available after a connection’s identities
            are verified.
          </div>
        )}
      </section>
      {current && (
        <section
          className="panel"
          aria-label={`Resource details: ${title(current)}`}
        >
          <div className="panel-toolbar">
            <div>
              <h2 className="font-semibold break-all">{title(current)}</h2>
              <p className="muted text-sm">{cloudReason(current)}</p>
            </div>
            <CloudState item={current} now={now} />
          </div>
          <div className="flex flex-col gap-5 p-5">
            <CloudFacts item={current} project={project} />
            {operate && (
              <div className="flex flex-wrap gap-2">
                {ready && current.resource?.kind === "object-storage" && (
                  <BindingForm
                    project={project}
                    resource={current}
                    environments={environments}
                  />
                )}
                {ready && current.resource?.kind === "backup-store" && (
                  <BackupForm
                    project={project}
                    resource={current}
                    catalog={configuration}
                  />
                )}
              </div>
            )}
            {operate && (
              <CloudActions
                key={`${current.id}-${current.generation}`}
                project={project}
                item={current}
                admin={admin}
                bound={!!bound}
              />
            )}
            {current.resource?.kind === "object-storage" &&
              !environments.length && (
                <p className="muted text-sm">
                  Deploy an environment before granting component access.
                </p>
              )}
          </div>
        </section>
      )}
    </div>
  );
}
