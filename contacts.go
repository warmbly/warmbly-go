package warmbly

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// ContactService manages contacts (leads) in the current organization.
//
// Contacts are the people campaigns send to. They can be created in bulk,
// searched with structured filters, tagged, annotated with notes, and have an
// activity timeline.
type ContactService service

// Contact is a contact (lead) as returned by the API.
type Contact struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Company   string `json:"company,omitempty"`
	Phone     string `json:"phone,omitempty"`
	// CustomFields holds arbitrary organization-defined string fields.
	CustomFields map[string]string `json:"custom_fields,omitempty"`
	// Subscribed reports whether the contact is opted in to receive messages.
	Subscribed bool     `json:"subscribed"`
	Tags       []string `json:"tags,omitempty"`
	// VerificationStatus is the deliverability state of the email address: one
	// of "valid", "risky", "invalid" or "unknown".
	VerificationStatus string    `json:"verification_status,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// ContactInput is the payload for creating or upserting a single contact via
// [ContactService.Create]. Email is required.
type ContactInput struct {
	FirstName    string            `json:"first_name,omitempty"`
	LastName     string            `json:"last_name,omitempty"`
	Email        string            `json:"email"`
	Company      string            `json:"company,omitempty"`
	Phone        string            `json:"phone,omitempty"`
	CustomFields map[string]string `json:"custom_fields,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
}

// ContactCreateResult summarizes the outcome of a bulk create, reporting how
// many contacts were newly created, updated (matched an existing email) or
// skipped.
type ContactCreateResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

// ContactUpdateParams are the parameters for updating a single contact. Only
// the non-nil fields are sent; a nil field leaves the current value untouched.
type ContactUpdateParams struct {
	FirstName    *string            `json:"first_name,omitempty"`
	LastName     *string            `json:"last_name,omitempty"`
	Company      *string            `json:"company,omitempty"`
	Phone        *string            `json:"phone,omitempty"`
	Subscribed   *bool              `json:"subscribed,omitempty"`
	CustomFields *map[string]string `json:"custom_fields,omitempty"`
	Tags         *[]string          `json:"tags,omitempty"`
}

// ContactBulkUpdateParams are the parameters for updating many contacts at once
// via [ContactService.BulkUpdate]. IDs selects the contacts to mutate; the
// remaining non-nil fields describe the change to apply to all of them.
type ContactBulkUpdateParams struct {
	IDs        []string `json:"ids"`
	Subscribed *bool    `json:"subscribed,omitempty"`
	AddTags    []string `json:"add_tags,omitempty"`
	RemoveTags []string `json:"remove_tags,omitempty"`
}

// ContactSearchParams filters and paginates a contact search. Unlike other list
// endpoints, search is a POST whose filters travel in the request body, so the
// pagination controls are explicit JSON fields rather than an embedded
// [ListOptions].
type ContactSearchParams struct {
	// Limit is the maximum number of items per page (server-capped). Zero uses
	// the server default.
	Limit int `json:"limit,omitempty"`
	// Cursor is the opaque pagination token from a previous page's NextCursor.
	Cursor string `json:"cursor,omitempty"`
	// Query is a free-text query matched against name, email and company.
	Query string `json:"query,omitempty"`
	// CampaignID restricts results to contacts enrolled in a campaign.
	CampaignID string `json:"campaign_id,omitempty"`
	// Status restricts results to contacts in a given lifecycle status.
	Status string `json:"status,omitempty"`
	// Subscribed, when set, restricts to subscribed or unsubscribed contacts.
	Subscribed *bool `json:"subscribed,omitempty"`
	// Tags restricts results to contacts carrying all of the given tags.
	Tags []string `json:"tags,omitempty"`
}

