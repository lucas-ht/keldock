"use client";
import { useState } from "react";
import type { Authorization, Project } from "@/lib/contracts";
import { mutate } from "@/lib/client-api";
import { date } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { FieldGroup } from "@/components/ui/field";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { ErrorNotice } from "@/components/shared/feedback";
import { ProjectGrants } from "@/components/people/project-grants";

export function Consent({
  authorization,
  projects,
}: {
  authorization: Authorization;
  projects: Project[];
}) {
  const [selected, setSelected] = useState<string[]>([]),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null);
  async function submit(allow: boolean) {
    setBusy(true);
    setError(null);
    try {
      const response = await mutate<{ redirectUrl: string }>(
        `/v1/mcp/authorizations/${authorization.id}/consent`,
        { allow, projects: allow ? selected : [] },
      );
      window.location.assign(response.redirectUrl);
    } catch (error) {
      setError((error as Error).message);
      setBusy(false);
    }
  }
  return (
    <div className="form-section">
      <Alert>
        <AlertTitle>
          {authorization.clientName} is requesting read access
        </AlertTitle>
        <AlertDescription>
          The name is supplied by the client. Verify its callback address before
          approving.
        </AlertDescription>
      </Alert>
      <dl className="flex flex-col gap-4">
        <div>
          <dt className="text-xs text-muted-foreground">Callback address</dt>
          <dd className="mt-1 break-all font-mono text-xs">
            {authorization.redirectUri}
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Keldock resource</dt>
          <dd className="mt-1 break-all font-mono text-xs">
            {authorization.resource}
          </dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Request expires</dt>
          <dd className="mt-1">{date(authorization.expiresAt)}</dd>
        </div>
      </dl>
      <p className="muted">
        This connection can read project and environment inventory, deployment
        status, and redacted controller diagnostics. It cannot deploy, modify
        accounts, or control clusters.
      </p>
      <FieldGroup>
        <ProjectGrants
          readOnly
          projects={projects}
          selected={selected}
          onChange={setSelected}
        />
        <ErrorNotice message={error} />
        <div className="flex flex-wrap gap-3">
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => submit(false)}
          >
            Deny access
          </Button>
          <Button
            disabled={busy || selected.length === 0}
            onClick={() => submit(true)}
          >
            {busy ? "Returning to client" : "Allow selected projects"}
          </Button>
        </div>
      </FieldGroup>
    </div>
  );
}
