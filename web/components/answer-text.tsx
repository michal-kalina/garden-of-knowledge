"use client";

import React from "react";

// Minimal answer renderer: paragraphs, unordered/ordered list lines, **bold**,
// `inline code`, and — the signature element — [n] citation leaf-tags that
// light up their source card. A full markdown library can replace this later;
// the citation handling would still need this custom pass, so the hand-rolled
// version earns its ~80 lines for now.

const INLINE = /(\*\*[^*]+\*\*|`[^`]+`|\[\d+\])/g;

function renderInline(
  text: string,
  onCite: (n: number) => void,
  keyPrefix: string,
): React.ReactNode[] {
  return text
    .split(INLINE)
    .filter(Boolean)
    .map((seg, i) => {
      const key = `${keyPrefix}-${i}`;
      if (seg.startsWith("**") && seg.endsWith("**")) {
        return <strong key={key}>{seg.slice(2, -2)}</strong>;
      }
      if (seg.startsWith("`") && seg.endsWith("`")) {
        return (
          <code key={key} className="rounded-sm bg-muted px-1 font-mono text-[0.85em]">
            {seg.slice(1, -1)}
          </code>
        );
      }
      const cite = /^\[(\d+)\]$/.exec(seg);
      if (cite) {
        const n = Number(cite[1]);
        return (
          <button
            key={key}
            type="button"
            className="citation"
            title={`Show source ${n}`}
            onClick={() => onCite(n)}
          >
            {n}
          </button>
        );
      }
      return <React.Fragment key={key}>{seg}</React.Fragment>;
    });
}

export function AnswerText({
  text,
  onCite,
}: {
  text: string;
  onCite: (n: number) => void;
}) {
  const blocks = text.split(/\n\n+/).filter((b) => b.trim() !== "");

  return (
    <div className="space-y-3 text-[0.925rem] leading-relaxed">
      {blocks.map((block, bi) => {
        const lines = block.split("\n");
        const isList = lines.every((l) => /^\s*([-*]|\d+\.)\s+/.test(l.trim()) || l.trim() === "");

        if (isList) {
          const items = lines.filter((l) => l.trim() !== "");
          const ordered = /^\s*\d+\./.test(items[0]);
          const List = ordered ? "ol" : "ul";
          return (
            <List
              key={bi}
              className={ordered ? "list-decimal space-y-1 pl-5" : "list-disc space-y-1 pl-5"}
            >
              {items.map((l, li) => (
                <li key={li}>
                  {renderInline(
                    l.trim().replace(/^([-*]|\d+\.)\s+/, ""),
                    onCite,
                    `${bi}-${li}`,
                  )}
                </li>
              ))}
            </List>
          );
        }

        return (
          <p key={bi} className="whitespace-pre-wrap">
            {renderInline(block, onCite, `${bi}`)}
          </p>
        );
      })}
    </div>
  );
}
