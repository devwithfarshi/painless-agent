"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { MemoryCard } from "@/components/agent/MemoryCard";
import { api } from "@/lib/api";
import { SearchIcon, DatabaseIcon } from "lucide-react";

export default function MemoryPage() {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);

  const search = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!query.trim()) return;
    setLoading(true);
    try {
      const data = await api.memory.search(query.trim());
      setResults(data.results ?? []);
      setSearched(true);
    } catch {
      alert("Search failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-8 max-w-3xl">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Memory</h1>
        <p className="text-muted-foreground text-sm mt-1">
          Semantic search over the agent&apos;s long-term episodic memory.
        </p>
      </div>

      <div className="rounded-lg border bg-card p-5">
        <div className="flex items-center gap-2 mb-3">
          <DatabaseIcon className="w-4 h-4 text-primary" />
          <h2 className="text-sm font-semibold">Search Memory</h2>
        </div>
        <form onSubmit={search} className="flex gap-2">
          <div className="relative flex-1">
            <SearchIcon className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground pointer-events-none" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search memories by meaning…"
              className="w-full rounded-md border bg-background pl-9 pr-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
            />
          </div>
          <Button type="submit" disabled={loading || !query.trim()} size="sm">
            {loading ? "Searching…" : "Search"}
          </Button>
        </form>
      </div>

      {searched && (
        <section>
          <h2 className="text-sm font-semibold mb-3 text-muted-foreground uppercase tracking-wide">
            Results {results.length > 0 && `(${results.length})`}
          </h2>
          {results.length === 0 ? (
            <div className="rounded-lg border border-dashed p-8 text-center">
              <p className="text-sm text-muted-foreground">No memories found for that query.</p>
            </div>
          ) : (
            <div className="space-y-3">
              {results.map((r, i) => (
                <MemoryCard key={i} content={r} index={i} />
              ))}
            </div>
          )}
        </section>
      )}

      {!searched && (
        <div className="rounded-lg border border-dashed p-10 text-center">
          <DatabaseIcon className="w-8 h-8 text-muted-foreground/40 mx-auto mb-2" />
          <p className="text-sm text-muted-foreground">
            Enter a search query to find relevant memories.
          </p>
        </div>
      )}
    </div>
  );
}
