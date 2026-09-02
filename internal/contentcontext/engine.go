// Package contentcontext contains the local, deterministic relevance boundary
// used by Timeline Related Context. It deliberately has no store, provider,
// browser, media, or mutation dependency: the store supplies bounded FTS
// candidates and this package decides which candidates are strong enough to
// show and how to explain the relationship publicly.
package contentcontext

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	MaxQueryTerms        = 12
	MaxFieldRunes        = 1600
	MaxTermRunes         = 48
	DefaultCandidatePool = 24
)

// Candidate is a local FTS candidate. BM25 is only a stable ordering hint; it
// is never sufficient to admit a weak lexical match.
type Candidate struct {
	Item       domain.MemoryItem
	BM25       float64
	Feedback   domain.ContentContextFeedbackVerdict
	FeedbackID string
}

// Query is the bounded feature set extracted from one Timeline item. Terms
// are used for candidate generation; anchors identify structured or carefully
// filtered distinctive terms that are allowed to admit a match; phrases are
// exact adjacent-token features containing one of those anchors.
type Query struct {
	Terms   []string
	Anchors []string
	Phrases []string
}

// Engine owns deterministic Content Context feature extraction, ranking, and
// admission. The zero value is ready to use.
type Engine struct {
	CandidatePool int
}

func NewEngine() Engine {
	return Engine{CandidatePool: DefaultCandidatePool}
}

// Extract builds a bounded, field-balanced query from the visible Timeline
// item. High-signal title/topic fields contribute exact phrase anchors.
func (e Engine) Extract(item domain.TimelineItem) Query {
	text := ""
	if item.Evidence != nil {
		text = item.Evidence.Text
	}
	fields := [][]string{
		tokenize(item.Item.WhatChanged),
		tokenize(strings.Join(append(append([]string{}, item.Assessment.TopicTags...), item.Assessment.TopicFacets...), " ")),
		tokenize(item.Item.WhyItMatters),
		tokenize(text),
	}
	terms := make([]string, 0, MaxQueryTerms)
	seen := make(map[string]bool, MaxQueryTerms)
	for round := 0; len(terms) < MaxQueryTerms; round++ {
		added := false
		for _, field := range fields {
			if round >= len(field) {
				continue
			}
			term := field[round]
			if seen[term] {
				continue
			}
			seen[term] = true
			terms = append(terms, term)
			added = true
			if len(terms) == MaxQueryTerms {
				break
			}
		}
		if !added {
			break
		}
	}

	// Structured topic tags/facets are the primary anchor source. They are
	// already an upstream topical classification, so a single genuinely
	// specific tag can admit a match. Title terms are only a fallback and go
	// through a stricter boilerplate/common-word filter; token length alone is
	// deliberately never treated as evidence of an entity.
	anchors := make([]string, 0, MaxQueryTerms)
	anchorSeen := make(map[string]bool, MaxQueryTerms)
	for _, field := range fields[1:2] {
		for _, term := range field {
			if !structuredAnchorTerm(term) || anchorSeen[term] {
				continue
			}
			anchorSeen[term] = true
			anchors = append(anchors, term)
		}
	}
	for _, term := range fields[0] {
		if !distinctiveTitleAnchorTerm(term) || anchorSeen[term] {
			continue
		}
		anchorSeen[term] = true
		anchors = append(anchors, term)
		if len(anchors) == MaxQueryTerms {
			break
		}
	}

	phraseFields := [][]string{fields[0], fields[1]}
	phrases := make([]string, 0, 6)
	phraseSeen := make(map[string]bool)
	for _, field := range phraseFields {
		for index := 0; index+1 < len(field); index++ {
			phrase := field[index] + " " + field[index+1]
			if phraseSeen[phrase] || !phraseContainsAnchor([]string{field[index], field[index+1]}, anchors) {
				continue
			}
			phraseSeen[phrase] = true
			phrases = append(phrases, phrase)
			if len(phrases) == 6 {
				break
			}
		}
		if len(phrases) == 6 {
			break
		}
	}
	return Query{Terms: terms, Anchors: anchors, Phrases: phrases}
}

