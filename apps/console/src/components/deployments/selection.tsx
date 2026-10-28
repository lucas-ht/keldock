"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import type { Environment, Page, Release } from "@/lib/contracts";
import { read, mutate } from "@/lib/client-api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogClose,
} from "@/components/ui/dialog";
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ErrorNotice } from "@/components/shared/feedback";

export function ReleaseSelection({
  environment,
  permitted,
}: {
  environment: Environment;
  permitted: boolean;
}) {
  const router = useRouter();
  const [target, setTarget] = useState<Environment | null>(null);
  const [releases, setReleases] = useState<Release[]>([]);
  const [after, setAfter] = useState("");
  const [selected, setSelected] = useState("");
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const key = useRef("");
  const loadEpoch = useRef(0);
  const expired =
    environment.deleted &&
    environment.deletionReason === "expired" &&
    environment.operation.state === "deleted";
  if (
    !permitted ||
    !["staging", "preview"].includes(environment.kind) ||
    environment.pullRequestClosed ||
    (environment.deleted && !expired)
  )
    return null;

  async function load(snapshot: Environment, cursor = "") {
    const epoch = loadEpoch.current;
    setLoading(true);
    setError(null);
    try {
      const page = await read<Page<Release>>(
        `/v1/projects/${snapshot.projectId}/releases?after=${encodeURIComponent(cursor)}`,
      );
      if (epoch !== loadEpoch.current) return;
      setReleases((previous) =>
        cursor ? [...previous, ...page.items] : page.items,
      );
      setAfter(page.nextCursor);
    } catch (error) {
      if (epoch === loadEpoch.current) setError((error as Error).message);
    } finally {
      if (epoch === loadEpoch.current) setLoading(false);
    }
  }
  const candidates = releases
    .filter(
      (release) =>
        target &&
        (target.pullRequest > 0
          ? release.source.pullRequest === target.pullRequest &&
            release.source.headCommit === target.pullRequestHead
          : !release.source.pullRequest &&
            release.application === target.application),
    )
    .sort(
      (a, b) =>
        b.source.runId - a.source.runId ||
        b.source.runAttempt - a.source.runAttempt,
    );
  const choice = candidates.find((release) => release.id === selected);
  const label = (release: Release) =>
    `${release.source.commit.slice(0, 7)} · Run ${release.source.runId}, attempt ${release.source.runAttempt}`;
  return (
    <>
      <Button
        variant="outline"
        onClick={() => {
          loadEpoch.current++;
          setTarget(environment);
          setSelected("");
          setReleases([]);
          setAfter("");
          key.current = crypto.randomUUID();
          void load(environment);
        }}
      >
        Deploy admitted release
      </Button>
      <Dialog
        open={!!target}
        onOpenChange={(open) => {
          if (!open && !busy) {
            loadEpoch.current++;
            setTarget(null);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Deploy an admitted release</DialogTitle>
            <DialogDescription>
              {target?.pullRequest ? `PR #${target.pullRequest}` : "Staging"} ·
              observed generation {target?.generation}. The control plane reuses
              verified images. This may rerun release jobs and supersedes the
              current selection.
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="admitted-release">Release</FieldLabel>
              <Select
                items={candidates.map((release) => ({
                  value: release.id,
                  label: label(release),
                }))}
                value={selected}
                onValueChange={(value) => {
                  setSelected(value ?? "");
                  key.current = crypto.randomUUID();
                }}
              >
                <SelectTrigger
                  id="admitted-release"
                  className="w-full"
                  disabled={loading || busy || !candidates.length}
                >
                  <SelectValue
                    placeholder={
                      loading ? "Loading admitted releases" : "Choose a release"
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {candidates.map((release) => (
                      <SelectItem key={release.id} value={release.id}>
                        {label(release)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <FieldDescription>
                {target?.pullRequest
                  ? "Only releases matching this PR's current source head are eligible."
                  : "Only admitted releases for this staging application are eligible."}
                {!loading &&
                  candidates.length === 0 &&
                  " No matching release is present in the loaded records."}
              </FieldDescription>
            </Field>
            {after && (
              <Button
                variant="outline"
                disabled={loading || busy}
                onClick={() => target && void load(target, after)}
              >
                {loading ? "Loading" : "Load more releases"}
              </Button>
            )}
            {choice && (
              <dl className="flex flex-col gap-3 text-sm">
                <div>
                  <dt className="muted">Release</dt>
                  <dd className="break-all font-mono text-xs">{choice.id}</dd>
                </div>
                <div>
                  <dt className="muted">Tested commit</dt>
                  <dd className="break-all font-mono text-xs">
                    {choice.source.commit}
                  </dd>
                </div>
                {choice.source.headCommit && (
                  <div>
                    <dt className="muted">PR head</dt>
                    <dd className="break-all font-mono text-xs">
                      {choice.source.headCommit}
                    </dd>
                  </div>
                )}
              </dl>
            )}
            <ErrorNotice message={error} />
          </FieldGroup>
          <DialogFooter>
            <DialogClose render={<Button variant="outline" disabled={busy} />}>
              Keep current state
            </DialogClose>
            <Button
              disabled={!choice || busy || loading}
              onClick={async () => {
                if (!target || !choice) return;
                setBusy(true);
                setError(null);
                try {
                  await mutate(
                    `/v1/projects/${target.projectId}/environments/${target.id}/deployments`,
                    { releaseId: choice.id, generation: target.generation },
                    "POST",
                    { "Idempotency-Key": key.current },
                  );
                  setTarget(null);
                  router.refresh();
                } catch (error) {
                  setError((error as Error).message);
                } finally {
                  setBusy(false);
                }
              }}
            >
              {busy ? "Submitting release" : "Deploy selected release"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
