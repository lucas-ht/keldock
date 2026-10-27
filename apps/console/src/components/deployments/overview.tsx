"use client";
import Link from "next/link";
import { useState } from "react";
import { ArrowRight, ChevronRight, GitBranch, Search } from "lucide-react";
import type { Environment, Project } from "@/lib/contracts";
import { age, environmentName } from "@/lib/format";
import { cn } from "cn";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableHeader,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
} from "@/components/ui/table";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
} from "@/components/ui/select";
import { EmptyState } from "@/components/shared/feedback";
import { Status } from "@/components/shared/status";
import { ComponentStrip } from "./components";

export function Overview({
  environments,
  projects,
  more,
}: {
  environments: Environment[];
  projects: Project[];
  more: boolean;
}) {
  const [selected, setSelected] = useState(
    environments.find((item) => item.operation.state === "failed")?.id ??
      environments[0]?.id,
  );
  const [query, setQuery] = useState("");
  const [state, setState] = useState("all");
  const visible = environments.filter(
    (item) =>
      !item.deleted &&
      (state === "all" ||
        (state === "attention"
          ? item.operation.state === "failed" || item.stale
          : item.operation.state === state)) &&
      `${item.application} ${item.projectId} ${environmentName(item.pullRequest)}`
        .toLowerCase()
        .includes(query.toLowerCase()),
  );
  const current = visible.find((item) => item.id === selected) ?? visible[0];
  const attention = environments.filter(
    (item) =>
      !item.deleted && (item.operation.state === "failed" || item.stale),
  ).length;
  return (
    <>
      <section className="panel" aria-label="Application environments">
        <div className="panel-toolbar">
          <div className="flex items-center gap-3">
            <h2 className="font-semibold">Environments</h2>
            <span className="text-xs text-muted-foreground">
              {attention} need attention
            </span>
          </div>
          <div className="flex w-full flex-wrap items-center gap-3 sm:w-auto">
            <div className="relative min-w-44 flex-1">
              <label className="sr-only" htmlFor="environment-search">
                Find an environment
              </label>
              <Search className="pointer-events-none absolute left-2.5 top-2 size-4 text-muted-foreground" />
              <Input
                id="environment-search"
                className="pl-8"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Find an environment…"
              />
            </div>
            <Select
              items={[
                { value: "all", label: "All statuses" },
                { value: "attention", label: "Needs attention" },
                { value: "ready", label: "Ready" },
                { value: "reconciling", label: "Deploying" },
                { value: "failed", label: "Failed" },
              ]}
              value={state}
              onValueChange={(value) => setState(value ?? "all")}
            >
              <SelectTrigger aria-label="Filter by status">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="all">All statuses</SelectItem>
                  <SelectItem value="attention">Needs attention</SelectItem>
                  <SelectItem value="ready">Ready</SelectItem>
                  <SelectItem value="reconciling">Deploying</SelectItem>
                  <SelectItem value="failed">Failed</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
        </div>
        {visible.length ? (
          <Table className="overview-table">
            <TableHeader>
              <TableRow>
                <TableHead>Application</TableHead>
                <TableHead>Environment</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="optional-column">Commit</TableHead>
                <TableHead className="hidden text-right sm:table-cell">
                  Observed age
                </TableHead>
                <TableHead>
                  <span className="sr-only">Details</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {visible.map((item) => (
                <TableRow
                  key={item.id}
                  data-state={current?.id === item.id ? "selected" : undefined}
                  className={cn(
                    "cursor-pointer",
                    current?.id === item.id && "bg-accent",
                  )}
                  onClick={() => setSelected(item.id)}
                >
                  <TableCell>
                    <Button
                      variant="ghost"
                      className="h-auto justify-start px-0"
                      aria-pressed={current?.id === item.id}
                      onClick={() => setSelected(item.id)}
                    >
                      {item.application}
                    </Button>
                    <span className="block text-xs text-muted-foreground">
                      {projects.find(
                        (project) => project.projectId === item.projectId,
                      )?.name ?? item.projectId}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="inline-flex items-center gap-2">
                      <GitBranch className="size-3.5" />
                      {environmentName(item.pullRequest)}
                    </span>
                  </TableCell>
                  <TableCell>
                    <Status state={item.operation.state} />
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {!item.observation
                        ? "Awaiting evidence"
                        : item.stale
                          ? "Stale evidence"
                          : `Gen ${item.generation} · ${age(item.observationAgeSeconds)} ago`}
                    </span>
                  </TableCell>
                  <TableCell className="optional-column">
                    <code className="text-xs">
                      {item.source?.commit.slice(0, 7) ?? "—"}
                    </code>
                  </TableCell>
                  <TableCell className="hidden text-right text-xs text-muted-foreground sm:table-cell">
                    {item.observation
                      ? `${age(item.observationAgeSeconds)}${item.stale ? " · stale" : ""}`
                      : "No evidence"}
                  </TableCell>
                  <TableCell className="hidden sm:table-cell">
                    <Link
                      className="inline-flex p-2"
                      href={`/projects/${item.projectId}/environments/${item.id}`}
                      aria-label={`Open ${item.application} ${environmentName(item.pullRequest)}`}
                    >
                      <ChevronRight className="size-4" />
                    </Link>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <EmptyState
            title={
              environments.length
                ? "No matching environments"
                : "No deployments yet"
            }
          >
            {environments.length
              ? "Try another name or status filter."
              : "Submit a verified release from your configured CI workflow. Its deployment and component health will appear here."}
          </EmptyState>
        )}
        <div className="panel-footer">
          <span>
            {visible.length} environments shown
            {more ? " · More available in Projects" : ""}
          </span>
          <span>Health is matched to the selected generation</span>
        </div>
      </section>
      {current && (
        <section
          className="selection-region"
          key={current.id}
          aria-label="Selected environment"
        >
          <div className="section-heading">
            <div>
              <h2>
                {current.application}{" "}
                <span className="font-normal text-muted-foreground">
                  / {environmentName(current.pullRequest)}
                </span>
              </h2>
              <p>
                <code>{current.releaseId.slice(0, 16)}</code> ·{" "}
                <code>{current.source?.commit.slice(0, 7)}</code> · Generation{" "}
                {current.generation}
              </p>
            </div>
            <Button
              variant="outline"
              nativeButton={false}
              render={
                <Link
                  href={`/projects/${current.projectId}/environments/${current.id}`}
                />
              }
            >
              Deployment details
              <ArrowRight data-icon="inline-end" />
            </Button>
          </div>
          <ComponentStrip environment={current} />
        </section>
      )}
    </>
  );
}
