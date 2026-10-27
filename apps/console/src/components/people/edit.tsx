"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import type { Project, User } from "@/lib/contracts";
import { mutate } from "@/lib/client-api";
import { date } from "@/lib/format";
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
import { ErrorNotice, SuccessNotice } from "@/components/shared/feedback";
import { SecretOutput } from "@/components/shared/secret";
import { Choice, roles } from "@/components/shared/choice";
import { ProjectGrants } from "./project-grants";

export function EditPerson({
  user,
  projects,
  self,
}: {
  user: User;
  projects: Project[];
  self: boolean;
}) {
  const router = useRouter();
  const [name, setName] = useState(user.name),
    [role, setRole] = useState<string>(user.role),
    [status, setStatus] = useState<string>(user.status);
  const [selected, setSelected] = useState(user.projects),
    [error, setError] = useState<string | null>(null),
    [success, setSuccess] = useState<string | null>(null);
  const [busy, setBusy] = useState(false),
    [confirm, setConfirm] = useState(false);
  const [invitation, setInvitation] = useState<{
    activationUrl: string;
    expiresAt: string;
  } | null>(null);
  return (
    <div className="form-section">
      <form
        onSubmit={async (event) => {
          event.preventDefault();
          setError(null);
          setSuccess(null);
          setBusy(true);
          try {
            await mutate(
              `/v1/users/${user.id}`,
              {
                name,
                role,
                status,
                projects: role === "admin" ? [] : selected,
                version: user.version,
              },
              "PATCH",
            );
            if (self) {
              window.location.assign("/sign-in");
              return;
            }
            setSuccess(
              "Access updated. Previous sessions and MCP connections have been revoked.",
            );
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
            <FieldLabel htmlFor="name">Full name</FieldLabel>
            <Input
              id="name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              maxLength={80}
              required
            />
          </Field>
          <Field>
            <FieldLabel>Email address</FieldLabel>
            <p>{user.email}</p>
            <FieldDescription>
              Account email is fixed. Invite another account to change it.
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="role">Role</FieldLabel>
            <Choice id="role" value={role} onChange={setRole} options={roles} />
          </Field>
          <Field>
            <FieldLabel htmlFor="status">Account status</FieldLabel>
            <Choice
              id="status"
              value={status}
              onChange={setStatus}
              options={[
                ...(user.status === "invited"
                  ? [{ value: "invited", label: "Invited" }]
                  : [{ value: "active", label: "Active" }]),
                { value: "disabled", label: "Disabled" },
              ]}
            />
          </Field>
          <ProjectGrants
            projects={projects}
            selected={selected}
            onChange={setSelected}
            admin={role === "admin"}
          />
          <FieldDescription>
            Saving revokes this person’s sessions and MCP connections.
            {self
              ? " You will need to sign in again."
              : " The last active administrator cannot be disabled or demoted."}
          </FieldDescription>
          <ErrorNotice message={error} />
          <SuccessNotice message={success} />
          <Button type="submit" disabled={busy}>
            {busy ? "Saving access" : "Save access"}
          </Button>
        </FieldGroup>
      </form>
      {self ? (
        <p className="muted">
          Use{" "}
          <Link className="underline" href="/account">
            your account
          </Link>{" "}
          to change your own password.
        </p>
      ) : (
        user.status !== "disabled" && (
          <div className="flex flex-col gap-4">
            <h2 className="text-lg font-semibold">
              {user.status === "invited"
                ? "Activation link"
                : "Password recovery"}
            </h2>
            <p className="muted">
              Create a new one-time link. Previous links will stop working;
              password recovery also revokes existing sessions and MCP
              connections.
            </p>
            <div>
              <Button variant="outline" onClick={() => setConfirm(true)}>
                {user.status === "invited"
                  ? "Replace activation link"
                  : "Create password reset link"}
              </Button>
            </div>
          </div>
        )
      )}
      {invitation && (
        <>
          <p className="muted">
            Share privately. Expires {date(invitation.expiresAt)}. No email was
            sent.
          </p>
          <SecretOutput
            label="Recovery link"
            value={invitation.activationUrl}
          />
        </>
      )}
      <Dialog open={confirm} onOpenChange={setConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Replace this person’s access link?</DialogTitle>
            <DialogDescription>
              {user.status === "invited"
                ? "Any previous activation link will stop working."
                : "Existing sessions and MCP connections will be revoked immediately. The new link can set a replacement password once."}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose render={<Button variant="outline" />}>
              Keep current access
            </DialogClose>
            <Button
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                setError(null);
                try {
                  setInvitation(
                    await mutate(`/v1/users/${user.id}/invitation`),
                  );
                  setConfirm(false);
                } catch (error) {
                  setError((error as Error).message);
                  setConfirm(false);
                } finally {
                  setBusy(false);
                }
              }}
            >
              {busy ? "Creating link" : "Create replacement link"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
