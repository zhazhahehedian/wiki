import { Skeleton } from "@/components/ui/skeleton";

export default function HubLoading() {
  return (
    <div className="space-y-6" role="status" aria-label="正在加载页面">
      <p className="text-sm text-zinc-500">正在加载页面…</p>
      <div aria-hidden="true" className="space-y-6">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-4 w-64 max-w-full" />
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    </div>
  );
}
