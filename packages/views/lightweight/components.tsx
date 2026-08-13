"use client";

import type { FormEvent, ReactNode } from "react";
import { Button } from "@dars/ui/components/ui/button";
import { Input } from "@dars/ui/components/ui/input";
import { Label } from "@dars/ui/components/ui/label";
import { Textarea } from "@dars/ui/components/ui/textarea";
import { cn } from "@dars/ui/lib/utils";

export function PageFrame({
  title,
  description,
  action,
  children,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <main className="h-full overflow-y-auto">
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-6 px-6 py-8">
        <header className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-display-sm font-semibold tracking-tight">{title}</h1>
            {description ? <p className="mt-1 text-body text-muted-foreground">{description}</p> : null}
          </div>
          {action}
        </header>
        {children}
      </div>
    </main>
  );
}

export function Panel({ children, className }: { children: ReactNode; className?: string }) {
  return <section className={cn("rounded-xl border bg-card p-5 shadow-sm", className)}>{children}</section>;
}

export function EmptyState({ children }: { children: ReactNode }) {
  return <div className="rounded-xl border border-dashed px-6 py-12 text-center text-body text-muted-foreground">{children}</div>;
}

export function ErrorState({ error }: { error: unknown }) {
  if (!error) return null;
  const message = error instanceof Error ? error.message : "Request failed";
  return <p role="alert" className="rounded-md bg-destructive/10 px-3 py-2 text-body text-destructive">{message}</p>;
}

export function Field({
  label,
  name,
  defaultValue,
  required,
  placeholder,
  type = "text",
}: {
  label: string;
  name: string;
  defaultValue?: string | number;
  required?: boolean;
  placeholder?: string;
  type?: string;
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor={name}>{label}</Label>
      <Input id={name} name={name} defaultValue={defaultValue} required={required} placeholder={placeholder} type={type} />
    </div>
  );
}

export function TextField({
  label,
  name,
  defaultValue,
  required,
  placeholder,
  rows = 5,
}: {
  label: string;
  name: string;
  defaultValue?: string;
  required?: boolean;
  placeholder?: string;
  rows?: number;
}) {
  return (
    <div className="grid gap-2">
      <Label htmlFor={name}>{label}</Label>
      <Textarea id={name} name={name} defaultValue={defaultValue} required={required} placeholder={placeholder} rows={rows} />
    </div>
  );
}

export function SubmitButton({
  pending,
  children = "Save",
  disabled,
}: {
  pending?: boolean;
  children?: ReactNode;
  disabled?: boolean;
}) {
  return (
    <Button type="submit" disabled={pending || disabled}>
      {pending ? "Saving…" : children}
    </Button>
  );
}

export function formValue(form: FormData, name: string): string {
  return String(form.get(name) ?? "").trim();
}

export function onForm(handler: (form: FormData) => unknown | Promise<unknown>) {
  return (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    void handler(new FormData(event.currentTarget));
  };
}
