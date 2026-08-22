package observability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

const (
	defaultPageSize               = 1000
	maxPageSize                   = 5000
	collectionMaxBytes            = 16 * 1024 * 1024
	skillDetailMaxBytes           = 4 * 1024 * 1024
	activeExecutionDetailMaxBytes = 4 * 1024 * 1024
	traceDetailMaxBytes           = 1 * 1024 * 1024
	instanceMaxBytes              = 1 * 1024 * 1024
)

type Service struct{}

func New() *Service {
	return &Service{}
}

func (service *Service) GetInstance(ctx context.Context, scope target.Scope) (InstanceStatus, *consolecore.Error) {
	endpoint := scope.Target.InstanceEndpoint()
	body, domain := scope.Upstream(ctx, endpoint, instanceMaxBytes)
	if domain != nil {
		return InstanceStatus{}, domain
	}
	var status InstanceStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return InstanceStatus{}, consolecore.NewError(consolecore.CodeConsoleError, "The instance status response could not be read.", string(scope.ID), consolecore.Details{}, err)
	}
	if err := validateInstanceStatus(status, scope.InstanceID); err != nil {
		return InstanceStatus{}, invalidUpstreamResponse(scope, "instance status", err)
	}
	status.TargetScopeID = string(scope.ID)
	if domain := scope.RequireCurrent(); domain != nil {
		return InstanceStatus{}, domain
	}
	return status, nil
}

func (service *Service) ListSkills(ctx context.Context, scope target.Scope, request ListRequest) (Page[SkillSummary], *consolecore.Error) {
	pageSize, domain := clampPageSize(request.PageSize)
	if domain != nil {
		return Page[SkillSummary]{}, domain
	}
	endpoint := buildCollectionURL(scope.Target.SkillsEndpoint(), request.Cursor, pageSize)
	body, domain := scope.Upstream(ctx, endpoint, collectionMaxBytes)
	if domain != nil {
		return Page[SkillSummary]{}, domain
	}
	var page Page[SkillSummary]
	if err := json.Unmarshal(body, &page); err != nil {
		return Page[SkillSummary]{}, consolecore.NewError(consolecore.CodeConsoleError, "The skills response could not be read.", string(scope.ID), consolecore.Details{}, err)
	}
	if err := validatePage(page, validateSkillSummary); err != nil {
		return Page[SkillSummary]{}, invalidUpstreamResponse(scope, "skills", err)
	}
	page.TargetScopeID = string(scope.ID)
	if domain := scope.RequireCurrent(); domain != nil {
		return Page[SkillSummary]{}, domain
	}
	return page, nil
}

func (service *Service) GetSkill(ctx context.Context, scope target.Scope, registeredName string) (SkillDetail, *consolecore.Error) {
	if registeredName == "" {
		return SkillDetail{}, consolecore.NewError(consolecore.CodeInvalidArgument, "A skill name is required.", string(scope.ID), consolecore.Details{}, nil)
	}
	endpoint := scope.Target.SkillEndpoint(registeredName)
	body, domain := scope.Upstream(ctx, endpoint, skillDetailMaxBytes)
	if domain != nil {
		return SkillDetail{}, domain
	}
	var detail SkillDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return SkillDetail{}, consolecore.NewError(consolecore.CodeConsoleError, "The skill detail response could not be read.", string(scope.ID), consolecore.Details{}, err)
	}
	if err := validateSkillDetail(detail, registeredName); err != nil {
		return SkillDetail{}, invalidUpstreamResponse(scope, "skill detail", err)
	}
	detail.TargetScopeID = string(scope.ID)
	if domain := scope.RequireCurrent(); domain != nil {
		return SkillDetail{}, domain
	}
	return detail, nil
}

