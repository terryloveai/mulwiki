import Link from "next/link";
import { BookOpen, ArrowRight } from "lucide-react";
import { Button } from "@mulwiki/ui/components/Button";

export default function HomePage() {
  return (
    <main className="flex min-h-svh flex-col items-center justify-center gap-8 px-4">
      <div className="flex flex-col items-center gap-4 text-center">
        <div className="flex h-12 w-12 items-center justify-center rounded-lg bg-brand">
          <BookOpen className="h-6 w-6 text-brand-foreground" />
        </div>
        <h1 className="text-3xl font-bold tracking-tight text-foreground">
          Mulwiki
        </h1>
        <p className="max-w-md text-muted-foreground">
          Structured knowledge base built from your documents. Upload sources,
          run compile jobs, and browse the resulting wiki.
        </p>
      </div>
      <Link href="/workspaces">
        <Button variant="brand" size="lg">
          View Workspaces
          <ArrowRight className="h-4 w-4" />
        </Button>
      </Link>
    </main>
  );
}
