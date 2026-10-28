"use client";
import { useId, useState } from "react";
import type { CloudCatalog, CloudIntent } from "@/lib/cloud-contracts";
import { cloudKinds } from "@/lib/cloud-contracts";
import { CloudForm } from "../request";
import { Choice } from "@/components/shared/choice";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldLabel,
  FieldGroup,
  FieldDescription,
  FieldSet,
  FieldLegend,
} from "@/components/ui/field";

export function ConnectionForm({
  project,
  catalog,
  current,
}: {
  project: string;
  catalog: CloudCatalog;
  current?: CloudIntent;
}) {
  const prefix = useId();
  const [name, setName] = useState(current?.id ?? "aws-default");
  const [account, setAccount] = useState(
    current?.connection?.account ?? catalog.accounts[0] ?? "",
  );
  const [region, setRegion] = useState(
    current?.connection?.region ?? catalog.regions[0] ?? "",
  );
  const [identity, setIdentity] = useState(
    current?.connection?.identityRef ?? catalog.identities[0] ?? "",
  );
  const [profiles, setProfiles] = useState(
    current?.connection?.profiles ??
      catalog.profiles.filter((p) => p.account === account).map((p) => p.id),
  );
  return (
    <CloudForm
      project={project}
      title={
        current
          ? "Update approved connection"
          : "Connect an approved AWS account"
      }
      trigger={current ? "Edit connection" : "Connect AWS"}
      submitLabel={current ? "Save new revision" : "Verify connection"}
      submitDisabled={profiles.length === 0}
      description="Choose an account and identity already approved by your installation operator. Keldock verifies the provider identities before allowing resources."
      command={() => ({
        kind: "connection",
        id: name,
        input: {
          provider: "aws",
          account,
          region,
          identityRef: identity,
          profiles,
          generation: current ? current.generation + 1 : 1,
        },
      })}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor={`${prefix}-name`}>Connection name</FieldLabel>
          <Input
            id={`${prefix}-name`}
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={!!current}
            required
            maxLength={40}
            pattern="[a-z][a-z0-9-]{0,39}"
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${prefix}-account`}>AWS account</FieldLabel>
          {current ? (
            <Input id={`${prefix}-account`} value={account} readOnly />
          ) : (
            <Choice
              id={`${prefix}-account`}
              value={account}
              onChange={(value) => {
                setAccount(value);
                setProfiles(
                  catalog.profiles
                    .filter((p) => p.account === value)
                    .map((p) => p.id),
                );
              }}
              options={catalog.accounts.map((value) => ({
                value,
                label: value,
              }))}
            />
          )}
        </Field>
        <Field>
          <FieldLabel htmlFor={`${prefix}-region`}>Region</FieldLabel>
          {current ? (
            <Input id={`${prefix}-region`} value={region} readOnly />
          ) : (
            <Choice
              id={`${prefix}-region`}
              value={region}
              onChange={setRegion}
              options={catalog.regions.map((value) => ({
                value,
                label: value,
              }))}
            />
          )}
        </Field>
        <Field>
          <FieldLabel htmlFor={`${prefix}-identity`}>
            Approved identity
          </FieldLabel>
          {current ? (
            <Input id={`${prefix}-identity`} value={identity} readOnly />
          ) : (
            <Choice
              id={`${prefix}-identity`}
              value={identity}
              onChange={setIdentity}
              options={catalog.identities.map((value) => ({
                value,
                label: value,
              }))}
            />
          )}
        </Field>
        <FieldSet>
          <FieldLegend>Resource profiles</FieldLegend>
          <FieldDescription>
            Select at least one approved profile.
          </FieldDescription>
          <FieldGroup>
            {catalog.profiles
              .filter((profile) => profile.account === account)
              .map((profile) => (
                <Field orientation="horizontal" key={profile.id}>
                  <Checkbox
                    id={`${prefix}-${profile.id}`}
                    checked={profiles.includes(profile.id)}
                    onCheckedChange={(checked) =>
                      setProfiles((values) =>
                        checked
                          ? [...values, profile.id]
                          : values.filter((value) => value !== profile.id),
                      )
                    }
                  />
                  <FieldLabel htmlFor={`${prefix}-${profile.id}`}>
                    {cloudKinds[profile.kind]} · {profile.id}
                  </FieldLabel>
                </Field>
              ))}
          </FieldGroup>
        </FieldSet>
        {current && (
          <FieldDescription>
            A new connection revision suspends existing resource grants until
            their authority is refreshed.
          </FieldDescription>
        )}
      </FieldGroup>
    </CloudForm>
  );
}
