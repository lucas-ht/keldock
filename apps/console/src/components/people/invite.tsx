"use client";
import { useState } from "react";
import Link from "next/link";
import type { Project, User } from "@/lib/contracts";
import { mutate } from "@/lib/client-api";
import { date } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { ErrorNotice, SuccessNotice } from "@/components/shared/feedback";
import { SecretOutput } from "@/components/shared/secret";
import { Choice, roles } from "@/components/shared/choice";
import { ProjectGrants } from "./project-grants";

type Invitation = { user: User; activationUrl: string; expiresAt: string };
export function InvitePerson({ projects }: { projects: Project[] }) {
  const [role, setRole] = useState("viewer");
  const [selected, setSelected] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [invitation, setInvitation] = useState<Invitation | null>(null);
  if (invitation)
    return (
      <div className="form-section">
        <SuccessNotice
          message={`Invitation created for ${invitation.user.name}.`}
        />
        <p className="muted">
          Share this link privately. It expires {date(invitation.expiresAt)} and
          can be used once. Keldock has not sent an email.
        </p>
        <SecretOutput
          label="Activation link"
          value={invitation.activationUrl}
        />
        <Button
          variant="outline"
          nativeButton={false}
          render={<Link href="/people" />}
        >
          Back to people
        </Button>
      </div>
    );
  return (
    <form
      className="form-section"
      onSubmit={async (event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        setError(null);
        setBusy(true);
        try {
          setInvitation(
            await mutate<Invitation>("/v1/users", {
              email: data.get("email"),
              name: data.get("name"),
              role,
              projects: role === "admin" ? [] : selected,
            }),
          );
        } catch (error) {
          setError((error as Error).message);
        } finally {
          setBusy(false);
        }
      }}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor="name">Full name</FieldLabel>
          <Input
            id="name"
            name="name"
            autoComplete="name"
            maxLength={80}
            required
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="email">Email address</FieldLabel>
          <Input
            id="email"
            name="email"
            type="email"
            autoComplete="email"
            maxLength={254}
            required
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="role">Role</FieldLabel>
          <Choice id="role" value={role} onChange={setRole} options={roles} />
        </Field>
        <ProjectGrants
          projects={projects}
          selected={selected}
          onChange={setSelected}
          admin={role === "admin"}
        />
        <ErrorNotice message={error} />
        <Button type="submit" disabled={busy}>
          {busy ? "Creating invitation" : "Create invitation"}
        </Button>
      </FieldGroup>
    </form>
  );
}