func (service *Service) ListActiveExecutions(ctx context.Context, scope target.Scope, request ListRequest) (ActivePage, *consolecore.Error) {
	pageSize, domain := clampPageSize(request.PageSize)
	if domain != nil {
		return ActivePage{}, domain
	}
	endpoint := buildCollectionURL(scope.Target.ActiveExecutionsEndpoint(), request.Cursor, pageSize)
	body, domain := scope.Upstream(ctx, endpoint, collectionMaxBytes)
	if domain != nil {
		return ActivePage{}, domain
	}
	if err := validateActivePageJSON(body); err != nil {
		return ActivePage{}, invalidUpstreamResponse(scope, "active executions", err)
	}
	var page ActivePage
	if err := json.Unmarshal(body, &page); err != nil {
		return ActivePage{}, consolecore.NewError(consolecore.CodeConsoleError, "The active executions response could not be read.", string(scope.ID), consolecore.Details{}, err)
	}
	if err := validatePage(page.Page, validateActiveExecution); err != nil {
		return ActivePage{}, invalidUpstreamResponse(scope, "active executions", err)
	}
	page.TargetScopeID = string(scope.ID)
	for index := range page.Items {
		page.Items[index].TargetScopeID = string(scope.ID)
	}
	if domain := scope.RequireCurrent(); domain != nil {
		return ActivePage{}, domain
	}
	return page, nil
}

func (service *Service) GetActiveExecution(ctx context.Context, scope target.Scope, sessionId string) (ActiveExecution, *consolecore.Error) {
	if sessionId == "" {
		return ActiveExecution{}, consolecore.NewError(consolecore.CodeInvalidArgument, "A session ID is required.", string(scope.ID), consolecore.Details{}, nil)
	}
	endpoint := scope.Target.ActiveExecutionEndpoint(sessionId)
	body, domain := scope.Upstream(ctx, endpoint, activeExecutionDetailMaxBytes)
	if domain != nil {
		return ActiveExecution{}, domain
	}
	if err := validateActiveExecutionJSON(body); err != nil {
		return ActiveExecution{}, invalidUpstreamResponse(scope, "active execution detail", err)
	}
	var execution ActiveExecution
	if err := json.Unmarshal(body, &execution); err != nil {
		return ActiveExecution{}, consolecore.NewError(consolecore.CodeConsoleError, "The active execution detail response could not be read.", string(scope.ID), consolecore.Details{}, err)
	}
	if err := validateActiveExecution(execution); err != nil || execution.SessionID != sessionId {
		if err == nil {
			err = errors.New("session ID does not match the requested resource")
		}
		return ActiveExecution{}, invalidUpstreamResponse(scope, "active execution detail", err)
	}
	execution.TargetScopeID = string(scope.ID)
	if domain := scope.RequireCurrent(); domain != nil {
		return ActiveExecution{}, domain
	}
	return execution, nil
}

func (service *Service) ListTraces(ctx context.Context, scope target.Scope, request ListRequest) (Page[Trace], *consolecore.Error) {
	pageSize, domain := clampPageSize(request.PageSize)
	if domain != nil {
		return Page[Trace]{}, domain
	}
	endpoint := buildCollectionURL(scope.Target.TracesEndpoint(), request.Cursor, pageSize)
	body, domain := scope.Upstream(ctx, endpoint, collectionMaxBytes)
	if domain != nil {
		return Page[Trace]{}, domain
	}
	var page Page[Trace]
	if err := json.Unmarshal(body, &page); err != nil {
		return Page[Trace]{}, consolecore.NewError(consolecore.CodeConsoleError, "The traces response could not be read.", string(scope.ID), consolecore.Details{}, err)
	}
	if err := validatePage(page, validateTrace); err != nil {
		return Page[Trace]{}, invalidUpstreamResponse(scope, "traces", err)
	}
	page.TargetScopeID = string(scope.ID)
	for index := range page.Items {
		page.Items[index].TargetScopeID = string(scope.ID)
	}
	if domain := scope.RequireCurrent(); domain != nil {
		return Page[Trace]{}, domain
	}
	return page, nil
}

