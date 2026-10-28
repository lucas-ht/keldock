"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { mutate } from "@/lib/client-api";
import type {
  DatabaseAllocation,
  DatabaseInventory,
} from "@/lib/database-contracts";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { ErrorNotice } from "@/components/shared/feedback";

type Action = "retain" | "reconnect" | "backup" | "restore" | "pause-backup";
const labels: Record<Action, string> = {
  retain: "Disconnect consumers",
  reconnect: "Reconnect retained data",
  backup: "Request base backup",
  restore: "Create isolated restore",
  "pause-backup": "Pause backups",
};
const descriptions: Record<Action, string> = {
  retain:
    "Remove the component bindings and deny new database connections. Data and its namespace remain retained. Existing sessions end as the consumer workloads roll out.",
  reconnect:
    "Explicitly reconnect this retained database to the current release’s declared components. This reuses existing data and records a new deployment using the same admitted images.",
  backup:
    "Create a new base backup using the operator-approved destination and its scoped writer. Existing archives remain retained. WAL protection is shown only after archive evidence is observed.",
  restore:
    "Create a separate PostgreSQL cluster using the read-only restore identity. The source stays in place. No application binding is created; verify recovered data before making any cutover decision.",
  "pause-backup":
    "Suspend scheduled backups and disconnect WAL archiving. Existing archives remain retained. New writes will no longer be protected by this integration.",
};

export function DatabaseActions({
  project,
  allocation,
  inventory,
  admin,
  operate,
  fresh,
  reconnectable,
}: {
  project: string;
  allocation: DatabaseAllocation;
  inventory: DatabaseInventory;
  admin: boolean;
  operate: boolean;
  fresh: boolean;
  reconnectable: boolean;
}) {
  const router = useRouter();
  const [action, setAction] = useState<Action | null>(null);
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null);
  const destinations = inventory.configuration.destinations.filter(
    (item) => item.allocationId === allocation.id,
  );
  function choose(next: Action) {
    setKey(crypto.randomUUID());
    setError(null);
    setAction(next);
  }
  const backup = allocation.observation?.backup;
  return (
    <>
      <div className="flex flex-wrap gap-2">
        {operate && allocation.action === "reconcile" && (
          <Button size="sm" variant="outline" onClick={() => choose("retain")}>
            Disconnect consumers
          </Button>
        )}
        {admin && allocation.action === "retain" && reconnectable && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => choose("reconnect")}
          >
            Reconnect
          </Button>
        )}
        {admin && destinations.length > 0 && (
          <Button size="sm" variant="outline" onClick={() => choose("backup")}>
            {allocation.backup ? "New base backup" : "Enable backups"}
          </Button>
        )}
        {admin &&
          fresh &&
          backup?.state === "ready" &&
          !allocation.backup?.paused && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => choose("restore")}
            >
              Isolated restore
            </Button>
          )}
        {admin && allocation.backup && !allocation.backup.paused && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => choose("pause-backup")}
          >
            Pause backups
          </Button>
        )}
      </div>
      <Dialog
        open={action !== null}
        onOpenChange={(open) => {
          if (!open && !busy) setAction(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{action && labels[action]}?</DialogTitle>
            <DialogDescription>
              {action && descriptions[action]}
            </DialogDescription>
          </DialogHeader>
          <dl className="detail-list">
            <dt>Database</dt>
            <dd>{allocation.alias}</dd>
            <dt>Current generation</dt>
            <dd>{allocation.generation}</dd>
            <dt>Environment</dt>
            <dd className="break-all">{allocation.environmentId}</dd>
          </dl>
          <ErrorNotice message={error} />
          <DialogFooter>
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => setAction(null)}
            >
              Keep current state
            </Button>
            <Button
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                setError(null);
                try {
                  await mutate(
                    `/v1/projects/${encodeURIComponent(project)}/databases`,
                    {
                      id: allocation.id,
                      generation: allocation.generation,
                      action,
                      ...(action === "backup" && !allocation.backup
                        ? { destinationId: destinations[0]?.id }
                        : {}),
                    },
                    "POST",
                    { "Idempotency-Key": key },
                  );
                  setAction(null);
                  router.refresh();
                } catch (error) {
                  setError((error as Error).message);
                } finally {
                  setBusy(false);
                }
              }}
            >
              {busy ? "Saving request" : action && labels[action]}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
