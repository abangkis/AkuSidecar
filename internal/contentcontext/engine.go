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
	MaxIdentityPhrases   = 4
	MaxSubjectFeatures   = 8
	MaxFocusPhrases      = 12
)

// QueryMode separates the automatic Timeline relationship from an explicit
// user-authored Library search. The zero value is deliberately the stricter
// automatic policy so a hand-built Query cannot accidentally request broad
// admission.
type QueryMode uint8

const (
	QueryRelated QueryMode = iota
	QueryLibrarySearch
)

// Candidate is a local FTS candidate. BM25 is only a stable ordering hint; it
// is never sufficient to admit a weak lexical match.
type Candidate struct {
	Item       domain.MemoryItem
	BM25       float64
	Feedback   domain.ContentContextFeedbackVerdict
	FeedbackID string
}

// Query separates retrieval terms from the stricter automatic relationship
// features. Anchors and Phrases are reserved for explicit Library searches;
// automatic matching uses complete identities, corroborated subjects, and
// exact adjacent focus phrases.
type Query struct {
	Terms           []string
	Anchors         []string
	Phrases         []string
	Mode            QueryMode
	IdentityPhrases []string
	SubjectFeatures []string
	FocusPhrases    []string

	// excludedAuthors is used only by the strict automatic policy. Keeping it
	// private prevents callers from changing the admission boundary.
	excludedAuthors map[string]bool
}

// Engine owns deterministic Content Context feature extraction, ranking, and
// admission. The zero value is ready to use.
type Engine struct {
	CandidatePool int
}

func NewEngine() Engine {
	return Engine{CandidatePool: DefaultCandidatePool}
}

// Extract builds a bounded query from substantive Timeline text. Automatic
// matching uses corroborated subject features, exact adjacent focus phrases,
// and complete versioned identities; broad legacy anchors are search-only.
func (e Engine) Extract(item domain.TimelineItem) Query {
	text := ""
	authors := []string{item.Item.Author}
	if item.Evidence != nil {
		text = item.Evidence.Text
		if strings.TrimSpace(item.Evidence.Author) != "" {
			authors = []string{item.Evidence.Author}
		}
	}
	sourceFields := []string{item.Item.WhatChanged, item.Item.WhyItMatters, text}
	excludedAuthors := authorTokens(authors)
	identityPhrases, identityTokens := relatedIdentities(sourceFields, excludedAuthors)
	strictTerms := relatedTerms(sourceFields, excludedAuthors, identityTokens)
	tags := append(append([]string{}, item.Assessment.TopicTags...), item.Assessment.TopicFacets...)
	subjectFeatures := relatedSubjectFeatures(sourceFields, tags, excludedAuthors, identityTokens)
	focusPhrases := relatedFocusPhrases(sourceFields, excludedAuthors, identityTokens)
	if len(identityPhrases) > MaxIdentityPhrases {
		identityPhrases = identityPhrases[:MaxIdentityPhrases]
	}
	if len(strictTerms)+len(identityPhrases) > MaxQueryTerms {
		strictTerms = strictTerms[:MaxQueryTerms-len(identityPhrases)]
	}
	return Query{
		Terms: strictTerms, SubjectFeatures: subjectFeatures, FocusPhrases: focusPhrases,
		Mode: QueryRelated, IdentityPhrases: identityPhrases,
		excludedAuthors: excludedAuthors,
	}
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
	return Query{Terms: terms, Anchors: anchors, Phrases: phrases, Mode: QueryLibrarySearch}
}

// Match applies the precision-first admission policy and returns at most the
// caller's requested limit. Weak generic-token overlaps are intentionally
// omitted, so an empty result is a valid and useful outcome.
func (e Engine) Match(query Query, candidates []Candidate, limit int) []domain.ContentContextMatch {
	if limit < domain.ContentContextMinLimit || limit > domain.ContentContextMaxLimit {
		return []domain.ContentContextMatch{}
	}
	return e.match(query, candidates, limit)
}

// MatchAll ranks every admitted candidate for internal callers that need to
// apply a second precision gate before choosing their public result limit.
func (e Engine) MatchAll(query Query, candidates []Candidate) []domain.ContentContextMatch {
	return e.match(query, candidates, len(candidates))
}

