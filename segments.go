package warmbly

import (
	"context"
	"net/url"
	"time"
)

// SegmentService manages segments: saved, reusable contact audiences. A
// segment is a list of conditions over contacts plus per-contact manual
// overrides. Membership is evaluated live on every read, so a segment never
// needs rebuilding and its counts are always current.
//
// A contact is a member when it matches the conditions (per [Segment.Match])
// or is manually included, and is not manually excluded. A segment with no
// conditions holds only its manual includes.
//
// Segments read and write under the contact scopes, since a segment is a view
// over contacts. The one exception is [SegmentService.AddToCampaign], which
// writes leads and therefore takes the campaign write scope.
//
// Two ways to feed a campaign from a segment:
//
//   - [SegmentService.AddToCampaign] is a snapshot: it enrolls the members at
//     the moment of the call and nothing more.
//   - Linking the segment to the campaign (PUT /campaigns/:id/segments, see
//     [CampaignService]) is a live audience: members are enrolled immediately
//     and contacts that later join the segment are enrolled as they appear,
//     keeping a continuous campaign fed without further calls.
//
// Contact search and export accept segment IDs to scope any contact query to
// a segment, and GET /contacts/:id/segments gives the contact-side view.
type SegmentService service

// SegmentMatch says how a segment combines its conditions.
type SegmentMatch string

const (
	// SegmentMatchAll requires every condition to hold (AND). The default.
	SegmentMatchAll SegmentMatch = "all"
	// SegmentMatchAny requires at least one condition to hold (OR).
	SegmentMatchAny SegmentMatch = "any"
)

// SegmentMemberMode is a manual override on one contact's membership. It
// takes precedence over the conditions: an included contact is a member
// whether or not it matches, an excluded one never is.
type SegmentMemberMode string

const (
	// SegmentMemberInclude pins the contact into the segment.
	SegmentMemberInclude SegmentMemberMode = "include"
	// SegmentMemberExclude pins the contact out of the segment.
	SegmentMemberExclude SegmentMemberMode = "exclude"
	// SegmentMemberAuto clears the override so the conditions decide again.
	// It is only ever sent; a lookup never reports it.
	SegmentMemberAuto SegmentMemberMode = "auto"
)

// SegmentFieldKind groups filterable fields by the operators they accept. The
// per-kind operator sets are listed on the [SegmentOperator] constants.
type SegmentFieldKind string

const (
	// SegmentFieldText fields (first_name, last_name, email, email_domain,
	// phone, company, custom.*) take the text operators; comparisons ignore
	// case.
	SegmentFieldText SegmentFieldKind = "text"
	// SegmentFieldEnum fields (source, verification_status, esp_provider)
	// take in/not_in with Values drawn from [SegmentFieldSpec.Options].
	SegmentFieldEnum SegmentFieldKind = "enum"
	// SegmentFieldBool fields (subscribed, suppressed, is_catch_all) take
	// is_true/is_false and no value.
	SegmentFieldBool SegmentFieldKind = "bool"
	// SegmentFieldDate fields (created_at, updated_at, last_sent_at,
	// last_opened_at, last_clicked_at, last_replied_at) take within_days,
	// not_within_days, before, after, is_empty and is_not_empty.
	SegmentFieldDate SegmentFieldKind = "date"
	// SegmentFieldNumber fields (campaign_count, emails_sent, emails_opened,
	// emails_clicked, emails_replied, emails_bounced) take the comparison
	// operators with a whole-number Value. Engagement counters add up every
	// campaign the contact has been in; opens count human opens only.
	SegmentFieldNumber SegmentFieldKind = "number"
	// SegmentFieldCategory is the "category" field: in/not_in over category
	// IDs, plus is_empty/is_not_empty.
	SegmentFieldCategory SegmentFieldKind = "category"
	// SegmentFieldCampaign is the "campaign" field: in/not_in over campaign
	// IDs, plus is_empty/is_not_empty.
	SegmentFieldCampaign SegmentFieldKind = "campaign"
	// SegmentFieldSegment is the "segment" field: in/not_in over other
	// segment IDs. References nest at most five levels deep and may not form
	// a loop or point at the segment itself.
	SegmentFieldSegment SegmentFieldKind = "segment"
)

// SegmentOperator is the comparison a [SegmentCondition] applies. Which
// operators a field accepts depends on its [SegmentFieldKind]; the server
// rejects a mismatch with a 400 naming the offending condition.
type SegmentOperator string

