import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center gap-1.5 rounded-sm px-1.5 py-0.5 font-mono text-[0.68rem] font-medium uppercase tracking-wide",
  {
    variants: {
      variant: {
        ready: "bg-primary/[0.12] text-primary",
        processing: "bg-amber/15 text-amber",
        pending: "bg-foreground/[0.08] text-muted-foreground",
        failed: "bg-destructive/[0.12] text-destructive",
      },
    },
    defaultVariants: { variant: "pending" },
  },
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />;
}

export { Badge, badgeVariants };
