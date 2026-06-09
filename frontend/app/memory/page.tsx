"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { MemoryCard } from "@/components/agent/MemoryCard";
import { api } from "@/lib/api";

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
    <div className="space-y-6 max-w-2xl">
      <h1 className="text-2xl font-bold">Memory</h1>
      <p className="text-muted-foreground text-sm">
        Semantic search over the agent&apos;s long-term episodic memory.
      </p>

      <form onSubmit={search} className="flex gap-2">
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search memories…"
          className="flex-1 rounded-md border bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
        />
        <Button type="submit" disabled={loading || !query.trim()}>
          {loading ? "Searching…" : "Search"}
        </Button>
      </form>

      {searched && results.length === 0 && (
        <p className="text-muted-foreground text-sm">No memories found for that query.</p>
      )}

      <div className="grid gap-3">
        {results.map((r, i) => (
          <MemoryCard key={i} content={r} index={i} />
        ))}
      </div>
    </div>
  );
}