// ExtractSearch builds a bounded relevance query from an explicit user-authored
// Library search. Unlike Timeline extraction, every non-generic search term may
// act as an anchor because the user deliberately supplied it. This keeps the
// read path provider-free while allowing short topic identities such as "AI".
func (e Engine) ExtractSearch(value string) Query {
	terms := uniqueTerms(tokenize(value))
	if len(terms) > MaxQueryTerms {
		terms = terms[:MaxQueryTerms]
	}
	anchors := make([]string, 0, len(terms))
	for _, term := range terms {
		if structuredAnchorTerm(term) {
			anchors = append(anchors, term)
		}
	}
	phrases := make([]string, 0, 6)
	for index := 0; index+1 < len(terms) && len(phrases) < 6; index++ {
		pair := []string{terms[index], terms[index+1]}
		if phraseContainsAnchor(pair, anchors) {
			phrases = append(phrases, strings.Join(pair, " "))
		}
	}
	return Query{Terms: terms, Anchors: anchors, Phrases: phrases}
}

// Match applies the precision-first admission policy and returns at most the
// caller's requested limit. Weak generic-token overlaps are intentionally
// omitted, so an empty result is a valid and useful outcome.
func (e Engine) Match(query Query, candidates []Candidate, limit int) []domain.ContentContextMatch {
	if limit < domain.ContentContextMinLimit || limit > domain.ContentContextMaxLimit {
		return []domain.ContentContextMatch{}
	}
	queryTerms := uniqueTerms(query.Terms)
	queryAnchors := uniqueTerms(query.Anchors)
	queryPhrases := uniqueTerms(query.Phrases)
	if len(queryTerms) == 0 || len(queryAnchors) == 0 {
		return []domain.ContentContextMatch{}
	}

	accepted := make([]rankedMatch, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Item.LifecycleState != "" && candidate.Item.LifecycleState != domain.MemoryStateActive {
			continue
		}
		if candidate.Feedback == domain.ContentContextFeedbackNotRelevant {
			continue
		}
		signals := scoreCandidate(queryTerms, queryAnchors, queryPhrases, candidate.Item)
		if !signals.admitted {
			continue
		}
		if candidate.Feedback == domain.ContentContextFeedbackRelevant {
			// Explicit pairwise feedback is stronger than lexical tie-breaking,
			// but it never admits a candidate that the current engine rejects.
			signals.strength += 1000
		}
		var feedback *domain.ContentContextFeedbackState
		if candidate.Feedback.ValidDecision() && candidate.FeedbackID != "" {
			feedback = &domain.ContentContextFeedbackState{ID: candidate.FeedbackID, Verdict: candidate.Feedback}
		}
		accepted = append(accepted, rankedMatch{
			match: domain.ContentContextMatch{
				Item:        candidate.Item,
				MatchReason: signals.reason,
				Feedback:    feedback,
			},
			strength: signals.strength,
			bm25:     candidate.BM25,
		})
	}
	sort.SliceStable(accepted, func(i, j int) bool {
		if accepted[i].strength != accepted[j].strength {
			return accepted[i].strength > accepted[j].strength
		}
		if accepted[i].bm25 != accepted[j].bm25 {
			return accepted[i].bm25 < accepted[j].bm25
		}
		left, right := accepted[i].match.Item, accepted[j].match.Item
		if left.UpdatedAt != right.UpdatedAt {
			return left.UpdatedAt > right.UpdatedAt
		}
		return left.ID > right.ID
	})
	if len(accepted) > limit {
		accepted = accepted[:limit]
	}
	result := make([]domain.ContentContextMatch, 0, len(accepted))
	for _, item := range accepted {
		result = append(result, item.match)
	}
	return result
}

// TopicIdentityMatches adds a precision gate for synthesized topic knowledge.
// A multi-token topic must match at least two identity tokens from its name or
// one alias. This keeps a broad parent such as "Codex" eligible while stopping
// the narrower "Codex Reset" from matching a Codex post that never discusses
// resets. Normal Memory matches keep their existing field-based policy.
func TopicIdentityMatches(query Query, name string, aliases []string) bool {
	return TopicIdentitySpecificity(query, name, aliases) > 0
}

