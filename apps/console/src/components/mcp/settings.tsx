"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import type { MCPSettings } from "@/lib/contracts";
import { mutate } from "@/lib/client-api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from "@/components/ui/dialog";
import { ErrorNotice } from "@/components/shared/feedback";

export function EndpointSettings({
  settings,
  admin,
}: {
  settings: MCPSettings;
  admin: boolean;
}) {
  const router = useRouter();
  const [confirm, setConfirm] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null);
  async function change(enabled: boolean) {
    setBusy(true);
    setError(null);
    try {
      await mutate(
        "/v1/mcp/settings",
        { enabled, version: settings.version },
        "PATCH",
      );
      setConfirm(false);
      router.refresh();
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
          <h2 className="text-lg font-semibold">Read tools</h2>
          <Badge variant="secondary">
            {settings.enabled ? "Enabled" : "Disabled"}
          </Badge>
        </div>
        {admin && (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => (settings.enabled ? setConfirm(true) : change(true))}
          >
            {busy
              ? "Saving"
              : settings.enabled
                ? "Disable read tools"
                : "Enable read tools"}
          </Button>
        )}
      </div>
      <p className="muted max-w-2xl">
        Connect an MCP client to inspect projects, environments, operations, and
        redacted diagnostics. Each connection uses OAuth and only the projects
        its owner explicitly allows.
      </p>
      <code className="secret-output">{settings.endpoint}</code>
      <p className="muted text-xs">
        Configure this endpoint in your MCP client, then sign in and approve its
        project access. No token needs to be copied from this screen.
      </p>
      <ErrorNotice message={error} />
      <Dialog open={confirm} onOpenChange={setConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Disable MCP read tools?</DialogTitle>
            <DialogDescription>
              Every existing connection and pending authorization will be
              revoked. Re-enabling the endpoint will require clients to obtain
              fresh consent.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose render={<Button variant="outline" disabled={busy} />}>
              Keep enabled
            </DialogClose>
            <Button disabled={busy} onClick={() => change(false)}>
              Disable and revoke access
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