// Scalar operators read [SegmentCondition.Value].
const (
	// SegmentOpEquals: text and number fields.
	SegmentOpEquals SegmentOperator = "equals"
	// SegmentOpNotEquals: text and number fields.
	SegmentOpNotEquals SegmentOperator = "not_equals"
	// SegmentOpContains: text fields, case-insensitive substring.
	SegmentOpContains SegmentOperator = "contains"
	// SegmentOpNotContains: text fields.
	SegmentOpNotContains SegmentOperator = "not_contains"
	// SegmentOpStartsWith: text fields.
	SegmentOpStartsWith SegmentOperator = "starts_with"
	// SegmentOpEndsWith: text fields.
	SegmentOpEndsWith SegmentOperator = "ends_with"
	// SegmentOpBefore: date fields; Value is YYYY-MM-DD or RFC 3339.
	SegmentOpBefore SegmentOperator = "before"
	// SegmentOpAfter: date fields; Value is YYYY-MM-DD or RFC 3339.
	SegmentOpAfter SegmentOperator = "after"
	// SegmentOpWithinDays: date fields; Value is a day count from 1 to 3650.
	SegmentOpWithinDays SegmentOperator = "within_days"
	// SegmentOpNotWithinDays: date fields; Value is a day count from 1 to
	// 3650. A contact with no date at all also matches.
	SegmentOpNotWithinDays SegmentOperator = "not_within_days"
	// SegmentOpGT: number fields.
	SegmentOpGT SegmentOperator = "gt"
	// SegmentOpGTE: number fields.
	SegmentOpGTE SegmentOperator = "gte"
	// SegmentOpLT: number fields.
	SegmentOpLT SegmentOperator = "lt"
	// SegmentOpLTE: number fields.
	SegmentOpLTE SegmentOperator = "lte"
)

// List operators read [SegmentCondition.Values] and need at least one entry.
const (
	// SegmentOpIn: enum, category, campaign and segment fields.
	SegmentOpIn SegmentOperator = "in"
	// SegmentOpNotIn: enum, category, campaign and segment fields.
	SegmentOpNotIn SegmentOperator = "not_in"
)

// Valueless operators ignore both Value and Values.
const (
	// SegmentOpIsEmpty: text, date, category and campaign fields.
	SegmentOpIsEmpty SegmentOperator = "is_empty"
	// SegmentOpIsNotEmpty: text, date, category and campaign fields.
	SegmentOpIsNotEmpty SegmentOperator = "is_not_empty"
	// SegmentOpIsTrue: bool fields.
	SegmentOpIsTrue SegmentOperator = "is_true"
	// SegmentOpIsFalse: bool fields.
	SegmentOpIsFalse SegmentOperator = "is_false"
)

// SegmentCustomFieldPrefix addresses a contact custom field in a condition:
// "custom.industry" filters on the custom field "industry". Custom fields are
// text fields. The workspace's current keys are returned by
// [SegmentService.Fields].
const SegmentCustomFieldPrefix = "custom."

// Segment limits enforced by the server.
const (
	// SegmentMaxConditions is the most conditions one segment may hold.
	SegmentMaxConditions = 50
	// SegmentMaxListValues is the most entries a list condition may hold.
	SegmentMaxListValues = 200
	// SegmentMaxMemberBatch is the most contact IDs one SetMembers or
	// MemberModes call accepts.
	SegmentMaxMemberBatch = 1000
	// SegmentMaxOverrides caps the Overrides listing.
	SegmentMaxOverrides = 500
)

// SegmentCondition is one predicate over a contact. Field names a filterable
// field (see [SegmentService.Fields]) or a custom field as
// [SegmentCustomFieldPrefix] plus its key; Operator picks the comparison.
// Scalar operators read Value, list operators read Values, and the valueless
// operators read neither. The server normalizes what it stores: dates become
// RFC 3339, numbers are canonicalized, and whichever of Value/Values the
// operator does not use is dropped.
type SegmentCondition struct {
	Field    string          `json:"field"`
	Operator SegmentOperator `json:"operator"`
	// Value is the scalar operand, always sent as a string: "30" for a day
	// count, "3" for a counter, "2026-01-15" for a date.
	Value string `json:"value,omitempty"`
	// Values is the list operand for in/not_in: enum options, or category,
	// campaign or segment IDs.
	Values []string `json:"values,omitempty"`
}

