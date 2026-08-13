"use client"

import * as React from "react"

import { cn } from "@dars/ui/lib/utils"

function Checkbox({ className, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      type="checkbox"
      data-slot="checkbox"
      className={cn(
        "size-4 shrink-0 rounded border border-input accent-primary outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50",
        className
      )}
      {...props}
    />
  )
}

export { Checkbox }
