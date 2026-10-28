"use client";
import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { LoaderCircle, RotateCcw, Square } from "lucide-react";
import type { Environment } from "@/lib/contracts";
import { mutate } from "@/lib/client-api";
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

export function DeploymentActions({
  environment,
  permitted,
}: {
  environment: Environment;
  permitted: boolean;
}) {
  const router = useRouter();
  const [action, setAction] = useState<"retry" | "cancel" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const key = useRef("");
  const intent = useRef("");
  const [target, setTarget] = useState<{
    projectId: string;
    operationId: string;
    generation: number;
  } | null>(null);
  function begin(next: "retry" | "cancel") {
    const identity = `${next}:${environment.operation.id}:${environment.generation}`;
    if (identity !== intent.current) {
      key.current = crypto.randomUUID();
      intent.current = identity;
    }
    setTarget({
      projectId: environment.projectId,
      operationId: environment.operation.id,
      generation: environment.generation,
    });
    setError(null);
    setAction(next);
  }
  const state = environment.operation.state;
  if (
    !permitted ||
    environment.kind === "production" ||
    environment.deleted ||
    environment.operation.kind !== "deploy"
  )
    return null;
  const retry = ["failed", "canceled"].includes(state);
  const cancel = ["pending", "publishing", "reconciling", "failed"].includes(
    state,
  );
  if (!retry && !cancel) return null;
  return (
    <>
      <div className="flex flex-wrap gap-2">
        {retry && (
          <Button variant="outline" onClick={() => begin("retry")}>
            <RotateCcw data-icon="inline-start" />
            Retry release
          </Button>
        )}
        {cancel && (
          <Button variant="outline" onClick={() => begin("cancel")}>
            <Square data-icon="inline-start" />
            Cancel
          </Button>
        )}
      </div>
      <Dialog
        open={action !== null}
        onOpenChange={(open) => {
          if (!busy && !open) setAction(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {action === "retry"
                ? "Retry this admitted release?"
                : "Cancel this operation?"}
            </DialogTitle>
            <DialogDescription>
              {action === "retry"
                ? "Keldock will select the same approved images and configuration in a new generation. No build or CI run will be created."
                : "Future dispatch will stop. Completed effects and running workloads are retained; canceling does not roll back a deployment."}
            </DialogDescription>
          </DialogHeader>
          <ErrorNotice message={error} />
          <DialogFooter>
            <DialogClose render={<Button variant="outline" disabled={busy} />}>
              Keep current state
            </DialogClose>
            <Button
              disabled={busy}
              onClick={async () => {
                if (!action || !target) return;
                setBusy(true);
                setError(null);
                try {
                  await mutate(
                    `/v1/projects/${target.projectId}/operations/${target.operationId}/${action}`,
                    { generation: target.generation },
                    "POST",
                    { "Idempotency-Key": key.current },
                  );
                  setAction(null);
                  key.current = "";
                  intent.current = "";
                  router.refresh();
                } catch (error) {
                  setError((error as Error).message);
                } finally {
                  setBusy(false);
                }
              }}
            >
              {busy ? (
                <LoaderCircle
                  data-icon="inline-start"
                  className="animate-spin"
                />
              ) : null}
              {action === "retry" ? "Retry release" : "Cancel operation"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
