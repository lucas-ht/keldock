"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import type { GitHubStatus } from "@/lib/github";
import { mutate } from "@/lib/client-api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ErrorNotice, EmptyState } from "@/components/shared/feedback";

export function GitHubRepositories({ status }: { status: GitHubStatus }) {
  const router = useRouter(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null);
  async function action(path: string) {
    setBusy(true);
    setError(null);
    try {
      await mutate(path, { expectedVersion: status.app.version });
      router.refresh();
    } catch (error) {
      setError((error as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <section>
        <div className="section-heading">
          <div>
            <h2>Repository delegation</h2>
            <p>
              Stable repository IDs keep authorization intact across renames.
              Transfers require an approved owner.
            </p>
          </div>
        </div>
        <div className="panel divide-y">
          {status.installations.length === 0 ? (
            <EmptyState title="No GitHub installation found">
              Choose repositories after connecting your App.
            </EmptyState>
          ) : (
            status.installations.map((installation) => (
              <div className="p-5" key={installation.id}>
                <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                  <div className="flex items-center gap-3">
                    <h3 className="font-medium">{installation.login}</h3>
                    <Badge variant="secondary">
                      {installation.blocked
                        ? "Disconnected locally"
                        : installation.active
                          ? "Active"
                          : "Unavailable on GitHub"}
                    </Badge>
                  </div>
                  <Button
                    variant="outline"
                    disabled={busy}
                    onClick={() =>
                      action(
                        `/v1/github/installations/${installation.id}/${installation.blocked ? "connect" : "disconnect"}`,
                      )
                    }
                  >
                    {installation.blocked
                      ? "Reconnect locally"
                      : "Disconnect locally"}
                  </Button>
                </div>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Repository</TableHead>
                      <TableHead className="hidden sm:table-cell">
                        Project
                      </TableHead>
                      <TableHead>Delivery access</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {status.repositories
                      .filter((repo) => repo.installationId === installation.id)
                      .map((repo) => (
                        <TableRow key={repo.id}>
                          <TableCell className="whitespace-normal">
                            <a
                              href={`https://github.com/${repo.fullName}`}
                              target="_blank"
                              rel="noreferrer"
                              className="break-all"
                            >
                              {repo.fullName} ↗
                            </a>
                            <span className="block text-xs text-muted-foreground">
                              ID {repo.id}
                            </span>
                          </TableCell>
                          <TableCell className="hidden sm:table-cell">
                            {repo.projectId || "Not assigned"}
                          </TableCell>
                          <TableCell>
                            {repo.active &&
                            repo.projectId &&
                            !installation.blocked
                              ? "Authorized"
                              : "Denied"}
                          </TableCell>
                        </TableRow>
                      ))}
                  </TableBody>
                </Table>
              </div>
            ))
          )}
        </div>
      </section>
      <section className="panel flex flex-col gap-4 p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-lg font-semibold">Delivery recovery</h2>
          <Button
            variant="outline"
            disabled={busy || !status.app.enabled}
            onClick={() => action("/v1/github/retry")}
          >
            Retry failed delivery
          </Button>
        </div>
        <dl className="grid grid-cols-2 gap-5 sm:grid-cols-4">
          {[
            ["Pending events", status.pendingEvents],
            ["Failed events", status.failedEvents],
            ["Pending reports", status.pendingFeedback],
            ["Failed reports", status.failedFeedback],
          ].map(([label, value]) => (
            <div key={label}>
              <dt className="muted text-xs">{label}</dt>
              <dd className="mt-1 text-xl font-semibold">{value}</dd>
            </div>
          ))}
        </dl>
        <p className="muted text-xs">
          Accepted deployments continue if the CI runner stops. GitHub reporting
          retries independently and never changes deployment admission.
        </p>
        <ErrorNotice message={error} />
      </section>
    </>
  );
}
