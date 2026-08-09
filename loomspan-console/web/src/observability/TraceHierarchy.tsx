import { useMemo, useState, type KeyboardEvent } from "react";
import type { TraceFrame } from "../api/contracts";

export function TraceHierarchy({ frames, selectedFrameId, onSelect }: { frames: TraceFrame[]; selectedFrameId?: string; onSelect: (frameId: string) => void }) {
  const byID = useMemo(() => new Map(frames.map((frame) => [frame.frameId, frame])), [frames]);
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());
  const visible = useMemo(() => {
    const result: Array<{ frame: TraceFrame; level: number }> = [];
    const visit = (frame: TraceFrame, level: number) => {
      result.push({ frame, level });
      if (!collapsed.has(frame.frameId)) for (const childID of frame.childFrameIds ?? []) { const child = byID.get(childID); if (child) visit(child, level + 1); }
    };
    for (const frame of frames) if (frame.parentFrameId == null || !byID.has(frame.parentFrameId)) visit(frame, 1);
    return result;
  }, [byID, collapsed, frames]);
  const toggle = (frameID: string) => setCollapsed((current) => { const next = new Set(current); if (next.has(frameID)) next.delete(frameID); else next.add(frameID); return next; });
  const move = (event: KeyboardEvent<HTMLButtonElement>, frame: TraceFrame) => {
    const buttons = [...(event.currentTarget.closest("ul")?.querySelectorAll<HTMLButtonElement>("button[data-frame]") ?? [])];
    const index = buttons.indexOf(event.currentTarget);
    const target = event.key === "Home" ? buttons[0] : event.key === "End" ? buttons.at(-1) : event.key === "ArrowDown" ? buttons[index + 1] : event.key === "ArrowUp" ? buttons[index - 1] : undefined;
    if (target) { event.preventDefault(); target.focus(); return; }
    if (event.key === "ArrowRight" && (frame.childFrameIds?.length ?? 0) > 0) { event.preventDefault(); if (collapsed.has(frame.frameId)) toggle(frame.frameId); else buttons[index + 1]?.focus(); }
    if (event.key === "ArrowLeft") { event.preventDefault(); if (!collapsed.has(frame.frameId) && (frame.childFrameIds?.length ?? 0) > 0) toggle(frame.frameId); else if (frame.parentFrameId) buttons.find((button) => button.dataset.frame === frame.parentFrameId)?.focus(); }
  };
  return <ul aria-label="Frame hierarchy" role="tree">{visible.map(({ frame, level }) => <li key={frame.frameId} role="treeitem" aria-selected={selectedFrameId === frame.frameId} aria-level={level} aria-expanded={(frame.childFrameIds?.length ?? 0) ? !collapsed.has(frame.frameId) : undefined} style={{ paddingInlineStart: `${(level - 1) * 1.25}rem` }}>
    {(frame.childFrameIds?.length ?? 0) > 0 && <button type="button" aria-label={`${collapsed.has(frame.frameId) ? "Expand" : "Collapse"} ${frame.route || frame.frameId}`} onClick={() => toggle(frame.frameId)}>{collapsed.has(frame.frameId) ? "+" : "−"}</button>}
    <button data-frame={frame.frameId} type="button" aria-pressed={selectedFrameId === frame.frameId} onKeyDown={(event) => move(event, frame)} onClick={() => onSelect(frame.frameId)}>{frame.frameType}: {frame.route || frame.frameId}{(frame.failureIds?.length ?? 0) > 0 && ` · ${frame.failureIds.length} failure${frame.failureIds.length === 1 ? "" : "s"}`}</button>{frame.inclusiveDurationMillis == null ? " (timing unavailable)" : ` (${frame.inclusiveDurationMillis} ms)`}
  </li>)}</ul>;
}
