import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { Link, useSearchParams } from "react-router";
import {
  BrowserAPIError,
  getContentRange,
  getTraceAnalysisSummary,
  getTraceFailures,
  getTraceFrames,
  getTraceRecords,
  getTraceUsage,
  listSkills,
  searchTraceEvidence,
  type TraceFrameFilter,
} from "../api/client";
import type {
  TraceAnalysisPage,
  TraceAnalysisSummary,
  TraceFailure,
  TraceFrame,
  TraceRange,
  TraceRecord,
  TraceSearchPage,
  TraceUsage as Usage,
  SkillSummary,
	TraceSource,
} from "../api/contracts";
import { useTarget } from "../target/TargetProvider";
import { TraceEvidenceDetail } from "./TraceEvidenceDetail";
import { TraceHierarchy } from "./TraceHierarchy";
import { TraceRecords } from "./TraceRecords";
import { TraceTimeline } from "./TraceTimeline";
import { TraceUsage } from "./TraceUsage";
import { requireCurrentTargetScope, scopeBoundPath } from "./scope";
import { TraceFailureFocus } from "./TraceFailureFocus";
import { TraceFailureDiagnostic } from "./TraceFailureDiagnostic";
import { readTraceExplorerState, setTraceExplorerSelection, type TraceExplorerView } from "./traceExplorerState";

const views: TraceExplorerView[] = ["hierarchy", "timeline", "usage", "records"];

function appendPage<T, P extends TraceAnalysisPage<T>>(current: P, next: P): P {
  return { ...next, items: [...current.items, ...next.items] };
}

function appendSearchPage(current: TraceSearchPage, next: TraceSearchPage): TraceSearchPage {
  const prefix = `p${current.items.length}-`;
  const rebasedDescriptors = next.contentDescriptors.map((descriptor) => ({ ...descriptor, contentId: `${prefix}${descriptor.contentId}` }));
  const rebasedItems = next.items.map((item) => ({ ...item, contentId: item.contentId ? `${prefix}${item.contentId}` : undefined }));
  return { ...next, items: [...current.items, ...rebasedItems], contentDescriptors: [...current.contentDescriptors, ...rebasedDescriptors] };
}

function mergeFrames(current: TraceAnalysisPage<TraceFrame> | undefined, added: TraceFrame[]) {
  if (!current) return undefined;
  const replacements = new Map(added.map((frame) => [frame.frameId, frame]));
  const items = current.items.map((frame) => replacements.get(frame.frameId) ?? frame);
  const known = new Set(items.map((frame) => frame.frameId));
  return { ...current, items: [...items, ...added.filter((frame) => !known.has(frame.frameId))] };
}

