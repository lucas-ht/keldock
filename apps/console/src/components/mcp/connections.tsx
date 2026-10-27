"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import type { Connection } from "@/lib/contracts";
import { mutate } from "@/lib/client-api";
import { date } from "@/lib/format";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from "@/components/ui/dialog";
import { EmptyState, ErrorNotice } from "@/components/shared/feedback";
import { Status } from "@/components/shared/status";

export function Connections({ items }: { items: Connection[] }) {
  const router = useRouter();
  const [selected, setSelected] = useState<Connection | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null);
  return (
    <>
      <ErrorNotice message={error} />
      <div className="panel data-list">
        {items.length ? (
          items.map((connection) => (
            <div className="data-row" key={connection.id}>
              <div className="min-w-0">
                <h3 className="font-medium">{connection.clientName}</h3>
                <p>
                  {connection.userName} · {connection.projects.join(", ")}
                </p>
                <p>
                  Last used {date(connection.lastUsedAt)} · Expires{" "}
                  {date(connection.expiresAt)}
                </p>
              </div>
              {connection.revokedAt ? (
                <Status state="revoked" />
              ) : (
                <Button
                  variant="outline"
                  onClick={() => setSelected(connection)}
                >
                  Revoke connection
                </Button>
              )}
            </div>
          ))
        ) : (
          <EmptyState title="No MCP connections yet">
            Connections appear after a client completes authorization. Access is
            always read-only and project scoped.
          </EmptyState>
        )}
      </div>
      <Dialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open && !busy) setSelected(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Revoke {selected?.clientName}?</DialogTitle>
            <DialogDescription>
              This connection’s access and refresh credentials will stop working
              immediately. Other connections are unaffected.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose render={<Button variant="outline" disabled={busy} />}>
              Keep connection
            </DialogClose>
            <Button
              disabled={busy}
              onClick={async () => {
                if (!selected) return;
                setBusy(true);
                setError(null);
                try {
                  await mutate(`/v1/mcp/connections/${selected.id}/revoke`);
                  setSelected(null);
                  router.refresh();
                } catch (error) {
                  setError((error as Error).message);
                  setSelected(null);
                } finally {
                  setBusy(false);
                }
              }}
            >
              {busy ? "Revoking" : "Revoke connection"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
