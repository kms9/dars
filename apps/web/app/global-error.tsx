"use client";

export default function GlobalError({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <html lang="en"><body className="grid min-h-screen place-items-center font-sans"><div className="max-w-md text-center"><h1 className="text-title-lg font-semibold">Something went wrong</h1><p className="mt-2 text-body text-neutral-600">The page hit an unexpected error. Try reloading.</p><button type="button" onClick={reset} className="mt-4 rounded border px-4 py-2">Reload</button></div></body></html>
  );
}
