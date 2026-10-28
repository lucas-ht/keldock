"use client";
import { useId, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import type { CloudCommand, CloudReceipt } from "@/lib/cloud-contracts";
import { mutate } from "@/lib/client-api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogTrigger,
  DialogClose,
} from "@/components/ui/dialog";
import { FieldSet } from "@/components/ui/field";
import { ErrorNotice } from "@/components/shared/feedback";

export function useCloudRequest(project: string) {
  const router = useRouter();
  const [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null);
  const pending = useRef({ body: "", key: "" });
  async function send(command: CloudCommand) {
    if (busy) return false;
    const body = JSON.stringify(command);
    if (pending.current.body !== body)
      pending.current = { body, key: crypto.randomUUID() };
    setBusy(true);
    setError(null);
    try {
      await mutate<CloudReceipt>(
        `/v1/projects/${encodeURIComponent(project)}/cloud`,
        command,
        "POST",
        { "Idempotency-Key": pending.current.key },
      );
      router.refresh();
      pending.current = { body: "", key: "" };
      return true;
    } catch (cause) {
      setError((cause as Error).message);
      return false;
    } finally {
      setBusy(false);
    }
  }
  return { send, busy, error };
}

export function CloudForm({
  project,
  title,
  description,
  trigger,
  submitLabel,
  command,
  children,
  disabled,
  submitDisabled,
}: {
  project: string;
  title: string;
  description: string;
  trigger: string;
  submitLabel: string;
  command: () => CloudCommand;
  children: React.ReactNode;
  disabled?: boolean;
  submitDisabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const { send, busy, error } = useCloudRequest(project);
  const id = useId();
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!busy) setOpen(value);
      }}
    >
      <DialogTrigger render={<Button variant="outline" disabled={disabled} />}>
        {trigger}
      </DialogTrigger>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <form
          id={id}
          className="flex flex-col gap-5"
          onSubmit={async (event) => {
            event.preventDefault();
            if (await send(command())) setOpen(false);
          }}
        >
          <FieldSet disabled={busy}>{children}</FieldSet>
          <ErrorNotice message={error} />
          <DialogFooter>
            <DialogClose
              render={
                <Button type="button" variant="outline" disabled={busy} />
              }
            >
              Cancel
            </DialogClose>
            <Button type="submit" disabled={busy || submitDisabled}>
              {busy ? "Submitting…" : submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