func (e Engine) match(query Query, candidates []Candidate, limit int) []domain.ContentContextMatch {
	if limit == 0 {
		return []domain.ContentContextMatch{}
	}
	queryTerms := uniqueTerms(query.Terms)
	queryAnchors := uniqueTerms(query.Anchors)
	queryPhrases := uniqueTerms(query.Phrases)
	if query.Mode == QueryLibrarySearch && (len(queryTerms) == 0 || len(queryAnchors) == 0) {
		return []domain.ContentContextMatch{}
	}
	if query.Mode != QueryLibrarySearch && len(queryTerms) == 0 && len(query.IdentityPhrases) == 0 && len(query.SubjectFeatures) == 0 && len(query.FocusPhrases) == 0 {
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
		var signals candidateSignals
		if query.Mode == QueryLibrarySearch {
			signals = scoreCandidate(queryTerms, queryAnchors, queryPhrases, candidate.Item)
		} else {
			signals = scoreRelatedCandidate(query, candidate.Item)
		}
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
	if query.Mode != QueryLibrarySearch && len(query.IdentityPhrases) > 0 {
		identities := append([]string{name}, aliases...)
		queryIdentities := make(map[string]bool, len(query.IdentityPhrases))
		for _, identity := range query.IdentityPhrases {
			queryIdentities[canonicalIdentityPhrase(identity)] = true
		}
		for _, identity := range identities {
			for _, candidate := range identityPhrases(identity, nil) {
				if queryIdentities[candidate.phrase] {
					return 250 + len(strings.Fields(candidate.phrase))
				}
			}
		}
		// A topic name or alias that mentions a different version, or omits
		// the queried model identity, cannot qualify through a shared feature.
		return 0
	}
	if query.Mode != QueryLibrarySearch && (len(query.SubjectFeatures) > 0 || len(query.FocusPhrases) > 0) {
		identities := append([]string{name}, aliases...)
		best := 0
		for _, identity := range identities {
			canonical := canonicalIdentityPhrase(identity)
			if containsExactFeatureIdentity(query, canonical) && best < 280 {
				best = 280
			}
			candidatePhrases := relatedFocusPhrases([]string{identity}, nil, nil)
			if len(sharedPhrases(query.FocusPhrases, candidatePhrases)) > 0 {
				if len(query.SubjectFeatures) == 0 || containsAnySubject(identity, query.SubjectFeatures) {
					if best < 220 {
						best = 220
					}
				}
			}
			if containsAnySubject(identity, query.SubjectFeatures) && best < 110 {
				best = 110
			}
		}
		return best
	}
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

func containsExactFeatureIdentity(query Query, identity string) bool {
	if identity == "" {
		return false
	}
	for _, feature := range append(append([]string{}, query.SubjectFeatures...), query.FocusPhrases...) {
		if canonicalIdentityPhrase(feature) == identity {
			return true
		}
	}
	return false
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

type relatedToken struct {
	value       string
	original    string
	breakBefore bool
}

type modelIdentity struct {
	phrase    string
	family    string
	version   string
	qualifier string
}

func scoreRelatedCandidate(query Query, item domain.MemoryItem) candidateSignals {
	queryTerms := make(map[string]bool, len(query.Terms))
	for _, term := range uniqueTerms(query.Terms) {
		if relatedFeatureTerm(term) {
			queryTerms[term] = true
		}
	}
	if len(queryTerms) == 0 && len(query.IdentityPhrases) == 0 && len(query.SubjectFeatures) == 0 && len(query.FocusPhrases) == 0 {
		return candidateSignals{}
	}

	excluded := make(map[string]bool, len(query.excludedAuthors)+4)
	for token := range query.excludedAuthors {
		excluded[token] = true
	}
	for token := range authorTokens([]string{item.Author}) {
		excluded[token] = true
	}
	fields := []memoryField{{label: "title", value: item.Title}, {label: "summary", value: item.Summary}}
	if item.FullContent != nil {
		fields = append(fields, memoryField{label: "retained text", value: *item.FullContent})
	}
	text := make([]string, 0, len(fields))
	for _, field := range fields {
		text = append(text, field.value)
	}
	candidateIdentities, identityTokens := relatedIdentities(text, excluded)
	candidateTerms, support := relatedContentTerms(fields, excluded, identityTokens)
	candidateFocus := relatedFocusPhrases(text, excluded, identityTokens)
	candidateTags := append(append([]string{}, item.Tags...), item.Facets...)
	candidateSubjects := relatedSubjectFeatures(text, candidateTags, excluded, identityTokens)
	shared := make([]string, 0, len(queryTerms))
	for _, term := range uniqueTerms(query.Terms) {
		if queryTerms[term] && candidateTerms[term] {
			shared = append(shared, term)
		}
	}
	sharedFocus := sharedPhrases(query.FocusPhrases, candidateFocus)
	sharedSubjects := sharedPhrases(query.SubjectFeatures, append(candidateSubjects, relatedContentSubjects(query.SubjectFeatures, fields, excluded, identityTokens)...))

	if len(query.IdentityPhrases) > 0 {
		queryIdentitySet := make(map[string]bool, len(query.IdentityPhrases))
		for _, identity := range query.IdentityPhrases {
			queryIdentitySet[canonicalIdentityPhrase(identity)] = true
		}
		exactIdentity := ""
		for _, identity := range candidateIdentities {
			if queryIdentitySet[identity] {
				exactIdentity = identity
				break
			}
		}
		// Model identity is the subject. A family, number, or shared feature
		// cannot stand in for the complete version and qualifier.
		if exactIdentity == "" || (len(shared) == 0 && !hasDistinctPhrase(sharedFocus, strings.Fields(exactIdentity))) {
			return candidateSignals{}
		}
		feature := ""
		if len(shared) > 0 {
			feature = shared[0]
		} else if len(sharedFocus) > 0 {
			feature = sharedFocus[0]
		}
		return candidateSignals{
			admitted: true,
			strength: 300 + len(shared)*100 + len(sharedFocus)*80 + len(support)*5,
			reason:   fmt.Sprintf("Shared model identity %q and topic %q.", exactIdentity, feature),
		}
	}

	if len(query.SubjectFeatures) > 0 {
		matchedSubject := ""
		for _, subject := range uniqueTerms(query.SubjectFeatures) {
			if containsString(sharedSubjects, subject) {
				matchedSubject = subject
				break
			}
		}
		namedSubjectConflict := len(candidateSubjects) > 0 && len(sharedSubjects) == 0
		if matchedSubject != "" && (hasDistinctTerm(shared, strings.Fields(matchedSubject)) || hasDistinctPhrase(sharedFocus, strings.Fields(matchedSubject))) {
			reason := fmt.Sprintf("Shared subject %q and focused feature.", matchedSubject)
			if len(sharedFocus) > 0 {
				bestPhrase := sharedFocus[0]
				for _, phrase := range sharedFocus {
					if containsString(strings.Fields(phrase), matchedSubject) {
						bestPhrase = phrase
						break
					}
				}
				reason = fmt.Sprintf("Shared focused phrase %q with subject %q.", bestPhrase, matchedSubject)
			}
			return candidateSignals{
				admitted: true,
				strength: 250 + len(shared)*100 + len(sharedFocus)*80 + len(support)*5,
				reason:   reason,
			}
		}
		if namedSubjectConflict || !hasDistinctPhrase(sharedFocus, nil) {
			return candidateSignals{}
		}
	} else if len(sharedFocus) == 0 {
		return candidateSignals{}
	}
	if len(sharedFocus) > 0 {
		return candidateSignals{
			admitted: true,
			strength: len(sharedFocus)*120 + len(shared)*50 + len(support)*5,
			reason:   fmt.Sprintf("Shared focused phrase %q.", sharedFocus[0]),
		}
	}
	return candidateSignals{}
}

func relatedTerms(fields []string, excluded map[string]bool, identityTokens map[string]bool) []string {
	fieldTerms := make([][]string, 0, len(fields))
	for _, field := range fields {
		terms := make([]string, 0, 12)
		for _, token := range relatedTokens(field) {
			if excluded[token.value] || identityTokens[token.value] || !relatedFeatureTerm(token.value) {
				continue
			}
			terms = append(terms, token.value)
		}
		fieldTerms = append(fieldTerms, terms)
	}
	terms := make([]string, 0, MaxQueryTerms)
	seen := make(map[string]bool, MaxQueryTerms)
	for round := 0; len(terms) < MaxQueryTerms; round++ {
		added := false
		for _, field := range fieldTerms {
			if round >= len(field) || seen[field[round]] {
				continue
			}
			seen[field[round]] = true
			terms = append(terms, field[round])
			added = true
			if len(terms) == MaxQueryTerms {
				break
			}
		}
		if !added {
			break
		}
	}
	return terms
}

func relatedSubjectFeatures(fields, tags []string, excluded, identityTokens map[string]bool) []string {
	result := make([]string, 0, MaxSubjectFeatures)
	seen := make(map[string]bool, MaxSubjectFeatures)
	add := func(value string) {
		value = canonicalIdentityPhrase(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		result = append(result, value)
	}
	// Mixed-case names such as GoPro and OpenUI provide a subject signal when
	// they occur in substantive text. Ordinary initial capitalization does not.
	for _, field := range fields {
		for _, token := range relatedTokens(field) {
			if excluded[token.value] || identityTokens[token.value] || !relatedFeatureTerm(token.value) || !hasInternalUppercase(token.original) {
				continue
			}
			add(token.value)
			if len(result) == MaxSubjectFeatures {
				return result
			}
		}
	}
	// Upstream tags may identify a subject only when the full tag occurs
	// contiguously in substantive text. Multiword tags remain intact; matching
	// one fragment of a product name is not enough.
	for _, tag := range tags {
		tokens := relatedTokens(tag)
		if !validSubjectTag(tokens, excluded, identityTokens) || !containsNamedFieldSequence(fields, tokens, excluded, identityTokens) {
			continue
		}
		parts := make([]string, 0, len(tokens))
		for _, token := range tokens {
			parts = append(parts, token.value)
		}
		add(strings.Join(parts, " "))
		if len(result) == MaxSubjectFeatures {
			break
		}
	}
	return result
}

func relatedFocusPhrases(fields []string, excluded, identityTokens map[string]bool) []string {
	result := make([]string, 0, MaxFocusPhrases)
	seen := make(map[string]bool, MaxFocusPhrases)
	for _, field := range fields {
		tokens := relatedTokens(field)
		for index := 0; index+1 < len(tokens); index++ {
			left, right := tokens[index], tokens[index+1]
			if right.breakBefore || excluded[left.value] || excluded[right.value] || identityTokens[left.value] || identityTokens[right.value] || !focusedPair(left.value, right.value) {
				continue
			}
			phrase := left.value + " " + right.value
			if seen[phrase] {
				continue
			}
			seen[phrase] = true
			result = append(result, phrase)
			if len(result) == MaxFocusPhrases {
				return result
			}
		}
	}
	return result
}

func focusedPair(left, right string) bool {
	if stopWords[left] || stopWords[right] || relatedWeakTerms[left] || relatedWeakTerms[right] {
		return false
	}
	leftSpecific, rightSpecific := relatedFeatureTerm(left), relatedFeatureTerm(right)
	if leftSpecific && rightSpecific {
		return focusedTechnicalTerms[left] || focusedTechnicalTerms[right]
	}
	return focusedTechnicalTerms[left] && focusPhraseCompanions[right] || focusedTechnicalTerms[right] && focusPhraseCompanions[left]
}

func validSubjectTag(tokens []relatedToken, excluded, identityTokens map[string]bool) bool {
	if len(tokens) == 0 || len(tokens) > 4 {
		return false
	}
	if len(tokens) == 1 {
		token := tokens[0].value
		return !excluded[token] && !identityTokens[token] && !weakSubjectTagTerms[token] && relatedFeatureTerm(token)
	}
	specific := false
	for _, token := range tokens {
		if token.value == "" || excluded[token.value] || identityTokens[token.value] || stopWords[token.value] || relatedWeakTerms[token.value] || weakSubjectTagTerms[token.value] {
			return false
		}
		if relatedFeatureTerm(token.value) {
			specific = true
		} else if !focusPhraseCompanions[token.value] {
			return false
		}
	}
	return specific
}

func containsNamedFieldSequence(fields []string, wanted []relatedToken, excluded, identityTokens map[string]bool) bool {
	if len(wanted) == 0 {
		return false
	}
	for _, field := range fields {
		tokens := relatedTokens(field)
		for index := 0; index+len(wanted) <= len(tokens); index++ {
			matched, named := true, false
			for offset, expected := range wanted {
				actual := tokens[index+offset]
				if actual.value != expected.value || excluded[actual.value] || identityTokens[actual.value] || (offset > 0 && actual.breakBefore) {
					matched = false
					break
				}
				named = named || hasUppercase(actual.original)
			}
			if matched && named {
				return true
			}
		}
	}
	return false
}

func containsFieldToken(fields []string, target string, excluded, identityTokens map[string]bool) bool {
	for _, field := range fields {
		for _, token := range relatedTokens(field) {
			if token.value == target && !excluded[token.value] && !identityTokens[token.value] {
				return true
			}
		}
	}
	return false
}

func relatedContentSubjects(subjects []string, fields []memoryField, excluded, identityTokens map[string]bool) []string {
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		values = append(values, field.value)
	}
	result := make([]string, 0, len(subjects))
	for _, subject := range uniqueTerms(subjects) {
		tokens := relatedTokens(subject)
		if validSubjectTag(tokens, excluded, identityTokens) && containsNamedFieldSequence(values, tokens, excluded, identityTokens) {
			result = append(result, subject)
		}
	}
	return result
}

func sharedPhrases(left, right []string) []string {
	rightSet := make(map[string]bool, len(right))
	for _, phrase := range right {
		rightSet[canonicalIdentityPhrase(phrase)] = true
	}
	shared := make([]string, 0, len(left))
	for _, phrase := range uniqueTerms(left) {
		phrase = canonicalIdentityPhrase(phrase)
		if phrase != "" && rightSet[phrase] {
			shared = append(shared, phrase)
		}
	}
	return shared
}

func hasDistinctTerm(values, excluded []string) bool {
	excludedSet := make(map[string]bool, len(excluded))
	for _, value := range excluded {
		excludedSet[value] = true
	}
	for _, value := range values {
		if !excludedSet[value] {
			return true
		}
	}
	return false
}

func hasDistinctPhrase(phrases, excluded []string) bool {
	for _, phrase := range phrases {
		excludedSet := make(map[string]bool, len(excluded))
		for _, value := range excluded {
			excludedSet[value] = true
		}
		for _, value := range strings.Fields(phrase) {
			if !excludedSet[value] && relatedFeatureTerm(value) {
				return true
			}
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func hasInternalUppercase(value string) bool {
	runes := []rune(value)
	if len(runes) < 2 || !unicode.IsUpper(runes[0]) {
		return false
	}
	hasLower := false
	hasLaterUpper := false
	for _, char := range runes[1:] {
		hasLower = hasLower || unicode.IsLower(char)
		hasLaterUpper = hasLaterUpper || unicode.IsUpper(char)
	}
	return hasLower && hasLaterUpper
}

func relatedContentTerms(fields []memoryField, excluded map[string]bool, identityTokens map[string]bool) (map[string]bool, map[string]bool) {
	terms := make(map[string]bool)
	support := make(map[string]bool)
	for _, field := range fields {
		for _, token := range relatedTokens(field.value) {
			if excluded[token.value] || identityTokens[token.value] || !relatedFeatureTerm(token.value) {
				continue
			}
			terms[token.value] = true
			support[field.label] = true
		}
	}
	return terms, support
}

func relatedIdentities(fields []string, excluded map[string]bool) ([]string, map[string]bool) {
	identities := make([]modelIdentity, 0, 4)
	seen := make(map[string]bool)
	identityTokens := make(map[string]bool)
	for _, field := range fields {
		tokens := relatedTokens(field)
		for _, identity := range identityPhrasesFromTokens(tokens, excluded) {
			if seen[identity.phrase] {
				continue
			}
			seen[identity.phrase] = true
			identities = append(identities, identity)
			for _, token := range strings.Fields(identity.phrase) {
				identityTokens[token] = true
			}
		}
	}
	phrases := make([]string, 0, len(identities))
	for _, identity := range identities {
		phrases = append(phrases, identity.phrase)
	}
	return phrases, identityTokens
}

func identityPhrases(value string, excluded map[string]bool) []modelIdentity {
	return identityPhrasesFromTokens(relatedTokens(value), excluded)
}

func containsAnySubject(value string, subjects []string) bool {
	for _, subject := range subjects {
		tokens := relatedTokens(subject)
		if len(tokens) == 0 {
			continue
		}
		if len(tokens) == 1 {
			if containsFieldToken([]string{value}, tokens[0].value, nil, nil) {
				return true
			}
			continue
		}
		if containsAdjacentSequence(value, tokens) {
			return true
		}
	}
	return false
}

func containsAdjacentSequence(value string, wanted []relatedToken) bool {
	if len(wanted) == 0 {
		return false
	}
	tokens := relatedTokens(value)
	for index := 0; index+len(wanted) <= len(tokens); index++ {
		matched := true
		for offset, expected := range wanted {
			actual := tokens[index+offset]
			if actual.value != expected.value || (offset > 0 && actual.breakBefore) {
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

func identityPhrasesFromTokens(tokens []relatedToken, excluded map[string]bool) []modelIdentity {
	result := make([]modelIdentity, 0, 4)
	seen := make(map[string]bool)
	for index, token := range tokens {
		if !isNumericToken(token.value) || index == 0 || excluded[token.value] || excluded[tokens[index-1].value] || !modelFamilyToken(tokens[index-1].value) {
			continue
		}
		familyIndex := index - 1
		versionEnd := index + 1
		versionParts := []string{token.value}
		for versionEnd < len(tokens) && len(versionParts) < 3 && !excluded[tokens[versionEnd].value] && isNumericToken(tokens[versionEnd].value) {
			versionParts = append(versionParts, tokens[versionEnd].value)
			versionEnd++
		}
		qualifier := ""
		if versionEnd < len(tokens) && !excluded[tokens[versionEnd].value] && isModelQualifier(tokens[versionEnd]) {
			qualifier = tokens[versionEnd].value
			versionEnd++
		}

		starts := []int{familyIndex}
		for start := familyIndex - 1; start >= 0 && familyIndex-start < 4; start-- {
			if excluded[tokens[start].value] || !hasUppercase(tokens[start].original) || !modelFamilyToken(tokens[start].value) {
				break
			}
			starts = append(starts, start)
		}
		for _, start := range starts {
			containsExcluded := false
			for cursor := start; cursor < versionEnd; cursor++ {
				if excluded[tokens[cursor].value] {
					containsExcluded = true
					break
				}
			}
			if containsExcluded {
				continue
			}
			parts := make([]string, 0, versionEnd-start+1)
			for cursor := start; cursor < versionEnd; cursor++ {
				parts = append(parts, tokens[cursor].value)
			}
			phrase := strings.Join(parts, " ")
			if seen[phrase] {
				continue
			}
			seen[phrase] = true
			result = append(result, modelIdentity{
				phrase: phrase, family: tokens[familyIndex].value,
				version: strings.Join(versionParts, " "), qualifier: qualifier,
			})
		}
	}
	return result
}

func relatedTokens(value string) []relatedToken {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > MaxFieldRunes {
		runes = runes[:MaxFieldRunes]
	}
	result := make([]relatedToken, 0, 24)
	current := make([]rune, 0, 16)
	currentNumeric := false
	currentBreakBefore := false
	breakPending := false
	flush := func() {
		if len(current) == 0 || len(current) > MaxTermRunes {
			current = current[:0]
			return
		}
		original := string(current)
		result = append(result, relatedToken{value: strings.ToLower(original), original: original, breakBefore: currentBreakBefore})
		current = current[:0]
	}
	for _, char := range runes {
		if !unicode.IsLetter(char) && !unicode.IsNumber(char) {
			flush()
			if char == '.' || char == '!' || char == '?' || char == '\n' || char == '\r' || char == ';' || char == ':' {
				breakPending = true
			}
			continue
		}
		isNumeric := unicode.IsNumber(char)
		if len(current) > 0 && isNumeric != currentNumeric {
			flush()
		}
		if len(current) == 0 {
			currentNumeric = isNumeric
			currentBreakBefore = breakPending
			breakPending = false
		}
		current = append(current, char)
	}
	flush()
	return result
}

func authorTokens(authors []string) map[string]bool {
	result := make(map[string]bool)
	for _, author := range authors {
		for _, token := range relatedTokens(author) {
			if token.value != "" {
				result[token.value] = true
			}
		}
	}
	return result
}

func canonicalIdentityPhrase(value string) string {
	parts := make([]string, 0, 8)
	for _, token := range relatedTokens(value) {
		parts = append(parts, token.value)
	}
	return strings.Join(parts, " ")
}

func relatedFeatureTerm(term string) bool {
	if term == "" || len([]rune(term)) < 3 || isNumericToken(term) || stopWords[term] || genericTerms[term] || boilerplateTitleTerms[term] || relatedWeakTerms[term] {
		return false
	}
	for _, char := range term {
		if unicode.IsNumber(char) {
			return false
		}
	}
	return true
}

func modelFamilyToken(value string) bool {
	return relatedFeatureTerm(value) && !modelContextWords[value]
}

func isModelQualifier(token relatedToken) bool {
	return modelQualifiers[token.value]
}

func hasUppercase(value string) bool {
	for _, char := range value {
		if unicode.IsUpper(char) {
			return true
		}
	}
	return false
}

func isNumericToken(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if !unicode.IsNumber(char) {
			return false
		}
	}
	return true
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

// These words describe broad categories, attribution, or ordinary prose. A
// single overlap on one of them cannot establish an automatic relationship,
// even when it appears in a summary or a structured tag.
var relatedWeakTerms = map[string]bool{
	"ai": true, "anthropic": true, "artificial": true, "author": true,
	"company": true, "companies": true, "described": true, "discussion": true,
	"explained": true, "explains": true, "outlined": true, "outline": true,
	"google": true, "microsoft": true, "meta": true, "nvidia": true,
	"openai": true, "quota": true, "quotas": true, "team": true,
	"teams": true, "product": true, "products": true,
	"usage": true, "user": true, "users": true, "intelligence": true,
	"overview": true, "topic": true, "topics": true, "written": true,
	"announce": true, "announces": true, "announced": true, "announcing": true,
	"announcement": true, "announcements": true, "launch": true, "launches": true,
	"launched": true, "launching": true, "release": true, "releases": true,
	"released": true, "releasing": true, "introduce": true, "introduces": true,
	"introducing": true, "introduced": true, "unveil": true, "unveils": true,
	"unveiled": true, "unveiling": true, "reveal": true, "reveals": true,
	"revealed": true, "revealing": true, "publish": true, "publishes": true,
	"published": true, "publishing": true, "share": true, "shares": true,
	"shared": true, "sharing": true, "report": true, "reports": true,
	"reported": true, "reporting": true, "state": true, "states": true,
	"stated": true, "stating": true, "say": true, "says": true,
	"said": true, "saying": true,
}

var modelContextWords = map[string]bool{
	"build": true, "chapter": true, "day": true, "episode": true,
	"model": true, "models": true, "month": true, "number": true,
	"part": true, "phase": true, "release": true, "released": true,
	"series": true, "stage": true, "update": true, "updated": true,
	"version": true, "volume": true, "week": true, "year": true,
}

// Version qualifiers extend the atomic identity. The list covers common
// release and model variants without consuming an adjacent feature word such
// as "coding" in "GPT-6.1 Coding limits".
var modelQualifiers = map[string]bool{
	"alpha": true, "base": true, "beta": true, "chat": true, "code": true,
	"experimental": true, "exp": true, "fast": true, "flash": true,
	"high": true, "instruct": true, "instruction": true, "instant": true,
	"large": true, "lite": true, "low": true, "max": true, "mini": true,
	"omni": true, "plus": true, "preview": true, "pro": true,
	"reasoning": true, "small": true, "thinking": true, "turbo": true,
	"ultra": true, "vision": true,
}

// Phrase companions may be broad alone but help identify a focused topic when
// they occur directly beside a specific term, as in "video stabilization".
var focusPhraseCompanions = map[string]bool{
	"camera": true, "data": true, "image": true, "images": true,
	"language": true, "metadata": true, "model": true, "models": true,
	"network": true, "networks": true, "system": true, "systems": true,
	"ui": true, "video": true,
}

// Only these focused technical terms may anchor a free-standing adjacent
// phrase. Product-specific features can still match through a corroborated
// subject, but ordinary prose pairs such as "coding performance" cannot
// establish an automatic relationship by themselves.
var focusedTechnicalTerms = map[string]bool{
	"calibration": true, "embedding": true, "embeddings": true,
	"generative": true, "gyroscope": true, "matrix": true, "motion": true,
	"quantum": true, "stabilization": true, "stabilisation": true,
	"vector": true,
}

// These common feature labels can corroborate a product relationship, but
// they cannot become the subject itself through an upstream tag.
var weakSubjectTagTerms = map[string]bool{
	"coding": true, "performance": true,
}
