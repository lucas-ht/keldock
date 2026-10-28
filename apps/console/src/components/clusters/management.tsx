"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { KeyRound, Unplug } from "lucide-react";
import type { Cluster } from "@/lib/contracts";
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
import { ErrorNotice } from "@/components/shared/feedback";
import { SecretOutput } from "@/components/shared/secret";

type Enrollment = {
  apiUrl: string;
  enrollment: { clusterId: string; token: string; expiresAt: string };
};
export function ClusterManagement({
  cluster,
  cloud = false,
  database = false,
}: {
  cluster: Cluster;
  cloud?: boolean;
  database?: boolean;
}) {
  const root = cloud ? "/v1/cloud" : database ? "/v1/database" : "/v1";
  const router = useRouter();
  const [enrollment, setEnrollment] = useState<Enrollment | null>(null),
    [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false),
    [action, setAction] = useState<"enroll" | "revoke" | null>(null);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" onClick={() => setAction("enroll")}>
          <KeyRound data-icon="inline-start" />
          {cluster.agentId ? "Replace enrollment" : "Enroll agent"}
        </Button>
        {cluster.state !== "revoked" && cluster.agentId && (
          <Button variant="destructive" onClick={() => setAction("revoke")}>
            <Unplug data-icon="inline-start" />
            Revoke agent
          </Button>
        )}
      </div>
      <ErrorNotice message={error} />
      {enrollment && (
        <div className="flex max-w-xl flex-col gap-5">
          <p className="muted">
            This grant is bound to{" "}
            <strong>{enrollment.enrollment.clusterId}</strong>, expires{" "}
            {date(enrollment.enrollment.expiresAt)}, and can be used once. Save
            it in an owner-only token file on the cluster operator’s machine.
          </p>
          <SecretOutput
            label="Enrollment token"
            value={enrollment.enrollment.token}
          />
          <p className="muted">
            Set <code>apiUrl</code> to <code>{enrollment.apiUrl}</code> and{" "}
            <code>credentialFile</code> in the private agent configuration, then
            run:
          </p>
          <pre className="secret-output">
            keldock enroll {cloud ? "--cloud " : database ? "--database " : ""}
            --config {database ? "database-agent.json" : "agent.json"}
            --cluster {cluster.id} --token-file enrollment.token
          </pre>
          <p className="muted">
            Start the enrolled agent with{" "}
            <code>
              keldock{" "}
              {cloud ? "cloud-agent" : database ? "database-agent" : "agent"}{" "}
              --config {database ? "database-agent.json" : "agent.json"}
            </code>
            . Existing workloads retain their last approved state during
            reconnection.
          </p>
        </div>
      )}
      <Dialog
        open={action !== null}
        onOpenChange={(open) => {
          if (!open && !busy) setAction(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {action === "enroll"
                ? "Create a new enrollment grant?"
                : "Revoke this cluster agent?"}
            </DialogTitle>
            <DialogDescription>
              {action === "enroll"
                ? "This invalidates unused enrollment grants. The existing agent is replaced only when the new grant is exchanged."
                : cloud
                  ? "The cloud agent identity and unused grants will stop working. AWS controllers retain their last approved state; AWS role revocation is a separate operator action."
                  : "The current identity and unused enrollment grants will stop working. Running workloads are retained; a new enrollment is required to resume control."}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose render={<Button variant="outline" disabled={busy} />}>
              Keep current state
            </DialogClose>
            <Button
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                setError(null);
                try {
                  if (action === "enroll")
                    setEnrollment(
                      await mutate(`${root}/clusters/${cluster.id}/enrollment`),
                    );
                  else {
                    await mutate(`${root}/clusters/${cluster.id}/revoke`);
                    setEnrollment(null);
                  }
                  setAction(null);
                  router.refresh();
                } catch (error) {
                  setError((error as Error).message);
                  setAction(null);
                } finally {
                  setBusy(false);
                }
              }}
            >
              {busy
                ? "Saving"
                : action === "enroll"
                  ? "Create enrollment grant"
                  : "Revoke agent"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