// Segment is a saved contact audience.
type Segment struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	CreatedBy      *string `json:"created_by,omitempty"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	// Color is a #rrggbb value, lower-cased by the server.
	Color      string             `json:"color"`
	Match      SegmentMatch       `json:"match"`
	Conditions []SegmentCondition `json:"conditions"`

	// ContactCount is the live membership size (conditions plus overrides).
	ContactCount int `json:"contact_count"`
	// IncludedCount is how many contacts are pinned in by hand.
	IncludedCount int `json:"included_count"`
	// ExcludedCount is how many contacts are pinned out by hand.
	ExcludedCount int `json:"excluded_count"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SegmentFieldSpec describes one field a condition may filter on. Kind
// determines the accepted operators and operand shape.
type SegmentFieldSpec struct {
	// Field is the value to put in [SegmentCondition.Field].
	Field string `json:"field"`
	// Label is the human-readable name, for a condition builder.
	Label string `json:"label"`
	// Group is the display group: "Contact", "Company", "Campaign
	// activity", "Email engagement", "Segments" or "Custom field".
	Group string           `json:"group"`
	Kind  SegmentFieldKind `json:"kind"`
	// Options lists the accepted Values of an enum field; nil otherwise.
	Options []string `json:"options,omitempty"`
}

// SegmentCreateParams creates a segment. Name is required; everything else
// has a server default (no description, color #0284c7, match all, no
// conditions). A name already used by another segment is rejected with a
// 409.
type SegmentCreateParams struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	// Color is a #rrggbb value.
	Color      *string            `json:"color,omitempty"`
	Match      *SegmentMatch      `json:"match,omitempty"`
	Conditions []SegmentCondition `json:"conditions,omitempty"`
}

// SegmentUpdateParams changes a segment. Nil fields are unchanged.
type SegmentUpdateParams struct {
	Name        *string       `json:"name,omitempty"`
	Description *string       `json:"description,omitempty"`
	Color       *string       `json:"color,omitempty"`
	Match       *SegmentMatch `json:"match,omitempty"`
	// Conditions replaces the whole condition list. A nil slice leaves the
	// conditions unchanged; an empty, non-nil slice ([]SegmentCondition{})
	// removes them all, leaving a segment of manual includes only.
	Conditions []SegmentCondition `json:"conditions"`
}

// SegmentPreviewParams is an unsaved definition to count.
type SegmentPreviewParams struct {
	// ID, when set, keeps that segment's manual overrides in the count, so
	// editing an existing segment previews the number its members will see.
	// It also lets the definition be checked for self-reference.
	ID *string `json:"id,omitempty"`
	// Match defaults to [SegmentMatchAll] when empty.
	Match      SegmentMatch       `json:"match,omitempty"`
	Conditions []SegmentCondition `json:"conditions"`
}

// SegmentPreviewResult is the answer to a preview.
type SegmentPreviewResult struct {
	// ContactCount is how many contacts the definition matches right now.
	ContactCount int `json:"contact_count"`
}