// TopicIdentitySpecificity distinguishes an exact topic identity from a
// shared parent token. Complete identities outrank partial sibling matches.
func TopicIdentitySpecificity(query Query, name string, aliases []string) int {
	queryTerms := make(map[string]bool, len(query.Terms))
	for _, term := range uniqueTerms(query.Terms) {
		queryTerms[term] = true
	}
	identities := append([]string{name}, aliases...)
	best := 0
	for _, identity := range identities {
		tokens := uniqueTerms(tokenize(identity))
		meaningful := make([]string, 0, len(tokens))
		for _, token := range tokens {
			if !genericTerms[token] && !stopWords[token] {
				meaningful = append(meaningful, token)
			}
		}
		if len(meaningful) == 0 {
			continue
		}
		required := 1
		if len(meaningful) > 1 {
			required = 2
		}
		matched := 0
		for _, token := range meaningful {
			if queryTerms[token] {
				matched++
			}
		}
		if matched == len(meaningful) {
			score := 100 + matched
			if score > best {
				best = score
			}
		} else if matched >= required && matched > best {
			best = matched
		}
	}
	return best
}

type rankedMatch struct {
	match    domain.ContentContextMatch
	strength int
	bm25     float64
}

type candidateSignals struct {
	admitted bool
	strength int
	reason   string
}

func scoreCandidate(queryTerms, queryAnchors, queryPhrases []string, item domain.MemoryItem) candidateSignals {
	fields := memoryFields(item)
	fieldTerms := make(map[string]map[string]bool, len(fields))
	for _, field := range fields {
		fieldTerms[field.label] = termSet(field.value)
	}
	shared := make([]string, 0, len(queryTerms))
	sharedAnchors := make([]string, 0, len(queryAnchors))
	anchorSet := make(map[string]bool, len(queryAnchors))
	for _, anchor := range queryAnchors {
		anchorSet[anchor] = true
	}
	support := make(map[string]bool)
	for _, term := range queryTerms {
		if genericTerms[term] {
			continue
		}
		matched := false
		for _, field := range fields {
			if fieldTerms[field.label][term] {
				matched = true
				support[field.label] = true
			}
		}
		if matched {
			shared = append(shared, term)
			if anchorSet[term] {
				sharedAnchors = append(sharedAnchors, term)
			}
		}
	}
	sharedPhrases := make([]string, 0, len(queryPhrases))
	for _, phrase := range queryPhrases {
		phraseTokens := strings.Fields(phrase)
		if len(phraseTokens) < 2 || !phraseContainsAnchor(phraseTokens, queryAnchors) {
			continue
		}
		for _, field := range fields {
			if containsPhrase(field.value, phraseTokens) {
				sharedPhrases = append(sharedPhrases, phrase)
				support[field.label] = true
				break
			}
		}
	}
	// At least one actual anchor must overlap. Two arbitrary evidence words
	// (for example "every" and "provides") are never enough to admit a row.
	if len(sharedAnchors) == 0 && len(sharedPhrases) == 0 {
		return candidateSignals{}
	}

	labels := make([]string, 0, len(support))
	for _, field := range fields {
		if support[field.label] {
			labels = append(labels, field.label)
		}
	}
	strength := len(sharedAnchors)*100 + len(sharedPhrases)*80 + len(labels)*5
	var reason string
	if len(sharedPhrases) > 0 {
		reason = fmt.Sprintf("Shared phrase %q; supported by %s.", sharedPhrases[0], strings.Join(labels, ", "))
	} else {
		shown := sharedAnchors
		if len(shown) > 3 {
			shown = shown[:3]
		}
		reason = fmt.Sprintf("Shared topics: %s; supported by %s.", strings.Join(shown, ", "), strings.Join(labels, ", "))
	}
	return candidateSignals{admitted: true, strength: strength, reason: reason}
}

type memoryField struct {
	label string
	value string
}

func memoryFields(item domain.MemoryItem) []memoryField {
	fields := []memoryField{
		{label: "title", value: item.Title},
		{label: "summary", value: item.Summary},
		{label: "author", value: item.Author},
		{label: "tags", value: strings.Join(item.Tags, " ")},
		{label: "facets", value: strings.Join(item.Facets, " ")},
	}
	if item.FullContent != nil {
		fields = append(fields, memoryField{label: "retained text", value: *item.FullContent})
	}
	return fields
}

