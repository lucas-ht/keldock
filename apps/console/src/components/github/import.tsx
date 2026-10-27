"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import type { GitHubStatus } from "@/lib/github";
import { mutate } from "@/lib/client-api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldDescription,
} from "@/components/ui/field";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogHeader,
  DialogFooter,
} from "@/components/ui/dialog";
import { ErrorNotice } from "@/components/shared/feedback";

export function ImportApp({ status }: { status: GitHubStatus }) {
  const router = useRouter();
  const [open, setOpen] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null);
  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const fields = new FormData(event.currentTarget),
        file = fields.get("key") as File;
      if (!file?.size || file.size > 16384)
        throw new Error("Choose a private-key PEM file smaller than 16 KiB.");
      await mutate("/v1/github/app", {
        expectedVersion: status.app.version,
        app: {
          appId: Number(fields.get("appId")),
          clientId: fields.get("clientId"),
          slug: fields.get("slug"),
          privateKey: await file.text(),
          webhookSecret: fields.get("secret"),
        },
      });
      setOpen(false);
      router.refresh();
    } catch (error) {
      setError((error as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <Button
        variant="outline"
        onClick={() => {
          setError(null);
          setOpen(true);
        }}
      >
        {status.app.enabled ? "Rotate App credentials" : "Use an existing App"}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {status.app.enabled
                ? "Rotate App credentials"
                : "Connect an existing App"}
            </DialogTitle>
            <DialogDescription>
              The private key and webhook secret are stored only by this
              installation. The same App identity is required for rotation.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={submit} className="flex flex-col gap-5">
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="github-app-id">App ID</FieldLabel>
                <Input
                  id="github-app-id"
                  name="appId"
                  type="number"
                  min="1"
                  required
                  defaultValue={status.app.appId || ""}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="github-client-id">Client ID</FieldLabel>
                <Input
                  id="github-client-id"
                  name="clientId"
                  required
                  defaultValue={status.app.clientId}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="github-slug">App slug</FieldLabel>
                <Input
                  id="github-slug"
                  name="slug"
                  required
                  defaultValue={status.app.slug}
                />
                <FieldDescription>
                  The final part of github.com/apps/your-app.
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="github-private-key">
                  Private-key PEM
                </FieldLabel>
                <Input
                  id="github-private-key"
                  name="key"
                  type="file"
                  accept=".pem"
                  required
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="github-webhook-secret">
                  Webhook secret
                </FieldLabel>
                <Input
                  id="github-webhook-secret"
                  name="secret"
                  type="password"
                  autoComplete="new-password"
                  minLength={32}
                  maxLength={256}
                  required
                />
                <FieldDescription>
                  Use the secret configured on your App. You can update the
                  webhook after connecting.
                </FieldDescription>
              </Field>
            </FieldGroup>
            <ErrorNotice message={error} />
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={() => setOpen(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={busy}>
                {busy ? "Verifying App" : "Verify and save"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}
