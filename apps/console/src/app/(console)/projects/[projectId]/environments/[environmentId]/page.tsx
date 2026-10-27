import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { api, requireSession } from "@/lib/server-api";
import type { Diagnostics, Environment, Event, Page } from "@/lib/contracts";
import {
  canOperate,
  date,
  environmentName,
  componentState,
} from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { Status } from "@/components/shared/status";
import { ComponentStrip } from "@/components/deployments/components";
import { DeploymentActions } from "@/components/deployments/actions";
import { Refresh } from "@/components/layout/refresh";
import { EmptyState } from "@/components/shared/feedback";

const eventNames: Record<string, string> = {
  "release.admitted": "Release admitted",
  "operation.retried": "Release retried",
  "operation.canceled": "Operation canceled",
  "preview.deleted": "Preview deletion requested",
  "preview.expired": "Preview expired",
};
export const metadata = { title: "Environment" };

export default async function EnvironmentPage({
  params,
}: {
  params: Promise<{ projectId: string; environmentId: string }>;
}) {
  const { projectId, environmentId } = await params;
  const path = `/v1/projects/${encodeURIComponent(projectId)}/environments/${encodeURIComponent(environmentId)}`;
  const [session, environment, diagnostics, history] = await Promise.all([
    requireSession(),
    api<Environment>(path),
    api<Diagnostics>(`${path}/diagnostics`),
    api<Page<Event>>(`${path}/timeline`),
  ]);
  return (
    <>
      <Link
        className="mb-5 inline-flex items-center gap-2 text-sm text-muted-foreground"
        href={`/projects/${projectId}`}
      >
        <ArrowLeft className="size-4" />
        Project environments
      </Link>
      <div className="page-heading">
        <div>
          <h1>
            {environment.application} /{" "}
            {environmentName(environment.pullRequest)}
          </h1>
          <p>
            Generation {environment.generation} ·{" "}
            <code>
              {environment.source?.commit.slice(0, 7) ?? "No release"}
            </code>{" "}
            · <Status state={environment.operation.state} />
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <DeploymentActions
            environment={environment}
            permitted={canOperate(session.user.role)}
          />
          <Refresh automatic />
        </div>
      </div>
      <ComponentStrip environment={environment} />
      <section className="mt-8">
        <div className="section-heading">
          <h2>Selected release</h2>
        </div>
        <dl className="panel grid gap-5 p-5 sm:grid-cols-2">
          <div>
            <dt className="text-xs text-muted-foreground">Release</dt>
            <dd className="mt-2 break-all font-mono text-xs">
              {environment.releaseId || "—"}
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Operation</dt>
            <dd className="mt-2 break-all font-mono text-xs">
              {environment.operation.id}
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Source</dt>
            <dd className="mt-2 break-all">
              {environment.source?.repository ?? "—"} · CI run{" "}
              {environment.source?.runId ?? "—"}, attempt{" "}
              {environment.source?.runAttempt ?? "—"}
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Endpoint</dt>
            <dd className="mt-2 break-all font-mono text-xs">
              {environment.hostname || "No endpoint selected"}
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">
              Last operation update
            </dt>
            <dd className="mt-2">{date(environment.operation.updatedAt)}</dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">
              Latest matching observation
            </dt>
            <dd className="mt-2">
              {date(environment.observation?.observedAt)}
            </dd>
          </div>
        </dl>
      </section>
      <section id="diagnostics" className="mt-8 scroll-mt-6">
        <div className="section-heading">
          <div>
            <h2>Diagnostics</h2>
            <p>
              Bounded controller evidence for this generation. Environment
              values and credentials are redacted.
            </p>
          </div>
        </div>
        {environment.stale && (
          <Alert className="mb-4">
            <AlertTitle>Current health is unknown</AlertTitle>
            <AlertDescription>
              {environment.observation
                ? "The evidence below is historical. Check the cluster connection and refresh for a new observation."
                : "The agent has not reported this generation yet. Check its connection and the publication state."}
            </AlertDescription>
          </Alert>
        )}
        {diagnostics.publicationError && (
          <Alert variant="destructive" className="mb-4">
            <AlertTitle>Publication failed</AlertTitle>
            <AlertDescription>
              <pre className="whitespace-pre-wrap break-words">
                {diagnostics.publicationError}
              </pre>
            </AlertDescription>
          </Alert>
        )}
        <div className="panel data-list">
          {diagnostics.components.length ? (
            diagnostics.components.map((component) => (
              <div key={component.name} className="data-row">
                <div className="min-w-0 w-full">
                  <h3 className="font-semibold">
                    {component.name} · {component.reason}
                  </h3>
                  <p className="whitespace-pre-wrap break-words">
                    {component.message || "No additional controller message."}
                  </p>
                  {component.appliedRevision && (
                    <p className="break-all font-mono text-xs">
                      {component.appliedRevision}
                    </p>
                  )}
                </div>
                <Status state={componentState(component, environment.stale)} />
              </div>
            ))
          ) : (
            <EmptyState title="No component evidence yet">
              Evidence appears after the enrolled agent observes this
              generation.
            </EmptyState>
          )}
        </div>
      </section>
      <section className="mt-8">
        <div className="section-heading">
          <div>
            <h2>Selection history</h2>
            <p>Latest 50 durable events for this environment.</p>
          </div>
        </div>
        <ol className="panel data-list">
          {history.items.map((event) => (
            <li className="data-row" key={event.id}>
              <div>
                <span className="font-medium">
                  {eventNames[event.event] ?? event.event.replaceAll(".", " ")}
                </span>
                <p>
                  Generation {event.generation} · {event.actor}
                </p>
              </div>
              <time
                className="text-xs text-muted-foreground"
                dateTime={event.createdAt}
              >
                {date(event.createdAt)}
              </time>
            </li>
          ))}
        </ol>
      </section>
    </>
  );
}
