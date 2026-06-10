"use client";

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { ComponentPropsWithoutRef } from "react";

interface Props {
  content: string;
  compact?: boolean;
}

export function MarkdownContent({ content, compact = false }: Props) {
  return (
    <div className={compact ? "prose prose-sm" : "prose"}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          pre: ({ children, ...props }: ComponentPropsWithoutRef<"pre">) => (
            <pre {...props} className="overflow-x-auto">
              {children}
            </pre>
          ),
          code: ({ className, children, ...props }: ComponentPropsWithoutRef<"code">) => {
            const isBlock = className?.startsWith("language-");
            if (isBlock) {
              return (
                <code className={className} {...props}>
                  {children}
                </code>
              );
            }
            return <code {...props}>{children}</code>;
          },
          a: ({ children, href, ...props }: ComponentPropsWithoutRef<"a">) => (
            <a href={href} target="_blank" rel="noopener noreferrer" {...props}>
              {children}
            </a>
          ),
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}
