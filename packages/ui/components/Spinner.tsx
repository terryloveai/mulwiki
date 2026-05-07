import { type HTMLAttributes } from "react";
import { cn } from "../lib/cn";

export function Spinner({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent", className)}
      role="status"
      {...props}
    >
      <span className="sr-only">Loading...</span>
    </div>
  );
}