func (service *Service) GetTrace(ctx context.Context, scope target.Scope, traceId string) (Trace, *consolecore.Error) {
	if traceId == "" {
		return Trace{}, consolecore.NewError(consolecore.CodeInvalidArgument, "A trace ID is required.", string(scope.ID), consolecore.Details{}, nil)
	}
	endpoint := scope.Target.TraceEndpoint(traceId)
	body, domain := scope.Upstream(ctx, endpoint, traceDetailMaxBytes)
	if domain != nil {
		return Trace{}, domain
	}
	var trace Trace
	if err := json.Unmarshal(body, &trace); err != nil {
		return Trace{}, consolecore.NewError(consolecore.CodeConsoleError, "The trace detail response could not be read.", string(scope.ID), consolecore.Details{}, err)
	}
	if err := validateTrace(trace); err != nil || trace.TraceID != traceId {
		if err == nil {
			err = errors.New("trace ID does not match the requested resource")
		}
		return Trace{}, invalidUpstreamResponse(scope, "trace detail", err)
	}
	trace.TargetScopeID = string(scope.ID)
	if domain := scope.RequireCurrent(); domain != nil {
		return Trace{}, domain
	}
	return trace, nil
}

func clampPageSize(requested int) (int, *consolecore.Error) {
	if requested < 0 {
		return 0, consolecore.NewError(consolecore.CodeInvalidArgument, "Page size must not be negative.", "", consolecore.Details{}, nil)
	}
	if requested == 0 {
		return defaultPageSize, nil
	}
	if requested > maxPageSize {
		return maxPageSize, nil
	}
	return requested, nil
}

func buildCollectionURL(base, cursor string, pageSize int) string {
	params := url.Values{}
	params.Set("pageSize", fmt.Sprintf("%d", pageSize))
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	if strings.Contains(base, "?") {
		return base + "&" + params.Encode()
	}
	return base + "?" + params.Encode()
}

func invalidUpstreamResponse(scope target.Scope, resource string, err error) *consolecore.Error {
	return consolecore.NewError(
		consolecore.CodeConsoleError,
		"The "+resource+" response was invalid.",
		string(scope.ID),
		consolecore.Details{},
		err,
	)
}

func validateInstanceStatus(status InstanceStatus, expectedInstanceID string) error {
	if status.InstanceID == "" || status.InstanceID != expectedInstanceID {
		return errors.New("instance ID is missing or does not match the response header")
	}
	if status.ConsoleCompatibilityVersion == "" || status.ObservedAt.IsZero() {
		return errors.New("compatibility version or observation time is missing")
	}
	if status.RegisteredSkillCount < 0 || status.ActiveExecutionCount < 0 || status.CatalogedTraceCount < 0 {
		return errors.New("instance counts must not be negative")
	}
	if status.TracePersistencePolicy == "" || status.CompletionGraceTtl == "" || status.TraceCatalogMetadataTtl == "" {
		return errors.New("retention metadata is missing")
	}
	return nil
}

func validatePage[T any](page Page[T], validateItem func(T) error) error {
	if page.Items == nil || page.ObservedAt.IsZero() {
		return errors.New("items or observation time is missing")
	}
	if page.HasMore {
		if page.NextCursor == nil || *page.NextCursor == "" {
			return errors.New("a continuing page must provide a next cursor")
		}
	} else if page.NextCursor != nil {
		return errors.New("a final page must not provide a next cursor")
	}
	for _, item := range page.Items {
		if err := validateItem(item); err != nil {
			return err
		}
	}
	return nil
}

func validateSkillSummary(skill SkillSummary) error {
	if skill.RegisteredName == "" || skill.SourcePath == "" {
		return errors.New("skill identity or source path is missing")
	}
	return nil
}

func validateSkillDetail(skill SkillDetail, requestedName string) error {
	if err := validateSkillSummary(SkillSummary{
		RegisteredName: skill.RegisteredName,
		SourcePath:     skill.SourcePath,
	}); err != nil {
		return err
	}
	if skill.RegisteredName != requestedName {
		return errors.New("registered name does not match the requested resource")
	}
	if skill.Yaml == "" {
		return errors.New("skill YAML is missing")
	}
	return nil
}

