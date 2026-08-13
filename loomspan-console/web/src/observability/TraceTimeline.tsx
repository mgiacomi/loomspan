import { useState } from "react";
import type { TraceFrame } from "../api/contracts";

function formatDuration(durationMillis: number) {
  const exact = `${durationMillis.toLocaleString("en-US")} ms`;
  if (durationMillis < 1000) return exact;

  const hours = Math.floor(durationMillis / 3_600_000);
  const minutes = Math.floor((durationMillis % 3_600_000) / 60_000);
  const remainingMillis = durationMillis % 60_000;
  const seconds = (remainingMillis / 1000).toFixed(3).replace(/\.?(?:0+)$/, "");
  const readable = [hours ? `${hours}h` : "", hours || minutes ? `${minutes}m` : "", `${seconds}s`].filter(Boolean).join(" ");
  return `${readable} (${exact})`;
}

type TimelineBarState = "normal" | "warning" | "error";

function timelineBarState(frame: TraceFrame): TimelineBarState {
  if (frame.failureIds.length > 0 || frame.outcomes.some((outcome) => outcome === "FAILED" || outcome === "ABORTED")) return "error";
  const hasRetry = frame.frameType === "RETRY" || frame.attemptIds.length > frame.retrySequenceIds.length;
  const hasValidationWarning = frame.validationStatuses.some((status) => !["PASSED", "SUCCEEDED", "VALID"].includes(status.toUpperCase()));
  return hasRetry || hasValidationWarning ? "warning" : "normal";
}

export function TraceTimeline({ frames, selectedFrameId, onSelect }: { frames: TraceFrame[]; selectedFrameId?: string; onSelect: (frameId: string) => void }) {
  const [hoveredFrameId, setHoveredFrameId] = useState<string>();
  const complete = frames.filter((frame) => frame.closedTimestampMillis != null);
  const start = complete.length ? Math.min(...complete.map((frame) => frame.openedTimestampMillis)) : 0;
  const end = complete.length ? Math.max(...complete.map((frame) => frame.closedTimestampMillis as number)) : start;
  const span = Math.max(1, end - start);
  return <div className="trace-timeline" aria-label="Trace timeline">
    {frames.map((frame) => {
      const closed = frame.closedTimestampMillis;
      const available = closed != null && frame.inclusiveDurationMillis != null;
      const x = available ? ((frame.openedTimestampMillis - start) / span) * 1000 : 0;
      const width = available ? Math.max(2, ((closed - frame.openedTimestampMillis) / span) * 1000) : 0;
      const barState = timelineBarState(frame);
      const stateLabel = barState === "error" ? "Error or failure" : barState === "warning" ? "Retry or warning" : undefined;
      const tooltip = available ? `Duration: ${formatDuration(frame.inclusiveDurationMillis as number)}${stateLabel ? ` · ${stateLabel}` : ""}` : "";
      const hitWidth = Math.max(width, 14);
      const hitX = Math.min(1000 - hitWidth, Math.max(0, x - (hitWidth - width) / 2));
      const tooltipPosition = Math.min(96, Math.max(4, (x + width / 2) / 10));
      return <div className="trace-timeline-row" key={frame.frameId} aria-current={selectedFrameId === frame.frameId ? "true" : undefined}>
        <button type="button" onClick={() => onSelect(frame.frameId)}>{frame.route || frame.frameId}</button>
        {available ? <div className="trace-timeline-chart"><svg viewBox="0 0 1000 20" role="img" aria-label={`${frame.inclusiveDurationMillis} ms, ${frame.selfDurationMillis == null ? "self timing unavailable" : `${frame.selfDurationMillis} ms self`}${stateLabel ? `, ${stateLabel.toLowerCase()}` : ""}`} preserveAspectRatio="none"><rect className="trace-timeline-track" x="0" y="5" width="1000" height="10" /><rect className={`trace-timeline-bar trace-timeline-bar-${barState}`} x={x} y="5" width={width} height="10" /><rect className="trace-timeline-hit-target" data-frame-id={frame.frameId} x={hitX} y="0" width={hitWidth} height="20" onPointerEnter={() => setHoveredFrameId(frame.frameId)} onPointerLeave={() => setHoveredFrameId(undefined)} /></svg>{hoveredFrameId === frame.frameId && <span className={`trace-timeline-tooltip trace-timeline-tooltip-${barState}`} role="tooltip" style={{ left: `${tooltipPosition}%` }}>{tooltip}</span>}</div> : <span>Timing unavailable or incomplete</span>}
      </div>;
    })}
  </div>;
}
