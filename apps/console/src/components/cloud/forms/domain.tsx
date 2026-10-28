"use client";
import { useId, useState } from "react";
import type {
  CloudCatalog,
  CloudEnvironment,
  CloudIntent,
} from "@/lib/cloud-contracts";
import { CloudForm } from "../request";
import { Choice } from "@/components/shared/choice";
import { Input } from "@/components/ui/input";
import {
  Field,
  FieldLabel,
  FieldGroup,
  FieldDescription,
} from "@/components/ui/field";

export function DomainForm({
  project,
  catalog,
  connections,
  environments,
}: {
  project: string;
  catalog: CloudCatalog;
  connections: CloudIntent[];
  environments: CloudEnvironment[];
}) {
  const prefix = useId();
  const candidates = environments.filter((item) =>
    item.components.some((component) => component.routed),
  );
  const [connectionId, setConnection] = useState(connections[0]?.id ?? ""),
    [environmentId, setEnvironment] = useState(candidates[0]?.id ?? ""),
    [profileId, setProfile] = useState("");
  const connection =
    connections.find((item) => item.id === connectionId) ?? connections[0];
  const environment =
    candidates.find((item) => item.id === environmentId) ?? candidates[0];
  const profiles = catalog.profiles.filter(
    (p) =>
      p.kind === "domain" && connection?.connection?.profiles.includes(p.id),
  );
  const profile = profiles.find((p) => p.id === profileId) ?? profiles[0];
  const routed =
    environment?.components.filter((component) => component.routed) ?? [];
  const [component, setComponent] = useState(""),
    [hostname, setHostname] = useState("");
  return (
    <CloudForm
      project={project}
      title="Claim a domain"
      trigger="Add domain"
      submitLabel="Claim domain"
      disabled={!environment || !profile}
      description="Keldock verifies DNS ownership before creating records. DNS convergence and certificate readiness are tracked separately."
      command={() => ({
        kind: "domain",
        input: {
          connectionId: connection?.id,
          profile: profile?.id,
          environmentId: environment?.id,
          component: component || routed[0]?.name,
          hostname,
          generation: environment?.generation,
        },
      })}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor={`${prefix}-connection`}>Connection</FieldLabel>
          <Choice
            id={`${prefix}-connection`}
            value={connection?.id ?? ""}
            onChange={setConnection}
            options={connections.map((item) => ({
              value: item.id,
              label: item.id,
            }))}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${prefix}-profile`}>Domain profile</FieldLabel>
          <Choice
            id={`${prefix}-profile`}
            value={profile?.id ?? ""}
            onChange={setProfile}
            options={profiles.map((p) => ({
              value: p.id,
              label: p.parentDomain ?? p.id,
            }))}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${prefix}-hostname`}>Hostname</FieldLabel>
          <Input
            id={`${prefix}-hostname`}
            value={hostname}
            onChange={(e) => setHostname(e.target.value)}
            placeholder={`app.${profile?.parentDomain ?? "example.com"}`}
            required
            maxLength={253}
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
          />
          <FieldDescription>
            Use a hostname below {profile?.parentDomain}.
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${prefix}-environment`}>Environment</FieldLabel>
          <Choice
            id={`${prefix}-environment`}
            value={environment?.id ?? ""}
            onChange={(value) => {
              setEnvironment(value);
              setComponent("");
            }}
            options={candidates.map((item) => ({
              value: item.id,
              label: item.label,
            }))}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${prefix}-component`}>
            HTTP component
          </FieldLabel>
          <Choice
            id={`${prefix}-component`}
            value={component || routed[0]?.name || ""}
            onChange={setComponent}
            options={routed.map((item) => ({
              value: item.name,
              label: item.name,
            }))}
          />
        </Field>
      </FieldGroup>
    </CloudForm>
  );
}
