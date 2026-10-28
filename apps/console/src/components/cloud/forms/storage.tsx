"use client";
import { useId, useState } from "react";
import type { CloudCatalog, CloudIntent } from "@/lib/cloud-contracts";
import { cloudKinds } from "@/lib/cloud-contracts";
import { CloudForm } from "../request";
import { Choice } from "@/components/shared/choice";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Field,
  FieldLabel,
  FieldGroup,
  FieldDescription,
} from "@/components/ui/field";

export function StorageForm({
  project,
  catalog,
  connections,
}: {
  project: string;
  catalog: CloudCatalog;
  connections: CloudIntent[];
}) {
  const prefix = useId();
  const [name, setName] = useState("uploads"),
    [connectionId, setConnection] = useState(connections[0]?.id ?? "");
  const connection =
    connections.find((item) => item.id === connectionId) ?? connections[0];
  const profiles = catalog.profiles.filter(
    (p) =>
      p.kind !== "domain" && connection?.connection?.profiles.includes(p.id),
  );
  const [profileId, setProfile] = useState(
    profiles.find((p) => p.kind === "object-storage")?.id ??
      profiles[0]?.id ??
      "",
  );
  const profile = profiles.find((p) => p.id === profileId) ?? profiles[0];
  const [existing, setExisting] = useState(false),
    [bucket, setBucket] = useState("");
  const existingNames = profile?.existingBuckets ?? [];
  return (
    <CloudForm
      project={project}
      title="Add retained storage"
      trigger="Add storage"
      submitLabel={existing ? "Connect bucket" : "Create storage"}
      disabled={!connection || !profile}
      description="Storage survives environment and preview deletion. Connecting an existing bucket preserves its external management owner."
      command={() => ({
        kind: "resource",
        id: name,
        input: {
          connectionId: connection?.id,
          kind: profile?.kind,
          profile: profile?.id,
          mode: existing ? "connect" : "create",
          existingName: existing ? bucket || existingNames[0] : "",
        },
      })}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor={`${prefix}-name`}>Resource name</FieldLabel>
          <Input
            id={`${prefix}-name`}
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            pattern="[a-z][a-z0-9-]{0,39}"
            maxLength={40}
          />
          <FieldDescription>
            Use lowercase letters, numbers, and hyphens.
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${prefix}-connection`}>Connection</FieldLabel>
          <Choice
            id={`${prefix}-connection`}
            value={connection?.id ?? ""}
            onChange={setConnection}
            options={connections.map((item) => ({
              value: item.id,
              label: `${item.id} · ${item.connection?.region}`,
            }))}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${prefix}-profile`}>Storage profile</FieldLabel>
          <Choice
            id={`${prefix}-profile`}
            value={profile?.id ?? ""}
            onChange={(value) => {
              setProfile(value);
              setExisting(false);
              setBucket("");
            }}
            options={profiles.map((p) => ({
              value: p.id,
              label: `${cloudKinds[p.kind]} · ${p.id}`,
            }))}
          />
        </Field>
        <Field orientation="horizontal">
          <FieldLabel htmlFor={`${prefix}-existing`}>
            Connect an existing bucket
          </FieldLabel>
          <Switch
            id={`${prefix}-existing`}
            checked={existing}
            disabled={!existingNames.length}
            onCheckedChange={setExisting}
          />
        </Field>
        {existing ? (
          <Field>
            <FieldLabel htmlFor={`${prefix}-bucket`}>
              Approved bucket
            </FieldLabel>
            <Choice
              id={`${prefix}-bucket`}
              value={bucket || existingNames[0] || ""}
              onChange={setBucket}
              options={existingNames.map((value) => ({ value, label: value }))}
            />
          </Field>
        ) : (
          <FieldDescription>
            Keldock generates an account-scoped bucket name, with private
            access, versioning, and encryption.
          </FieldDescription>
        )}
      </FieldGroup>
    </CloudForm>
  );
}
