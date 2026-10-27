import { AlertCircle, FolderOpen } from "lucide-react";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";

export function ErrorNotice({ message }: { message?: string | null }) {
  return message ? (
    <Alert variant="destructive">
      <AlertCircle />
      <AlertTitle>Action could not be completed</AlertTitle>
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  ) : null;
}
export function SuccessNotice({ message }: { message?: string | null }) {
  return message ? (
    <Alert role="status">
      <AlertTitle>{message}</AlertTitle>
    </Alert>
  ) : null;
}
export function EmptyState({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <Empty>
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <FolderOpen />
        </EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
        <EmptyDescription>{children}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}
