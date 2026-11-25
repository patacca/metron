package core

import (
	"fmt"
	"slices"
)

// Ingest validates a role report and returns the events that accepting it
// yields, in the order of docs/architecture.md, section 7. digest is the
// digest of the report's canonical JSON form and rawDigest that of the bytes
// as submitted; both are "sha256:<hex>".
//
// The report has passed structural validation. Ingest applies the semantic
// layer, then the transition layer, and refuses with a *Rejection of the
// first layer that finds problems, listing all of them.
//
// When the report's canonical digest equals the one already accepted for its
// packet, Ingest returns no drafts, duplicate true and no error: the report
// was already accepted and there is nothing to do.
func Ingest(s State, f Facts, r Report, digest, rawDigest string) (drafts []Draft, duplicate bool, err error) {
	if !validDigest(digest) || !validDigest(rawDigest) {
		return nil, false, fmt.Errorf("report digests %q and %q must be sha256 digests", digest, rawDigest)
	}
	if p := ledgerProblem(s, f); p != "" {
		return nil, false, refuse(LayerTransition, "%s", p)
	}
	i := s.packetIndex(r.Packet)
	if i < 0 {
		var ps problems
		ps.add("/packet", "packet %q was never issued: answer the packet you were given", r.Packet)
		return nil, false, ps.err(LayerTransition)
	}
	pk := s.packets[i]
	// Identifiers repeat across ledgers; the recorded digest does not. This
	// comes first so that nothing, not even the duplicate shortcut, accepts a
	// report written for another packet.
	if r.PacketDigest != pk.Digest {
		var ps problems
		if r.PacketDigest == "" {
			ps.add("/packet_digest", "the report names no packet digest: copy packet_digest from packet %s and submit it again", pk.Packet)
		} else {
			ps.add("/packet_digest", "the report was written for a different packet %s (digest %s, but this ledger issued %s): it comes from another ledger, another repository or an earlier issue of the same identifier; get the current packet again and answer it",
				pk.Packet, r.PacketDigest, pk.Digest)
		}
		return nil, false, ps.err(LayerTransition)
	}
	if pk.Report != nil && pk.Report.Digest == digest {
		return nil, true, nil
	}
	if err := ready(s, f); err != nil {
		return nil, false, err
	}
	if err := checkCandidate(f); err != nil {
		return nil, false, err
	}
	in := &ingestion{s: s, f: f, c: *f.Contract, r: r, pk: pk}
	in.th = in.c.Thresholds()
	in.envelope()
	if r.Role == pk.Role {
		in.body()
	}
	if err := in.sem.err(LayerSemantic); err != nil {
		return nil, false, err
	}
	in.transition()
	if err := in.tr.err(LayerTransition); err != nil {
		return nil, false, err
	}
	return in.events(digest, rawDigest), false, nil
}

// ingestion holds one report under validation.
type ingestion struct {
	s       State
	f       Facts
	c       Contract
	th      Convergence
	r       Report
	pk      PacketRecord
	sem, tr problems

	// The drafts under construction, the last finding number assigned, and
	// the escalations to raise once every other event is in.
	drafts      []Draft
	nextFinding int
	raised      []EscalationRaised

	// Exactly one body is set once the body has decoded.
	reviewer    *ReviewerBody
	implementer *ImplementerBody
	verifier    *VerifierBody
	auditor     *AuditorBody
}

func (in *ingestion) envelope() {
	if in.r.Schema != SchemaReport {
		in.sem.add("/schema", "schema is %q: it must be %q", in.r.Schema, SchemaReport)
	}
	if !in.r.Role.Valid() {
		in.sem.add("/role", "%q is not a role", in.r.Role)
	}
	if blank(in.r.Author.ID) {
		in.sem.add("/author/id", "the author id is blank: declare a stable identity")
	}
	if in.r.Author.Kind != AuthorAgent && in.r.Author.Kind != AuthorHuman {
		in.sem.add("/author/kind", "%q is not %q or %q", in.r.Author.Kind, AuthorAgent, AuthorHuman)
	}
}

func (in *ingestion) body() {
	var err error
	switch in.pk.Role {
	case RoleReviewer:
		var b ReviewerBody
		if b, err = payload[ReviewerBody](in.r.Body); err == nil {
			in.reviewer = &b
			in.checkReviewer(b)
		}
	case RoleImplementer:
		var b ImplementerBody
		if b, err = payload[ImplementerBody](in.r.Body); err == nil {
			in.implementer = &b
			in.checkImplementer(b)
		}
	case RoleVerifier:
		var b VerifierBody
		if b, err = payload[VerifierBody](in.r.Body); err == nil {
			in.verifier = &b
			in.checkVerifier(b)
		}
	case RoleAuditor:
		var b AuditorBody
		if b, err = payload[AuditorBody](in.r.Body); err == nil {
			in.auditor = &b
			in.checkAuditor(b)
		}
	default:
		err = fmt.Errorf("packet %s has no report body for role %q", in.pk.Packet, in.pk.Role)
	}
	if err != nil {
		in.sem.add("/body", "the body is not a valid %s body (%v): write it as the report schema describes", in.pk.Role, err)
	}
}

func (in *ingestion) exists(id string) bool { return in.s.findingIndex(id) >= 0 }

