import { Skeleton } from "@/components/ui/skeleton";
export default function Loading() {
  return (
    <div
      aria-label="Loading workspace"
      role="status"
      className="flex flex-col gap-6"
    >
      <Skeleton className="h-8 w-60" />
      <Skeleton className="h-5 w-80 max-w-full" />
      <Skeleton className="h-64 w-full" />
      <Skeleton className="h-40 w-full" />
      <span className="sr-only">Loading workspace</span>
    </div>
  );
}
