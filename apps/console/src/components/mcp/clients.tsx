"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import type { MCPClient } from "@/lib/contracts";
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
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from "@/components/ui/dialog";
import { EmptyState, ErrorNotice } from "@/components/shared/feedback";
import { Status } from "@/components/shared/status";

export function Clients({
  items,
  enabled,
}: {
  items: MCPClient[];
  enabled: boolean;
}) {
  const router = useRouter();
  const [form, setForm] = useState(false),
    [selected, setSelected] = useState<MCPClient | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<MCPClient | null>(null);
  return (
    <div className="flex flex-col gap-5">
      <div className="section-heading">
        <div>
          <h2>Registered clients</h2>
          <p>
            Public registration metadata. A client still needs a person’s
            consent to read any project.
          </p>
        </div>
        <Button
          variant="outline"
          disabled={!enabled}
          onClick={() => {
            setForm(!form);
            setError(null);
          }}
        >
          {form ? "Close registration form" : "Register client"}
        </Button>
      </div>
      <ErrorNotice message={error} />
      {form && (
        <form
          className="form-section panel p-5"
          onSubmit={async (event) => {
            event.preventDefault();
            const data = new FormData(event.currentTarget);
            setBusy(true);
            setError(null);
            try {
              const client = await mutate<MCPClient>("/v1/mcp/clients", {
                name: data.get("name"),
                redirectUris: [data.get("redirect")],
              });
              setCreated(client);
              setForm(false);
              router.refresh();
            } catch (error) {
              setError((error as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="client-name">Client name</FieldLabel>
              <Input id="client-name" name="name" maxLength={80} required />
            </Field>
            <Field>
              <FieldLabel htmlFor="redirect">Exact callback URL</FieldLabel>
              <Input
                id="redirect"
                name="redirect"
                type="url"
                placeholder="http://127.0.0.1:3001/callback"
                required
              />
              <FieldDescription>
                HTTPS, or HTTP on a native client’s loopback address. No
                wildcard callbacks.
              </FieldDescription>
            </Field>
            <Button type="submit" disabled={busy}>
              {busy ? "Registering" : "Register public PKCE client"}
            </Button>
          </FieldGroup>
        </form>
      )}
      {created && (
        <p className="muted break-all">
          Registered client ID: <code>{created.id}</code>
        </p>
      )}
      <div className="panel data-list">
        {items.length ? (
          items.map((client) => (
            <div className="data-row" key={client.id}>
              <div className="min-w-0">
                <h3 className="font-medium">{client.name}</h3>
                <p className="font-mono text-xs">{client.id}</p>
                {client.redirectUris.map((uri) => (
                  <p key={uri}>{uri}</p>
                ))}
              </div>
              {client.disabledAt ? (
                <Status state="disabled" />
              ) : (
                <Button variant="outline" onClick={() => setSelected(client)}>
                  Disable client
                </Button>
              )}
            </div>
          ))
        ) : (
          <EmptyState title="No clients registered">
            Clients can register during OAuth connection, or an administrator
            can register an exact callback URL here.
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
            <DialogTitle>Disable {selected?.name}?</DialogTitle>
            <DialogDescription>
              All connections for this client will be revoked. The client cannot
              authorize again with this registration.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose render={<Button variant="outline" disabled={busy} />}>
              Keep client
            </DialogClose>
            <Button
              disabled={busy}
              onClick={async () => {
                if (!selected) return;
                setBusy(true);
                setError(null);
                try {
                  await mutate(`/v1/mcp/clients/${selected.id}/disable`);
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
              Disable client
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
