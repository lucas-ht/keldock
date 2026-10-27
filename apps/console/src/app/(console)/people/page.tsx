import Link from "next/link";
import { Plus, ChevronRight } from "lucide-react";
import { api, requireAdmin } from "@/lib/server-api";
import type { User, Page } from "@/lib/contracts";
import { date } from "@/lib/format";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Status } from "@/components/shared/status";
export const metadata = { title: "People" };
export default async function People({
  searchParams,
}: {
  searchParams: Promise<{ after?: string }>;
}) {
  await requireAdmin();
  const { after } = await searchParams;
  const page = await api<Page<User>>(
    `/v1/users?after=${encodeURIComponent(after ?? "")}`,
  );
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>People</h1>
          <p>
            Manage accounts, roles, and project access in this installation.
          </p>
        </div>
        <Button nativeButton={false} render={<Link href="/people/invite" />}>
          <Plus data-icon="inline-start" />
          Invite person
        </Button>
      </div>
      <div className="panel">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Person</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="optional-column">Last sign-in</TableHead>
              <TableHead>
                <span className="sr-only">Manage person</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {page.items.map((user) => (
              <TableRow key={user.id}>
                <TableCell>
                  <Link
                    className="font-medium hover:underline"
                    href={`/people/${user.id}`}
                  >
                    {user.name}
                  </Link>
                  <span className="mt-1 block text-xs text-muted-foreground">
                    {user.email}
                  </span>
                </TableCell>
                <TableCell className="capitalize">{user.role}</TableCell>
                <TableCell>
                  <Status state={user.status} />
                </TableCell>
                <TableCell className="optional-column text-xs text-muted-foreground">
                  {date(user.lastLoginAt)}
                </TableCell>
                <TableCell>
                  <Link
                    className="inline-flex p-2"
                    href={`/people/${user.id}`}
                    aria-label={`Manage ${user.name}`}
                  >
                    <ChevronRight className="size-4" />
                  </Link>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <div className="panel-footer">
          <span>{page.items.length} people shown</span>
          <span>
            Account changes revoke existing sessions and MCP connections
          </span>
        </div>
      </div>
      {page.nextCursor && (
        <Button
          variant="outline"
          className="mt-6"
          nativeButton={false}
          render={
            <Link
              href={`/people?after=${encodeURIComponent(page.nextCursor)}`}
            />
          }
        >
          Next people
        </Button>
      )}
      {after && (
        <p className="mt-4">
          <Link href="/people">Back to first page</Link>
        </p>
      )}
    </>
  );
}
