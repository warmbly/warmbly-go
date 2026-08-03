package warmbly

import (
	"context"
	"net/url"
	"time"
)

// GenerationService writes and rewrites campaign copy with AI, grounded in the
// workspace voice profile and any enabled [AISkill] playbooks.
//
// Every call here spends AI credits and returns the remaining balance, so a
// caller can surface cost without a second request. None of them send anything.
type GenerationService service

// Generation is the result of an AI writing call.
type Generation struct {
	Text string `json:"text"`
	// CreditsCharged is what this call cost, including any usage overage;
	// CreditsRemaining is the workspace balance afterwards.
	CreditsCharged   int    `json:"credits_charged"`
	CreditsRemaining int    `json:"credits_remaining"`
	TokensUsed       int    `json:"tokens_used"`
	Model            string `json:"model"`
}

// WriteParams asks for a fresh piece of copy.
type WriteParams struct {
	Prompt string `json:"prompt"`
	// Tone steers the register, for example "direct" or "friendly".
	Tone string `json:"tone,omitempty"`
}

// EditParams rewrites an existing passage.
type EditParams struct {
	// Text is the passage to rewrite.
	Text string `json:"text"`
	// Instruction is what to change about it.
	Instruction string `json:"instruction"`
	// Context is the surrounding draft, which is fenced so quoted inbound mail
	// inside it cannot steer the rewrite.
	Context string `json:"context,omitempty"`
	Tone    string `json:"tone,omitempty"`
}

// AI variable resolution modes for [AIVariableParams.Mode].
const (
	// AIVariableInstant resolves from what is already known about the contact.
	AIVariableInstant = "instant"
	// AIVariableResearch researches the contact first, and costs more.
	AIVariableResearch = "research"
)

// AIVariableParams previews a per-recipient AI variable block against one
// contact, so the editor can show what a merge tag will actually produce.
type AIVariableParams struct {
	// Mode is [AIVariableInstant] or [AIVariableResearch].
	Mode   string `json:"mode,omitempty"`
	Prompt string `json:"prompt"`
	Tone   string `json:"tone,omitempty"`
	// WebSearch lets a research-mode preview search the web.
	WebSearch bool `json:"web_search,omitempty"`
	// ContactID is the contact to render against.
	ContactID string `json:"contact_id,omitempty"`
	// ContextBefore and ContextAfter are the surrounding copy, each clamped
	// server-side so a caller cannot inflate the prompt.
	ContextBefore string `json:"context_before,omitempty"`
	ContextAfter  string `json:"context_after,omitempty"`
}

// Write generates a new piece of copy.
func (s *GenerationService) Write(ctx context.Context, params *WriteParams, opts ...RequestOption) (*Generation, *Response, error) {
	return send[Generation](ctx, s.client, s.client.post, "generation/write", params, opts)
}

// Edit rewrites a passage according to an instruction.
func (s *GenerationService) Edit(ctx context.Context, params *EditParams, opts ...RequestOption) (*Generation, *Response, error) {
	return send[Generation](ctx, s.client, s.client.post, "generation/edit", params, opts)
}

// AIVariable previews an AI variable block against a single contact. A prompt
// that renders empty against the contact costs nothing.
func (s *GenerationService) AIVariable(ctx context.Context, params *AIVariableParams, opts ...RequestOption) (*Generation, *Response, error) {
	return send[Generation](ctx, s.client, s.client.post, "generation/ai-variable", params, opts)
}

// SkillService manages AI skills: workspace playbooks folded into every AI
// writing surface, on top of the voice profile on [Organization].
//
// An enabled skill applies everywhere the assistant writes, so keep them short
// and behavioral — "never open with 'I hope this finds you well'" — rather
// than piling in per-campaign detail.
type SkillService service

// AISkill is one workspace playbook.
type AISkill struct {
	ID          string `json:"id"`
	OrgID       string `json:"org_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Content is the instruction text handed to the model.
	Content string `json:"content"`
	// Enabled reports whether the skill is currently applied.
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SkillCreateParams creates a skill. Name is required.
type SkillCreateParams struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content,omitempty"`
	// Enabled defaults to true when nil.
	Enabled *bool `json:"enabled,omitempty"`
}

// SkillUpdateParams updates a skill. Nil fields are left unchanged.
type SkillUpdateParams struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Content     *string `json:"content,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
}

// List returns the workspace's AI skills.
func (s *SkillService) List(ctx context.Context, opts ...RequestOption) ([]AISkill, *Response, error) {
	return fetchData[AISkill](ctx, s.client, "ai/skills", opts)
}

// Create adds an AI skill.
func (s *SkillService) Create(ctx context.Context, params *SkillCreateParams, opts ...RequestOption) (*AISkill, *Response, error) {
	return send[AISkill](ctx, s.client, s.client.post, "ai/skills", params, opts)
}

// Update modifies an AI skill.
func (s *SkillService) Update(ctx context.Context, id string, params *SkillUpdateParams, opts ...RequestOption) (*AISkill, *Response, error) {
	return send[AISkill](ctx, s.client, s.client.patch, "ai/skills/"+url.PathEscape(id), params, opts)
}

// Delete removes an AI skill.
func (s *SkillService) Delete(ctx context.Context, id string, opts ...RequestOption) (*Response, error) {
	return s.client.delete(ctx, "ai/skills/"+url.PathEscape(id), opts...)
}
