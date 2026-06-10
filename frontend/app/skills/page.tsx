import { api } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { ZapIcon, ChevronRightIcon } from "lucide-react";

export const dynamic = "force-dynamic";

export default async function SkillsPage() {
  let skills: Awaited<ReturnType<typeof api.skills.list>> = [];
  try {
    skills = await api.skills.list();
  } catch {
    // API unavailable during build
  }

  return (
    <div className="space-y-8 max-w-4xl">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Skills</h1>
        <p className="text-muted-foreground text-sm mt-1">
          Reusable workflows the agent has learned from completed tasks.
        </p>
      </div>

      {skills.length === 0 ? (
        <div className="rounded-lg border border-dashed p-10 text-center">
          <ZapIcon className="w-8 h-8 text-muted-foreground/40 mx-auto mb-2" />
          <p className="text-sm text-muted-foreground">
            No skills yet. Complete tasks to let the agent learn reusable workflows.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {skills.map((skill) => (
            <Card key={skill.ID} className="overflow-hidden">
              <CardHeader className="pb-3">
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <ZapIcon className="w-4 h-4 text-primary shrink-0 mt-0.5" />
                    <h2 className="font-semibold text-sm">{skill.Name}</h2>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Badge variant="secondary" className="text-xs">
                      {skill.UsageCount}× used
                    </Badge>
                    <span className="text-xs text-muted-foreground">
                      {new Date(skill.CreatedAt).toLocaleDateString()}
                    </span>
                  </div>
                </div>
                <p className="text-sm text-muted-foreground mt-1 ml-6">{skill.Description}</p>
              </CardHeader>

              {skill.Steps && skill.Steps.length > 0 && (
                <CardContent className="pt-0">
                  <div className="ml-6 border-l pl-4 space-y-1.5">
                    {skill.Steps.map((s, i) => (
                      <div key={i} className="flex items-start gap-2">
                        <ChevronRightIcon className="w-3.5 h-3.5 text-muted-foreground/60 shrink-0 mt-0.5" />
                        <div className="text-xs">
                          <span>{s.description}</span>
                          {s.tool_hint && (
                            <span className="ml-1.5 text-muted-foreground font-mono">
                              [{s.tool_hint}]
                            </span>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                </CardContent>
              )}
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