func tokenize(value string) []string {
	runes := []rune(strings.ToLower(strings.TrimSpace(value)))
	if len(runes) > MaxFieldRunes {
		runes = runes[:MaxFieldRunes]
	}
	result := make([]string, 0, 16)
	current := make([]rune, 0, 24)
	flush := func() {
		if len(current) > 0 && len(current) <= MaxTermRunes && !stopWords[string(current)] {
			result = append(result, string(current))
		}
		current = current[:0]
	}
	for _, char := range runes {
		if unicode.IsLetter(char) || unicode.IsNumber(char) {
			current = append(current, char)
			continue
		}
		flush()
	}
	flush()
	return result
}

func uniqueTerms(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func termSet(value string) map[string]bool {
	set := make(map[string]bool)
	for _, term := range tokenize(value) {
		set[term] = true
	}
	return set
}

func containsPhrase(value string, phrase []string) bool {
	tokens := tokenize(value)
	if len(phrase) == 0 || len(tokens) < len(phrase) {
		return false
	}
	for index := 0; index+len(phrase) <= len(tokens); index++ {
		matched := true
		for offset, token := range phrase {
			if tokens[index+offset] != token {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func structuredAnchorTerm(term string) bool {
	return term != "" && !stopWords[term] && !genericTerms[term]
}

func distinctiveTitleAnchorTerm(term string) bool {
	if !structuredAnchorTerm(term) || len([]rune(term)) < 4 {
		return false
	}
	return !boilerplateTitleTerms[term]
}

func phraseContainsAnchor(tokens, anchors []string) bool {
	anchorSet := make(map[string]bool, len(anchors))
	for _, anchor := range anchors {
		anchorSet[anchor] = true
	}
	for _, token := range tokens {
		if anchorSet[token] {
			return true
		}
	}
	return false
}

var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "in": true, "is": true,
	"it": true, "of": true, "on": true, "or": true, "that": true, "the": true,
	"their": true, "this": true, "to": true, "was": true, "with": true,
	"you": true, "your": true, "we": true, "they": true, "every": true,
	"provides": true, "published": true, "shared": true, "released": true,
}

// These terms are common enough in social posts to be weak evidence by
// themselves. A phrase containing one of them can still be admitted when the
// phrase also carries a strong topical anchor, such as "video stabilization".
var genericTerms = map[string]bool{
	"about": true, "all": true, "already": true, "camera": true, "change": true, "content": true, "context": true, "data": true,
	"detail": true, "event": true, "feature": true, "information": true, "issue": true,
	"latest": true, "local": true, "memory": true, "movement": true, "news": true,
	"people": true, "platform": true, "project": true, "research": true, "result": true,
	"system": true, "systems": true, "thing": true, "tool": true, "update": true,
	"using": true, "video": true, "way": true, "work": true,
	"open": true, "source": true, "own": true, "knows": true, "fix": true, "shaky": true,
	"foundation": true, "model": true, "models": true, "provides": true, "published": true,
	"shared": true, "released": true, "reported": true, "reports": true, "announced": true,
	"announces": true, "says": true, "said": true, "shows": true, "showed": true,
	"offers": true, "includes": true, "include": true, "uses": true,
	"used": true, "new": true, "general": true, "common": true, "every": true,
	"you": true, "your": true, "we": true, "they": true,
}

// These words can look topical in a title but are boilerplate rather than a
// distinctive entity/topic anchor. The list is intentionally separate from
// genericTerms because a term may remain useful for FTS candidate generation
// while still being disallowed as a title-derived admission anchor.
var boilerplateTitleTerms = map[string]bool{
	"across": true, "after": true, "before": true, "build": true, "built": true,
	"companies": true, "company": true, "create": true, "created": true,
	"develop": true, "developed": true, "developing": true, "guide": true,
	"helps": true, "improve": true, "improves": true, "inside": true,
	"know": true, "launch": true, "launched": true, "making": true,
	"offers": true, "published": true, "released": true, "reported": true,
	"reports": true, "shared": true, "shows": true, "using": true,
}
