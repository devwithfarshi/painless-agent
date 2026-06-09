import { api } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader } from "@/components/ui/card";

export const dynamic = "force-dynamic";

export default async function SkillsPage() {
  let skills: Awaited<ReturnType<typeof api.skills.list>> = [];
  try {
    skills = await api.skills.list();
  } catch {
    // API unavailable during build
  }

  return (
    <div className="space-y-6 max-w-3xl">
      <div>
        <h1 className="text-2xl font-bold">Skills</h1>
        <p className="text-muted-foreground text-sm">
          Reusable workflows learned from completed tasks.
        </p>
      </div>

      {skills.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          No skills yet. Complete tasks to let the agent learn reusable workflows.
        </p>
      ) : (
        <div className="grid gap-4">
          {skills.map((skill) => (
            <Card key={skill.ID}>
              <CardHeader className="pb-2 flex flex-row items-center justify-between">
                <h2 className="font-semibold">{skill.Name}</h2>
                <Badge variant="secondary">used {skill.UsageCount}×</Badge>
              </CardHeader>
              <CardContent>
                <p className="text-sm text-muted-foreground">{skill.Description}</p>
                {skill.Steps && skill.Steps.length > 0 && (
                  <ol className="mt-2 space-y-1 list-decimal list-inside">
                    {skill.Steps.map((s, i) => (
                      <li key={i} className="text-xs">
                        {s.description}
                        {s.tool_hint && (
                          <span className="ml-1 text-muted-foreground">({s.tool_hint})</span>
                        )}
                      </li>
                    ))}
                  </ol>
                )}
                <p className="text-xs text-muted-foreground mt-2">
                  Created {new Date(skill.CreatedAt).toLocaleDateString()}
                </p>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
