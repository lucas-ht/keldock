"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import type { GitHubStatus, GitHubSetup } from "@/lib/github";
import { mutate } from "@/lib/client-api";
import { date } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogHeader,
  DialogFooter,
} from "@/components/ui/dialog";
import { ErrorNotice } from "@/components/shared/feedback";
import { ImportApp } from "./import";

export function GitHubConnection({ status }: { status: GitHubStatus }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null),
    [message, setMessage] = useState<string | null>(null),
    [confirm, setConfirm] = useState(false),
    [handoff, setHandoff] = useState<GitHubSetup | null>(null);
  async function action(kind: string) {
    setBusy(true);
    setError(null);
    setMessage(null);
    try {
      if (kind === "manifest" || kind === "installation") {
        setHandoff(
          await mutate<GitHubSetup>("/v1/github/setup", {
            expectedVersion: status.app.version,
            purpose: kind,
          }),
        );
      } else {
        await mutate(
          kind === "disconnect" ? "/v1/github/app" : `/v1/github/${kind}`,
          { expectedVersion: status.app.version },
          kind === "disconnect" ? "DELETE" : "POST",
        );
        setMessage(
          kind === "sync"
            ? "Repository selection synchronized."
            : kind === "webhook"
              ? "Signed webhook configured."
              : "App disconnected from this installation.",
        );
        setConfirm(false);
        router.refresh();
      }
    } catch (error) {
      setError((error as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="panel flex flex-col gap-5 p-5">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold">GitHub App</h2>
          <Badge variant="secondary">
            {status.app.enabled ? "Connected" : "Not connected"}
          </Badge>
        </div>
        {status.app.enabled && (
          <a
            href={`https://github.com/settings/apps/${encodeURIComponent(status.app.slug)}`}
            target="_blank"
            rel="noreferrer"
          >
            App settings ↗
          </a>
        )}
      </div>
      <p className="muted max-w-2xl">
        The App reads selected repositories and workflow state, then publishes
        deployment Checks, Deployments, and one updated PR comment. GitHub
        always controls which repositories it can access.
      </p>
      {status.app.enabled && (
        <dl className="grid gap-3 text-sm sm:grid-cols-2">
          <div>
            <dt className="muted">App</dt>
            <dd className="font-medium">{status.app.slug}</dd>
          </div>
          <div>
            <dt className="muted">Last synchronized</dt>
            <dd>
              {status.app.syncedAt
                ? date(status.app.syncedAt)
                : "Awaiting synchronization"}
            </dd>
          </div>
        </dl>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          disabled={busy}
          onClick={() =>
            action(status.app.enabled ? "installation" : "manifest")
          }
        >
          {status.app.enabled
            ? "Choose repositories"
            : "Create an installation-owned App"}
        </Button>
        <ImportApp status={status} />
        {status.app.enabled && (
          <>
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => action("sync")}
            >
              Sync now
            </Button>
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => action("webhook")}
            >
              Update webhook
            </Button>
            <Button
              variant="ghost"
              disabled={busy}
              onClick={() => setConfirm(true)}
            >
              Disconnect App
            </Button>
          </>
        )}
      </div>
      {handoff && (
        <Alert>
          <AlertDescription>
            <p>
              Continue on GitHub to review the requested access. This setup link
              expires in 10 minutes.
            </p>
            {handoff.manifest ? (
              <form action={handoff.url} method="post">
                <input
                  type="hidden"
                  name="manifest"
                  value={JSON.stringify(handoff.manifest)}
                />
                <Button type="submit" className="mt-3">
                  Review App registration on GitHub ↗
                </Button>
              </form>
            ) : (
              <a href={handoff.url} className="mt-2 inline-block">
                Review selected repositories on GitHub ↗
              </a>
            )}
          </AlertDescription>
        </Alert>
      )}
      {status.app.syncError && (
        <Alert>
          <AlertDescription>{status.app.syncError}</AlertDescription>
        </Alert>
      )}
      {message && (
        <p role="status" className="text-sm">
          {message}
        </p>
      )}
      <ErrorNotice message={error} />
      <Separator />
      <div className="flex flex-col gap-2 text-xs text-muted-foreground">
        <p>
          Required access: Contents, Actions and Metadata read; Checks,
          Deployments and Pull requests write.
        </p>
        <p>
          Webhook: <code className="break-all">{status.webhookUrl}</code>
        </p>
        <p>
          Private GHCR pulls use a separate registry reader credential. The App
          does not grant package access.
        </p>
      </div>
      <Dialog open={confirm} onOpenChange={setConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Disconnect this App?</DialogTitle>
            <DialogDescription>
              New CI submissions and GitHub feedback will be denied. Running
              workloads are retained. The App registration on GitHub remains
              yours.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => setConfirm(false)}
            >
              Keep connected
            </Button>
            <Button disabled={busy} onClick={() => action("disconnect")}>
              Disconnect App
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