// find returns a finding the semantic layer has established exists. An
// unknown identifier yields a zero Finding, whose empty status no transition
// accepts.
func (in *ingestion) find(id string) Finding {
	if i := in.s.findingIndex(id); i >= 0 {
		return in.s.findings[i]
	}
	return Finding{ID: id}
}

// citesContract reports whether a finding cites an applicable requirement or
// an unacceptable risk of the contract.
func (in *ingestion) citesContract(fi FindingInput) bool {
	if _, ok := in.applicable(fi.Requirement); ok && fi.Requirement != "" {
		return true
	}
	return fi.Risk != "" && slices.ContainsFunc(in.c.Risks.Unacceptable, func(r Risk) bool { return r.ID == fi.Risk })
}

func (in *ingestion) applicable(id string) (Requirement, bool) {
	for _, r := range in.c.Applicable() {
		if r.ID == id {
			return r, true
		}
	}
	return Requirement{}, false
}

// conflicts lists, for each role, the roles the same author may never have
// held in the ledger.
var conflicts = map[Role][]Role{
	RoleImplementer: {RoleReviewer, RoleVerifier, RoleAuditor},
	RoleReviewer:    {RoleImplementer, RoleAuditor},
	RoleVerifier:    {RoleImplementer, RoleAuditor},
	RoleAuditor:     {RoleImplementer, RoleReviewer, RoleVerifier},
}

func (in *ingestion) transition() {
	pk := in.pk
	if in.r.Role != pk.Role {
		in.tr.add("/role", "packet %s was issued to the %s, not the %s: answer it with a %s report", pk.Packet, pk.Role, in.r.Role, pk.Role)
	}
	switch {
	case pk.Report != nil:
		in.tr.add("/packet", "packet %s was already answered by a different report (%s): request the next packet", pk.Packet, pk.Report.Digest)
	case !pk.Open:
		in.tr.add("/packet", "packet %s was voided (%s): request a new packet", pk.Packet, pk.VoidReason)
	case in.f.PacketProblem != "":
		in.tr.add("/packet", "the stored file of packet %s is unusable (%s): run metron packet again for a new packet and answer that one", pk.Packet, packetInvalid(in.s, in.f, pk))
	default:
		if why := packetInvalid(in.s, in.f, pk); why != "" {
			in.tr.add("/packet", "packet %s is no longer valid (%s): request a new packet", pk.Packet, why)
		}
	}
	for _, other := range conflicts[in.r.Role] {
		if in.s.actedAs(in.r.Author.ID, other) {
			in.tr.add("/author/id", "author %q acted as %s in this ledger and cannot act as %s: use a separate identity", in.r.Author.ID, other, in.r.Role)
		}
	}
	switch {
	case in.reviewer != nil:
		in.reviewerTransition(*in.reviewer)
	case in.implementer != nil:
		in.implementerTransition(*in.implementer)
	case in.verifier != nil:
		in.verifierTransition(*in.verifier)
	case in.auditor != nil:
		in.auditorTransition(*in.auditor)
	}
}

// events returns the drafts that accepting the report yields: the report,
// the finding events, the verdict, directives, resolutions, then raised
// escalations by kind and subject.
func (in *ingestion) events(digest, rawDigest string) []Draft {
	in.nextFinding = len(in.s.findings)
	in.add(EventReportAccepted, ReportAccepted{
		Packet: in.pk.Packet, Role: in.pk.Role, Author: in.r.Author, Digest: digest, RawDigest: rawDigest,
		Provenance: Provenance{Mode: ProvenanceDeclared},
	})
	switch {
	case in.reviewer != nil:
		in.reviewerEvents(*in.reviewer)
	case in.implementer != nil:
		in.implementerEvents(*in.implementer)
	case in.verifier != nil:
		in.verifierEvents(*in.verifier)
	case in.auditor != nil:
		in.auditorEvents(*in.auditor)
	}
	for i, e := range in.raised {
		e.ID = seqID("E", len(in.s.escalations)+i+1)
		in.add(EventEscalationRaised, e)
	}
	return in.drafts
}

func (in *ingestion) add(t EventType, data any) {
	in.drafts = append(in.drafts, Draft{Type: t, Data: data})
}

// open adds the opening of a reported finding and returns its identifier.
func (in *ingestion) open(source Role, fi FindingInput) string {
	in.nextFinding++
	id := seqID("F", in.nextFinding)
	if fi.Components == nil {
		fi.Components = []string{}
	}
	in.add(EventFindingOpened, FindingOpened{ID: id, Round: in.pk.Round, Source: source, Packet: in.pk.Packet, Input: fi})
	return id
}

// raise queues an escalation; events numbers and adds them after every
// other event of the report.
func (in *ingestion) raise(kind EscalationKind, reason string, findings, components, requirements []string) {
	in.raised = append(in.raised, EscalationRaised{Kind: kind, Owner: kind.Owner(), Round: in.pk.Round, Packet: in.pk.Packet,
		Reason: reason, Findings: findings, Components: components, Requirements: requirements})
}

// compareIDs orders ledger identifiers that share a prefix by sequence
// number.
func compareIDs(a, b string) int {
	switch {
	case idLess(a, b):
		return -1
	case idLess(b, a):
		return 1
	}
	return 0
}