export function TraceExplorer({ traceId, source = "TARGET", onArtifactUnavailable }: { traceId: string; source?: TraceSource; onArtifactUnavailable?: (error: BrowserAPIError) => void }) {
  const { target, scopeGeneration, refresh } = useTarget();
  const [params, setParams] = useSearchParams();
  const setParamsRef = useRef(setParams);
  setParamsRef.current = setParams;
  const state = readTraceExplorerState(params);
  const [summary, setSummary] = useState<TraceAnalysisSummary>();
  const [frames, setFrames] = useState<TraceAnalysisPage<TraceFrame>>();
	const [detailedFrames, setDetailedFrames] = useState<TraceAnalysisPage<TraceFrame>>();
  const [records, setRecords] = useState<TraceAnalysisPage<TraceRecord>>();
  const [failures, setFailures] = useState<TraceAnalysisPage<TraceFailure>>();
  const [searchText, setSearchText] = useState("");
  const [searchResults, setSearchResults] = useState<TraceSearchPage>();
  const [usage, setUsage] = useState<Usage>();
  const [usageFrames, setUsageFrames] = useState<TraceAnalysisPage<TraceFrame>>();
  const [usageResponseRecords, setUsageResponseRecords] = useState<TraceRecord[]>();
  const [registeredSkills, setRegisteredSkills] = useState<Set<string>>();
  const [range, setRange] = useState<TraceRange>();
  const [rangeRequest, setRangeRequest] = useState<{ contentRef: string; recordSequence?: number }>();
  const [rangeError, setRangeError] = useState<string>();
  const [error, setError] = useState<string>();
  const [scopeMismatch, setScopeMismatch] = useState(false);
  const [pending, setPending] = useState<Set<string>>(() => new Set());
  const currentScopeID = target.status.targetScopeId;
  const select = useCallback((values: Record<string, string | number | undefined>) => {
    setParamsRef.current((current) => setTraceExplorerSelection(current, values), { replace: true });
  }, []);
	const verifyImportedResponse = useCallback(async <T extends { source: TraceSource; targetScopeId?: string }>(response: T) => {
		if (response.source !== "IMPORTED" || response.targetScopeId) {
			setScopeMismatch(true);
			select({ frameId: undefined, recordSequence: undefined, failureId: undefined });
		}
		return response;
	}, [select]);
	const verifyTargetResponse = useCallback(async <T extends { source: TraceSource; targetScopeId?: string }>(response: T) => {
		if (response.source !== "TARGET" || !response.targetScopeId || response.targetScopeId !== currentScopeID) {
      setScopeMismatch(true);
      select({ frameId: undefined, recordSequence: undefined, failureId: undefined });
    }
		await requireCurrentTargetScope(response.targetScopeId ?? "", currentScopeID, refresh);
    return response;
	}, [currentScopeID, refresh, select]);
	const verifyScope = source === "TARGET" ? verifyTargetResponse : verifyImportedResponse;
	const evidenceScopeKey = source === "TARGET" ? currentScopeID : "";
  const reportError = useCallback((value: unknown, artifactLookup = false) => {
    const normalized = value instanceof Error ? value : new Error("The trace evidence could not be loaded.");
    const browserError = normalized instanceof BrowserAPIError || "code" in normalized ? normalized as BrowserAPIError : undefined;
    if (browserError?.code === "TARGET_CHANGED") {
      select({ frameId: undefined, recordSequence: undefined, failureId: undefined });
    }
    if (browserError && (browserError.code === "ARTIFACT_EXPIRED" || (artifactLookup && browserError.code === "NOT_FOUND"))) {
      select({ view: undefined, frameId: undefined, recordSequence: undefined, failureId: undefined });
      setSummary(undefined);
      setFrames(undefined);
      setRecords(undefined);
      setRange(undefined);
      onArtifactUnavailable?.(browserError);
    }
    setError(normalized.message);
  }, [onArtifactUnavailable, select]);
  const begin = (key: string) => setPending((value) => new Set(value).add(key));
  const end = (key: string) => setPending((value) => { const next = new Set(value); next.delete(key); return next; });

  useEffect(() => {
    if (!state.valid) select({ view: undefined, frameId: undefined, recordSequence: undefined, failureId: undefined });
  }, [select, state.valid]);
  useEffect(() => {
		if (!state.valid || (source === "TARGET" && !currentScopeID)) return;
    let stopped = false;
    setSummary(undefined);
    setFrames(undefined);
		setDetailedFrames(undefined);
    setRecords(undefined);
    setFailures(undefined);
    setSearchResults(undefined);
    setUsage(undefined);
    setUsageFrames(undefined);
    setUsageResponseRecords(undefined);
    setRegisteredSkills(undefined);
    setRange(undefined);
    setRangeRequest(undefined);
    setRangeError(undefined);
    setError(undefined);
    setScopeMismatch(false);
		Promise.all([getTraceAnalysisSummary(traceId, source).then(verifyScope), getTraceFrames(traceId, undefined, {}, "CANONICAL", source, "COMPACT", 1000).then(verifyScope)])
      .then(([summaryResult, framePage]) => { if (!stopped) { setSummary(summaryResult); setFrames(framePage); } })
      .catch((value) => { if (!stopped) reportError(value, true); });
    return () => { stopped = true; };
	}, [evidenceScopeKey, reportError, source === "TARGET" ? scopeGeneration : 0, state.valid, traceId, verifyScope, source]);

  const loadRecords = useCallback(() => {
    if (scopeMismatch || records || pending.has("record-facts")) return;
    begin("record-facts");
    Promise.all([
      getTraceRecords(traceId, undefined, {}, source).then(verifyScope),
      getTraceFailures(traceId, undefined, source).then(verifyScope),
    ]).then(([recordPage, failurePage]) => {
      setRecords(recordPage);
      setFailures(failurePage);
    }).catch((value) => reportError(value, true)).finally(() => end("record-facts"));
  }, [pending, records, reportError, scopeMismatch, source, traceId, verifyScope]);
  const loadUsage = useCallback(() => {
    if (!scopeMismatch && !usage && !pending.has("usage")) {
      begin("usage");
      const loadResponseRecords = async () => {
        const items: TraceRecord[] = [];
        let cursor: string | undefined;
        do {
          const page = await getTraceRecords(traceId, cursor, { types: ["MODEL_RESPONSE_RECEIVED"] }, source).then(verifyScope);
          items.push(...page.items);
          cursor = page.hasMore && page.nextCursor ? page.nextCursor : undefined;
        } while (cursor);
        return items;
      };
      void Promise.all([
        getTraceUsage(traceId, source).then(verifyScope),
        getTraceFrames(traceId, undefined, {}, "USAGE_DESC", source, "DETAILED").then(verifyScope),
        loadResponseRecords(),
      ]).then(([usageResult, contributorPage, responseRecords]) => {
        setUsage(usageResult);
        setUsageFrames(contributorPage);
        setUsageResponseRecords(responseRecords);
      }).catch((value) => reportError(value, true)).finally(() => end("usage"));
    }
  }, [pending, reportError, scopeMismatch, source, traceId, usage, verifyScope]);
	const loadDetailedFrames = useCallback(() => {
		if (scopeMismatch || detailedFrames || pending.has("detailed-frames")) return;
		begin("detailed-frames");
		void getTraceFrames(traceId, undefined, {}, "CANONICAL", source, "DETAILED").then(verifyScope).then(setDetailedFrames)
			.catch((value) => reportError(value, true)).finally(() => end("detailed-frames"));
	}, [detailedFrames, pending, reportError, scopeMismatch, source, traceId, verifyScope]);
  useEffect(() => {
    if (!summary) return;
    if (state.view === "records") loadRecords();
    if (state.view === "usage") loadUsage();
		if (state.view === "timeline") loadDetailedFrames();
	}, [loadDetailedFrames, loadRecords, loadUsage, state.view, summary]);

  const effectiveFailureId = state.failureId
    ?? (state.frameId ? undefined : summary?.terminalFailureId)
    ?? undefined;
  useEffect(() => {
    if (!summary || !effectiveFailureId || failures || scopeMismatch || state.view === "records") return;
    begin("failure-focus");
    void getTraceFailures(traceId, undefined, source).then(verifyScope).then(setFailures)
      .catch((value) => reportError(value, true)).finally(() => end("failure-focus"));
  }, [effectiveFailureId, failures, reportError, scopeMismatch, state.view, summary, traceId, verifyScope]);

	const compactSelectedFrame = frames?.items.find((frame) => frame.frameId === state.frameId);
	const selectedFrame = detailedFrames?.items.find((frame) => frame.frameId === state.frameId)
		?? usageFrames?.items.find((frame) => frame.frameId === state.frameId)
		?? compactSelectedFrame;
  const selectedFailure = failures?.items.find((failure) => failure.failureId === effectiveFailureId);
  const selectFrame = useCallback((frameId: string) => {
    const frame = frames?.items.find((item) => item.frameId === frameId)
      ?? usageFrames?.items.find((item) => item.frameId === frameId);
    select({ frameId, failureId: frame?.failureIds?.[0] });
  }, [frames, select, usageFrames]);
	useEffect(() => setRegisteredSkills(undefined), [source === "TARGET" ? scopeGeneration : 0, selectedFrame?.frameId, source]);
	useEffect(() => {
		if (scopeMismatch || !state.frameId || !compactSelectedFrame || detailedFrames?.items.some((frame) => frame.frameId === state.frameId) || pending.has("selected-frame-detail")) return;
		begin("selected-frame-detail");
		void getTraceFrames(traceId, undefined, { frameIds: [state.frameId] }, "CANONICAL", source, "DETAILED").then(verifyScope).then((page) => {
			setDetailedFrames((current) => current ? mergeFrames(current, page.items) : page);
		}).catch((value) => reportError(value, true)).finally(() => end("selected-frame-detail"));
	}, [compactSelectedFrame, detailedFrames, pending, reportError, scopeMismatch, source, state.frameId, traceId, verifyScope]);
  useEffect(() => {
    if (effectiveFailureId && selectedFailure?.frameId && selectedFailure.frameId !== state.frameId) {
      select({ frameId: selectedFailure.frameId, failureId: effectiveFailureId });
    }
  }, [effectiveFailureId, select, selectedFailure, state.frameId]);

  useEffect(() => {
	const names = selectedFrame?.skillNames ?? [];
	if (names.length === 0 || registeredSkills || scopeMismatch) return;
	if (source === "IMPORTED") {
		setRegisteredSkills(new Set());
		return;
	}
    let stopped = false;
    const load = async () => {
      const found = new Set<string>();
      let cursor: string | undefined;
      do {
		const page = await listSkills(cursor, 100);
		await requireCurrentTargetScope(page.targetScopeId, currentScopeID, refresh);
        for (const skill of page.items as SkillSummary[]) {
          if (names.includes(skill.registeredName)) found.add(skill.registeredName);
        }
        cursor = page.hasMore && page.nextCursor ? page.nextCursor : undefined;
      } while (cursor && found.size < names.length);
      if (!stopped) setRegisteredSkills(found);
    };
    void load().catch((value) => { if (!stopped) reportError(value); });
    return () => { stopped = true; };
	}, [currentScopeID, refresh, registeredSkills, reportError, scopeMismatch, selectedFrame, source]);
  const loadAncestry = useCallback(async (startingFrame: TraceFrame) => {
    const known = new Map((frames?.items ?? []).map((frame) => [frame.frameId, frame]));
    known.set(startingFrame.frameId, startingFrame);
    const added = [startingFrame];
    const visited = new Set<string>();
    let current: TraceFrame | undefined = startingFrame;
    while (current?.parentFrameId && !visited.has(current.parentFrameId)) {
      const parentFrameId: string = current.parentFrameId;
      visited.add(parentFrameId);
      let parent = known.get(parentFrameId);
      if (!parent) {
        const page: TraceAnalysisPage<TraceFrame> = await getTraceFrames(traceId, undefined, { frameIds: [parentFrameId] }, "CANONICAL", source).then(verifyScope);
        parent = page.items[0];
        if (!parent) break;
        known.set(parent.frameId, parent);
        added.push(parent);
      }
      current = parent;
    }
    setFrames((value) => mergeFrames(value, added));
    return startingFrame;
  }, [frames, source, traceId, verifyScope]);
  useEffect(() => {
    if (scopeMismatch || !frames || !state.frameId || selectedFrame || pending.has("deep-frame")) return;
    begin("deep-frame");
    void getTraceFrames(traceId, undefined, { frameIds: [state.frameId] }, "CANONICAL", source).then(verifyScope).then((page) => {
      const frame = page.items[0];
      if (frame) return loadAncestry(frame);
      select({ frameId: undefined, failureId: undefined });
      return undefined;
    }).catch((value) => reportError(value, true)).finally(() => end("deep-frame"));
  }, [frames, loadAncestry, pending, reportError, scopeMismatch, select, selectedFrame, state.frameId, traceId, verifyScope]);
  useEffect(() => {
    if (scopeMismatch || !records || !state.recordSequence || records.items.some((record) => record.sequence === state.recordSequence) || pending.has("deep-record")) return;
    begin("deep-record");
    void getTraceRecords(traceId, undefined, { minSequence: state.recordSequence, maxSequence: state.recordSequence }, source).then(verifyScope).then((page) => {
      const record = page.items[0];
      if (record) setRecords((current) => current ? { ...current, items: [...current.items, record] } : page);
      else select({ recordSequence: undefined });
    }).catch((value) => reportError(value, true)).finally(() => end("deep-record"));
  }, [pending, records, reportError, scopeMismatch, select, source, state.recordSequence, traceId, verifyScope]);
  useEffect(() => {
    if (!scopeMismatch && failures && state.failureId && !failures.items.some((failure) => failure.failureId === state.failureId) && !failures.hasMore) {
      select({ failureId: undefined });
    }
  }, [failures, scopeMismatch, select, state.failureId]);
  const breadcrumbs = useMemo(() => {
    if (!selectedFrame || !frames) return [];
    const byID = new Map(frames.items.map((frame) => [frame.frameId, frame]));
    const result: TraceFrame[] = [];
    const visited = new Set<string>();
    let current: TraceFrame | undefined = selectedFrame;
    while (current && !visited.has(current.frameId)) {
      visited.add(current.frameId);
      result.unshift(current);
      current = current.parentFrameId ? byID.get(current.parentFrameId) : undefined;
    }
    return result;
  }, [frames, selectedFrame]);
  const selectRelatedFrame = (filter: TraceFrameFilter) => {
    if (pending.has("related-frame")) return;
    begin("related-frame");
    void getTraceFrames(traceId, undefined, filter, "CANONICAL", source).then(verifyScope).then((page) => {
      const frame = page.items[0];
      if (!frame) {
        setError("No frame is mechanically related to that evidence.");
        return;
      }
      return loadAncestry(frame).then(() => select({ frameId: frame.frameId, failureId: frame.failureIds?.[0] }));
    }).catch((value) => reportError(value, true)).finally(() => end("related-frame"));
  };
  const selectFrameRecord = (frameId: string) => {
    if (pending.has("frame-record")) return;
    begin("frame-record");
    const findRecord = async () => {
      const opened = await getTraceRecords(traceId, undefined, { types: ["FRAME_OPENED"], frameId }, source).then(verifyScope);
      if (opened.items[0]) return opened.items[0];
      const related = await getTraceRecords(traceId, undefined, { frameId }, source).then(verifyScope);
      return related.items.sort((left, right) => left.sequence - right.sequence)[0];
    };
    void findRecord().then((record) => {
      if (!record) {
        setError("No record is available for that frame.");
        return;
      }
      setRecords((current) => {
        if (!current || current.items.some((candidate) => candidate.sequence === record.sequence)) return current;
        return { ...current, items: [...current.items, record].sort((left, right) => left.sequence - right.sequence) };
      });
      select({ view: "records", recordSequence: record.sequence, frameId: record.frameId, failureId: undefined });
    }).catch((value) => reportError(value, true)).finally(() => end("frame-record"));
  };
  const readContent = (contentRef: string, cursor?: string, recordSequence?: number) => {
    if (pending.has("range")) return;
    setRangeRequest({ contentRef, recordSequence });
    setRangeError(undefined);
    if (!cursor) setRange(undefined);
    begin("range");
    void getContentRange(traceId, contentRef, cursor, source).then(verifyScope).then((result) => {
      setRange(result);
    }).catch((value) => {
      setRangeError(value instanceof Error ? value.message : "The content could not be read.");
      reportError(value, false);
    }).finally(() => end("range"));
  };
  const nextRange = () => {
    if (!range?.hasMore || !range.nextCursor) return;
    if (rangeRequest) readContent(rangeRequest.contentRef, range.nextCursor, rangeRequest.recordSequence);
  };
  const clearRange = () => { setRange(undefined); setRangeRequest(undefined); setRangeError(undefined); };
  const rangeBelongsToRecord = rangeRequest?.recordSequence !== undefined;
  const search = () => {
    if (!searchText || pending.has("search")) return;
    begin("search");
    void searchTraceEvidence(traceId, searchText, undefined, source).then(verifyScope).then(setSearchResults).catch((value) => reportError(value, true)).finally(() => end("search"));
  };
  const loadMore = <T, P extends TraceAnalysisPage<T> = TraceAnalysisPage<T>>(key: string, current: P | undefined, request: (cursor: string) => Promise<P>, setter: (value: P) => void) => {
    if (!current?.hasMore || !current.nextCursor || pending.has(key)) return;
    begin(key);
    void request(current.nextCursor).then(verifyScope).then((next) => setter(appendPage(current, next))).catch((value) => reportError(value, true)).finally(() => end(key));
  };
  useEffect(() => {
    if (!failures || !effectiveFailureId || failures.items.some((failure) => failure.failureId === effectiveFailureId) || !failures.hasMore || !failures.nextCursor || pending.has("deep-failure")) return;
    begin("deep-failure");
    void getTraceFailures(traceId, failures.nextCursor, source).then(verifyScope).then((next) => setFailures(appendPage(failures, next))).catch((value) => reportError(value, true)).finally(() => end("deep-failure"));
  }, [effectiveFailureId, failures, pending, reportError, source, traceId, verifyScope]);
  const handleTabKey = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    let nextIndex: number | undefined;
    if (event.key === "ArrowRight" || event.key === "ArrowDown") nextIndex = (index + 1) % views.length;
    if (event.key === "ArrowLeft" || event.key === "ArrowUp") nextIndex = (index - 1 + views.length) % views.length;
    if (event.key === "Home") nextIndex = 0;
    if (event.key === "End") nextIndex = views.length - 1;
    if (nextIndex === undefined) return;
    event.preventDefault();
    const nextView = views[nextIndex];
    select({ view: nextView });
    const tabs = event.currentTarget.closest('[role="tablist"]')?.querySelectorAll<HTMLButtonElement>('[role="tab"]');
    tabs?.[nextIndex]?.focus();
  };
  const [failureFocusRequest, setFailureFocusRequest] = useState<{ view: TraceExplorerView; token: number }>();
  const [failurePanelFocusRequest, setFailurePanelFocusRequest] = useState(0);
  const showFailureView = useCallback((view: TraceExplorerView) => {
    setFailureFocusRequest((current) => ({ view, token: (current?.token ?? 0) + 1 }));
    select({ view, failureId: effectiveFailureId });
  }, [effectiveFailureId, select]);
  const viewFailure = useCallback((failureId: string) => {
    select({ failureId });
    setFailurePanelFocusRequest((request) => request + 1);
  }, [select]);
  useEffect(() => {
    if (!failureFocusRequest || failureFocusRequest.view !== state.view) return;
    const frameButton = state.view === "hierarchy" && state.frameId
      ? [...document.querySelectorAll<HTMLButtonElement>("button[data-frame]")]
        .find((button) => button.dataset.frame === state.frameId)
      : undefined;
    (frameButton ?? document.getElementById(`trace-panel-${state.view}`))?.focus();
    setFailureFocusRequest(undefined);
  }, [failureFocusRequest, frames, state.frameId, state.view]);
  useEffect(() => {
    if (failurePanelFocusRequest === 0) return;
    document.getElementById("trace-failure-panel")?.focus();
    setFailurePanelFocusRequest(0);
  }, [failurePanelFocusRequest, selectedFailure]);
  const hasFailurePanel = Boolean(((summary?.outcome === "FAILED" || summary?.outcome === "ABORTED") && summary.terminalFailureId) || selectedFailure);

  return <section className="trace-explorer" aria-labelledby="trace-explorer-title">
    <h3 id="trace-explorer-title">Trace explorer</h3>
    <div aria-live="polite" aria-atomic="true">{error && <p className="target-error" role="alert">{error}</p>}</div>
    {!summary ? <p role="status">Loading trace evidence&hellip;</p> : <>
      <p>{summary.outcome} &middot; {summary.frameCount} frames &middot; {summary.recordCount} records{!summary.usageComplete && " · usage incomplete"}</p>
      {hasFailurePanel && <section id="trace-failure-panel" className="trace-failure-panel" aria-label="Trace failure details" tabIndex={-1}>
        <TraceFailureFocus summary={summary} failure={selectedFailure} frame={selectedFrame} onView={showFailureView} />
        <TraceFailureDiagnostic traceId={traceId} source={source} failure={selectedFailure} scopeGeneration={source === "TARGET" ? scopeGeneration : 0} verifyScope={verifyScope} />
      </section>}
      {breadcrumbs.length > 0 && <nav aria-label="Selected frame breadcrumbs">{breadcrumbs.map((frame, index) => <span key={frame.frameId}>{index > 0 && " / "}<button type="button" onClick={() => selectFrame(frame.frameId)}>{frame.route || frame.frameId}</button></span>)}</nav>}
      {selectedFrame && <section aria-labelledby="selected-frame-skills"><h4 id="selected-frame-skills">Recorded skill names</h4>{(selectedFrame.skillNames?.length ?? 0) === 0 ? <p>No recorded skill name is associated with this frame.</p> : <ul>{selectedFrame.skillNames.map((name) => <li key={name}>{registeredSkills?.has(name) ? <Link to={scopeBoundPath(`/skills/${encodeURIComponent(name)}`, currentScopeID)}>{name}</Link> : <><code>{name}</code> <span>not in current registered catalog</span></>}</li>)}</ul>}</section>}
      <div role="tablist" aria-label="Trace evidence views">{views.map((view, index) => <button id={`trace-tab-${view}`} aria-controls={`trace-panel-${view}`} key={view} type="button" role="tab" tabIndex={state.view === view ? 0 : -1} aria-selected={state.view === view} onKeyDown={(event) => handleTabKey(event, index)} onClick={() => select({ view })}>{view[0].toUpperCase() + view.slice(1)}</button>)}</div>
      <div id={`trace-panel-${state.view}`} role="tabpanel" aria-labelledby={`trace-tab-${state.view}`} tabIndex={0}>
        {state.view === "hierarchy" && <><TraceHierarchy frames={frames?.items ?? []} selectedFrameId={state.frameId} onSelect={selectFrame} />{frames?.hasMore && <button type="button" disabled={pending.has("frames")} onClick={() => loadMore("frames", frames, (cursor) => getTraceFrames(traceId, cursor, {}, "CANONICAL", source, "COMPACT", 1000), setFrames)}>Load more frames</button>}</>}
		{state.view === "timeline" && <><TraceTimeline frames={detailedFrames?.items ?? []} selectedFrameId={state.frameId} onSelect={selectFrame} />{detailedFrames?.hasMore && <button type="button" disabled={pending.has("detailed-frames-page")} onClick={() => loadMore("detailed-frames-page", detailedFrames, (cursor) => getTraceFrames(traceId, cursor, {}, "CANONICAL", source, "DETAILED"), setDetailedFrames)}>Load more timeline frames</button>}</>}
        {state.view === "usage" && <TraceUsage usage={usage} frame={selectedFrame} summary={summary} contributors={usageFrames?.items} responseRecords={usageResponseRecords} recordHref={(record) => {
          const target = setTraceExplorerSelection(params, { view: "records", frameId: record.frameId || undefined, recordSequence: record.sequence, failureId: undefined });
          return `?${target.toString()}`;
        }} />}
        {state.view === "records" && <>
          <form onSubmit={(event) => { event.preventDefault(); search(); }}><label>Literal search <input value={searchText} onChange={(event) => setSearchText(event.target.value)} /></label><button type="submit" disabled={!searchText || pending.has("search")}>Search</button></form>
          {searchResults && <section aria-label="Literal search results"><p role="status">{searchResults.items.length} literal matches</p>{searchResults.search && <p>{searchResults.search.workComplete ? "Search complete" : "Search has more work"}; {searchResults.search.caseSensitive ? "case-sensitive" : "case-insensitive"}; fields {searchResults.search.searchedFields.join(", ")}{searchResults.search.limitations.map((value) => `; ${value.code}: ${value.message}`).join("")}</p>}<ol>{searchResults.items.map((match) => {
            const contentRef = match.contentId ? searchResults.contentDescriptors.find((descriptor) => descriptor.contentId === match.contentId)?.contentRef : undefined;
            return <li key={`${match.sequence}-${match.searchedField}-${match.matchOffset}`}><button type="button" onClick={() => select({ view: "records", recordSequence: match.sequence, frameId: match.frameId || undefined, failureId: undefined })}>{match.recordType} record {match.sequence}</button> &middot; {match.searchedField} bytes {match.matchOffset}&ndash;{match.matchOffset + match.matchLength}{contentRef && <> &middot; <button type="button" onClick={() => readContent(contentRef)}>Read match content</button></>}</li>;
          })}</ol>{searchResults.hasMore && <button type="button" disabled={pending.has("search-page")} onClick={() => {
            if (!searchResults.nextCursor || pending.has("search-page")) return;
            begin("search-page");
            void searchTraceEvidence(traceId, searchText, searchResults.nextCursor, source).then(verifyScope).then((next) => setSearchResults(appendSearchPage(searchResults, next))).catch((value) => reportError(value, true)).finally(() => end("search-page"));
          }}>Load more matches</button>}</section>}
          <TraceRecords traceId={traceId} source={source} records={records?.items ?? []} frames={frames?.items ?? []} failures={failures?.items ?? []} selectedRecordSequence={state.recordSequence} selectedFailureId={state.failureId} activeContentRecordSequence={rangeRequest?.recordSequence} contentRange={range} contentPending={pending.has("range")} contentError={rangeError} onSelectRecord={(record) => select({ recordSequence: record.sequence, frameId: record.frameId || undefined, failureId: undefined })} onSelectFailure={viewFailure} onRelatedFrame={selectRelatedFrame} onFrameRecord={selectFrameRecord} onContent={(contentRef, recordSequence) => readContent(contentRef, undefined, recordSequence)} onNextContent={nextRange} onClearContent={clearRange} />
          <div className="trace-continuations" role="group" aria-label="Additional evidence pages">
            {records?.hasMore && <button type="button" disabled={pending.has("records")} onClick={() => loadMore("records", records, (cursor) => getTraceRecords(traceId, cursor, {}, source), setRecords)}>Load more records</button>}
          </div>
        </>}
      </div>
      {!rangeBelongsToRecord && <TraceEvidenceDetail range={range} pending={pending.has("range")} error={rangeError} onNext={nextRange} onClear={clearRange} />}
    </>}
  </section>;
}