// SegmentOverride is one contact pinned into or out of a segment by hand.
type SegmentOverride struct {
	ContactID string `json:"contact_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Company   string `json:"company"`
	// Mode is [SegmentMemberInclude] or [SegmentMemberExclude].
	Mode SegmentMemberMode `json:"mode"`
	// CreatedAt is when the override was set.
	CreatedAt time.Time `json:"created_at"`
}

// SegmentAddToCampaignResult reports a snapshot enrollment.
type SegmentAddToCampaignResult struct {
	CampaignID string `json:"campaign_id"`
	// Added is how many leads were new to the campaign.
	Added int `json:"added"`
	// Members is the segment's membership size at enroll time; Members minus
	// Added were already in the campaign.
	Members int `json:"members"`
}

// List returns every segment in the organization with live counts. The
// endpoint is not paginated: a workspace holds at most 200 segments.
func (s *SegmentService) List(ctx context.Context, opts ...RequestOption) ([]Segment, *Response, error) {
	return fetchData[Segment](ctx, s.client, "segments", opts)
}

// Fields returns every field a condition may filter on: the built-in
// catalog followed by the workspace's custom fields, each as
// "custom.<key>" with kind text. Use it to validate conditions client-side
// or to drive a condition builder.
func (s *SegmentService) Fields(ctx context.Context, opts ...RequestOption) ([]SegmentFieldSpec, *Response, error) {
	return fetchData[SegmentFieldSpec](ctx, s.client, "segments/fields", opts)
}

// Preview counts the contacts an unsaved definition matches, without
// creating anything. Invalid conditions come back as a 400 naming the
// condition, so it doubles as a validator.
func (s *SegmentService) Preview(ctx context.Context, params *SegmentPreviewParams, opts ...RequestOption) (*SegmentPreviewResult, *Response, error) {
	return send[SegmentPreviewResult](ctx, s.client.post, "segments/preview", params, opts)
}

// Create saves a segment and returns it with its initial live counts.
func (s *SegmentService) Create(ctx context.Context, params *SegmentCreateParams, opts ...RequestOption) (*Segment, *Response, error) {
	return send[Segment](ctx, s.client.post, "segments", params, opts)
}

// Get returns one segment with live counts.
func (s *SegmentService) Get(ctx context.Context, id string, opts ...RequestOption) (*Segment, *Response, error) {
	return fetch[Segment](ctx, s.client, "segments/"+url.PathEscape(id), opts)
}

// Update changes a segment's definition. Changing the conditions changes
// membership immediately, and any campaign the segment is linked to enrolls
// the new members on its next sync.
func (s *SegmentService) Update(ctx context.Context, id string, params *SegmentUpdateParams, opts ...RequestOption) (*Segment, *Response, error) {
	return send[Segment](ctx, s.client.patch, "segments/"+url.PathEscape(id), params, opts)
}

// Delete removes a segment and its overrides. Contacts are untouched. It
// fails with a 409 while another segment's conditions reference this one or
// a campaign has it linked as a live audience; detach it first.
func (s *SegmentService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "segments/"+url.PathEscape(id), opts...)
}

// SetMembers writes one manual override on a batch of contacts:
// [SegmentMemberInclude] pins them in, [SegmentMemberExclude] pins them out,
// [SegmentMemberAuto] clears any override so the conditions decide again. Up
// to [SegmentMaxMemberBatch] IDs per call; IDs outside the organization are
// ignored. It returns how many rows changed. Pinning in (or clearing an
// exclude) can admit new members, so linked campaigns are synced afterwards.
func (s *SegmentService) SetMembers(ctx context.Context, id string, contactIDs []string, mode SegmentMemberMode, opts ...RequestOption) (int, *Response, error) {
	body := struct {
		Contacts []string          `json:"contacts"`
		Mode     SegmentMemberMode `json:"mode"`
	}{Contacts: contactIDs, Mode: mode}
	var out struct {
		Updated int `json:"updated"`
	}
	resp, err := s.client.post(ctx, "segments/"+url.PathEscape(id)+"/members", body, &out, opts...)
	if err != nil {
		return 0, resp, err
	}
	return out.Updated, resp, nil
}

// MemberModes reports the manual override on each of the given contacts,
// keyed by contact ID. Contacts with no override are absent from the map, so
// a missing key means the conditions alone decide. Up to
// [SegmentMaxMemberBatch] IDs per call.
func (s *SegmentService) MemberModes(ctx context.Context, id string, contactIDs []string, opts ...RequestOption) (map[string]SegmentMemberMode, *Response, error) {
	body := struct {
		Contacts []string `json:"contacts"`
	}{Contacts: contactIDs}
	var out struct {
		Data map[string]SegmentMemberMode `json:"data"`
	}
	resp, err := s.client.post(ctx, "segments/"+url.PathEscape(id)+"/members/lookup", body, &out, opts...)
	if err != nil {
		return nil, resp, err
	}
	if out.Data == nil {
		out.Data = map[string]SegmentMemberMode{}
	}
	return out.Data, resp, nil
}

// Overrides lists every contact pinned into or out of the segment, includes
// first, newest first, capped at [SegmentMaxOverrides].
func (s *SegmentService) Overrides(ctx context.Context, id string, opts ...RequestOption) ([]SegmentOverride, *Response, error) {
	return fetchData[SegmentOverride](ctx, s.client, "segments/"+url.PathEscape(id)+"/overrides", opts)
}

// AddToCampaign enrolls every current member of the segment as a lead in the
// campaign. Contacts already in the campaign are skipped, each new lead gets
// a campaign_added activity, and a running campaign is woken so the leads
// are scheduled. Safe to retry.
//
// This is a snapshot: contacts that join the segment later are not added
// until the call is repeated. For a live link that keeps enrolling members
// as they appear, link the segment to the campaign instead (PUT
// /campaigns/:id/segments via [CampaignService]).
//
// Unlike the rest of the service this call takes the campaign write scope.
func (s *SegmentService) AddToCampaign(ctx context.Context, id, campaignID string, opts ...RequestOption) (*SegmentAddToCampaignResult, *Response, error) {
	body := struct {
		CampaignID string `json:"campaign_id"`
	}{CampaignID: campaignID}
	return send[SegmentAddToCampaignResult](ctx, s.client.post, "segments/"+url.PathEscape(id)+"/add-to-campaign", body, opts)
}