func validateActiveExecution(execution ActiveExecution) error {
	if execution.SessionID == "" || execution.TraceID == "" || execution.EntrySkill == "" {
		return errors.New("execution identity is missing")
	}
	if execution.StartedAt.IsZero() || execution.UpdatedAt.IsZero() ||
		execution.UpdatedAt.Before(execution.StartedAt) {
		return errors.New("execution timestamps are missing or inconsistent")
	}
	if execution.LastCanonicalSequence < 0 || execution.ElapsedMillis < 0 ||
		execution.TotalFrameDepth < 0 || execution.Status == "" || execution.Phase == "" ||
		execution.ActivePath == nil {
		return errors.New("execution state is incomplete or invalid")
	}
	usage := execution.Usage
	if usage.SkillInvocations < 0 || usage.ToolInvocations < 0 || usage.LinterRetries < 0 ||
		usage.ModelCalls < 0 || usage.ProviderAttempts < 0 || usage.PromptUnits < 0 ||
		usage.CompletionUnits < 0 || usage.UsageUnits < 0 || usage.ExactModelResponses < 0 ||
		usage.HeuristicModelResponses < 0 || usage.UnavailableModelResponses < 0 {
		return errors.New("execution usage must not be negative")
	}
	limits := execution.ConfiguredLimits
	if limits.MaxSkillInvocations < 0 || limits.MaxToolInvocations < 0 || limits.MaxLinterRetries < 0 ||
		limits.MaxModelCalls < 0 || limits.MaxProviderAttempts < 0 || limits.MaxUsageUnits < 0 {
		return errors.New("execution configured limits must not be negative")
	}
	return nil
}

var requiredUsageMembers = []string{
	"skillInvocations", "toolInvocations", "linterRetries", "modelCalls", "providerAttempts",
	"promptUnits", "completionUnits", "usageUnits", "exactModelResponses", "heuristicModelResponses",
	"unavailableModelResponses",
}

var requiredLimitMembers = []string{
	"maxSkillInvocations", "maxToolInvocations", "maxLinterRetries", "maxModelCalls",
	"maxProviderAttempts", "maxUsageUnits",
}

func validateActivePageJSON(body []byte) error {
	var page struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return err
	}
	for index, item := range page.Items {
		if err := validateActiveExecutionJSON(item); err != nil {
			return fmt.Errorf("item %d: %w", index, err)
		}
	}
	return nil
}

func validateActiveExecutionJSON(body []byte) error {
	var execution map[string]json.RawMessage
	if err := json.Unmarshal(body, &execution); err != nil {
		return err
	}
	if err := requireNonnegativeIntegerMembers(execution["usage"], "usage", requiredUsageMembers); err != nil {
		return err
	}
	return requireNonnegativeIntegerMembers(execution["configuredLimits"], "configuredLimits", requiredLimitMembers)
}

func requireNonnegativeIntegerMembers(raw json.RawMessage, objectName string, names []string) error {
	if len(raw) == 0 || string(raw) == "null" {
		return fmt.Errorf("%s is missing", objectName)
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return fmt.Errorf("%s is invalid: %w", objectName, err)
	}
	for _, name := range names {
		valueRaw, ok := members[name]
		if !ok || string(valueRaw) == "null" {
			return fmt.Errorf("%s.%s is missing", objectName, name)
		}
		var value int
		if err := json.Unmarshal(valueRaw, &value); err != nil || value < 0 {
			return fmt.Errorf("%s.%s must be a nonnegative integer", objectName, name)
		}
	}
	return nil
}

func validateTrace(trace Trace) error {
	if trace.TraceID == "" || trace.SessionID == "" || trace.EntrySkill == "" || trace.Outcome == "" ||
		trace.PersistencePolicy == "" {
		return errors.New("trace identity or state is missing")
	}
	if trace.FinalizedAt.IsZero() || trace.ApplicationTraceExpiresAt.IsZero() ||
		trace.ApplicationTraceExpiresAt.Before(trace.FinalizedAt) || trace.SizeBytes < 0 {
		return errors.New("trace timestamps or size are invalid")
	}
	return nil
}