// TimelineEvent is a single activity entry on a contact's timeline, such as a
// send, open, click or reply.
type TimelineEvent struct {
	Type        string         `json:"type"`
	Description string         `json:"description"`
	OccurredAt  time.Time      `json:"occurred_at"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// ContactNote is a free-text note attached to a contact.
type ContactNote struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	AuthorID  string    `json:"author_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Search returns a page of contacts matching params. Because the search
// endpoint is a POST whose filters are sent as a JSON body, the returned page
// is not wired for automatic paging: [Page.Next] reports [ErrNoMorePages].
// To page through results, set params.Cursor to the previous response's
// NextCursor and call Search again until NextCursor is empty.
func (s *ContactService) Search(ctx context.Context, params *ContactSearchParams) (*Page[Contact], error) {
	if params == nil {
		params = &ContactSearchParams{}
	}
	page := &Page[Contact]{}
	resp, err := s.client.post(ctx, "contacts/search", params, page)
	if err != nil {
		return nil, err
	}
	page.resp = resp
	return page, nil
}

// Create bulk-creates (and upserts on matching email) the given contacts,
// returning a summary of how many were created, updated or skipped.
func (s *ContactService) Create(ctx context.Context, contacts []ContactInput) (*ContactCreateResult, *Response, error) {
	body := struct {
		Contacts []ContactInput `json:"contacts"`
	}{Contacts: contacts}
	result := new(ContactCreateResult)
	resp, err := s.client.post(ctx, "contacts", body, result)
	if err != nil {
		return nil, resp, err
	}
	return result, resp, nil
}

// BulkUpdate applies a change to every contact named in params.IDs.
func (s *ContactService) BulkUpdate(ctx context.Context, params *ContactBulkUpdateParams) (*Response, error) {
	return s.client.patch(ctx, "contacts", params, nil)
}

// BulkDelete permanently deletes the given contacts by ID.
func (s *ContactService) BulkDelete(ctx context.Context, ids []string) (*Response, error) {
	body := struct {
		IDs []string `json:"ids"`
	}{IDs: ids}
	req, err := s.client.newRequest(ctx, http.MethodDelete, "contacts", body)
	if err != nil {
		return nil, err
	}
	return s.client.do(req, nil)
}

// Get retrieves a single contact by ID.
func (s *ContactService) Get(ctx context.Context, id string) (*Contact, *Response, error) {
	contact := new(Contact)
	resp, err := s.client.get(ctx, "contacts/"+url.PathEscape(id), contact)
	if err != nil {
		return nil, resp, err
	}
	return contact, resp, nil
}

// Update modifies a single contact and returns the updated representation.
func (s *ContactService) Update(ctx context.Context, id string, params *ContactUpdateParams) (*Contact, *Response, error) {
	contact := new(Contact)
	resp, err := s.client.patch(ctx, "contacts/"+url.PathEscape(id), params, contact)
	if err != nil {
		return nil, resp, err
	}
	return contact, resp, nil
}

// Delete permanently deletes a single contact by ID.
func (s *ContactService) Delete(ctx context.Context, id string) (*Response, error) {
	return s.client.delete(ctx, "contacts/"+url.PathEscape(id))
}

// Timeline returns the activity timeline for a contact, most recent first.
func (s *ContactService) Timeline(ctx context.Context, id string) ([]TimelineEvent, *Response, error) {
	var out struct {
		Data []TimelineEvent `json:"data"`
	}
	resp, err := s.client.get(ctx, "contacts/"+url.PathEscape(id)+"/timeline", &out)
	if err != nil {
		return nil, resp, err
	}
	return out.Data, resp, nil
}

// Notes returns the notes attached to a contact.
func (s *ContactService) Notes(ctx context.Context, id string) ([]ContactNote, *Response, error) {
	var out struct {
		Data []ContactNote `json:"data"`
	}
	resp, err := s.client.get(ctx, "contacts/"+url.PathEscape(id)+"/notes", &out)
	if err != nil {
		return nil, resp, err
	}
	return out.Data, resp, nil
}

// AddNote attaches a new note to a contact and returns the created note.
func (s *ContactService) AddNote(ctx context.Context, id, body string) (*ContactNote, *Response, error) {
	payload := struct {
		Body string `json:"body"`
	}{Body: body}
	note := new(ContactNote)
	resp, err := s.client.post(ctx, "contacts/"+url.PathEscape(id)+"/notes", payload, note)
	if err != nil {
		return nil, resp, err
	}
	return note, resp, nil
}
