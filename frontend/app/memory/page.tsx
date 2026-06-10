"use client";

import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { MemoryCard } from "@/components/agent/MemoryCard";
import { api } from "@/lib/api";
import { SearchIcon, DatabaseIcon, XIcon } from "lucide-react";

type MemoryEntry = { content: string; created_at: string };

export default function MemoryPage() {
  const [query, setQuery] = useState("");
  const [allMemories, setAllMemories] = useState<MemoryEntry[]>([]);
  const [searchResults, setSearchResults] = useState<MemoryEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [searching, setSearching] = useState(false);
  const [isSearchMode, setIsSearchMode] = useState(false);

  useEffect(() => {
    api.memory
      .list(50)
      .then((d) => setAllMemories(d.results ?? []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const search = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!query.trim()) return;
    setSearching(true);
    setIsSearchMode(true);
    try {
      const data = await api.memory.search(query.trim());
      setSearchResults(data.results ?? []);
    } catch {
      alert("Search failed");
    } finally {
      setSearching(false);
    }
  };

  const clearSearch = () => {
    setQuery("");
    setIsSearchMode(false);
    setSearchResults([]);
  };

  const displayed = isSearchMode ? searchResults : allMemories;

  return (
    <div className="space-y-8 max-w-3xl">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Memory</h1>
        <p className="text-muted-foreground text-sm mt-1">
          Long-term episodic memory stored by the agent across tasks.
        </p>
      </div>

      <div className="rounded-lg border bg-card p-5">
        <div className="flex items-center gap-2 mb-3">
          <SearchIcon className="w-4 h-4 text-primary" />
          <h2 className="text-sm font-semibold">Semantic Search</h2>
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
          <Button type="submit" disabled={searching || !query.trim()} size="sm">
            {searching ? "Searching…" : "Search"}
          </Button>
          {isSearchMode && (
            <Button type="button" variant="ghost" size="sm" onClick={clearSearch}>
              <XIcon className="w-4 h-4" />
            </Button>
          )}
        </form>
      </div>

      <section>
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
            {isSearchMode
              ? `Search Results (${displayed.length})`
              : `Recent Memories (${allMemories.length})`}
          </h2>
        </div>

        {loading ? (
          <div className="space-y-3">
            {[...Array(3)].map((_, i) => (
              <div key={i} className="h-24 rounded-lg border bg-muted/30 animate-pulse" />
            ))}
          </div>
        ) : displayed.length === 0 ? (
          <div className="rounded-lg border border-dashed p-10 text-center">
            <DatabaseIcon className="w-8 h-8 text-muted-foreground/40 mx-auto mb-2" />
            <p className="text-sm text-muted-foreground">
              {isSearchMode
                ? "No memories matched that query."
                : "No memories yet. Run tasks to let the agent build its memory."}
            </p>
          </div>
        ) : (
          <div className="space-y-3">
            {displayed.map((m, i) => (
              <MemoryCard key={i} content={m.content} created_at={m.created_at} index={i} />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
