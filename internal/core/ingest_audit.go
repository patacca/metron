package core

import (
	"slices"
	"strings"
)

// Ingestion of auditor reports.

func (in *ingestion) checkAuditor(b AuditorBody) {
	resolved := make(map[string]string)
	for i, r := range b.Resolutions {
		p := ptr("body", "resolutions", i)
		if !slices.Contains(in.pk.Escalations, r.Escalation) {
			in.sem.add(p+"/escalation", "escalation %q is not in packet %s: resolve only the escalations it lists", r.Escalation, in.pk.Packet)
		} else if prev, dup := resolved[r.Escalation]; dup {
			in.sem.add(p+"/escalation", "escalation %s is already resolved at %s: resolve each escalation once", r.Escalation, prev)
		} else {
			resolved[r.Escalation] = p
		}
		if blank(r.Analysis) {
			in.sem.add(p+"/analysis", "the analysis is blank: analyse the escalation")
		}
		if blank(r.Conclusion) {
			in.sem.add(p+"/conclusion", "the conclusion is blank: state the conclusion that discharges the escalation")
		}
	}
	for _, id := range in.pk.Escalations {
		if _, ok := resolved[id]; !ok {
			in.sem.add("/body/resolutions", "escalation %s has no resolution: give an analysis and a conclusion for every escalation of the packet", id)
		}
	}
	seen := make(map[string]string)
	for i, a := range b.Adjudications {
		p := ptr("body", "adjudications", i)
		if !slices.Contains(in.pk.Adjudicable, a.Finding) {
			in.sem.add(p+"/finding", "finding %q is not adjudicable in packet %s: decide only on the findings it lists", a.Finding, in.pk.Packet)
		} else if prev, dup := seen[a.Finding]; dup {
			in.sem.add(p+"/finding", "finding %s is already decided at %s: decide each finding once", a.Finding, prev)
		} else {
			seen[a.Finding] = p
		}
		switch a.Decision {
		case DecisionUphold, DecisionWithdraw:
			if a.Tier != "" {
				in.sem.add(p+"/tier", "a tier is given only with %s: remove it", DecisionReclassify)
			}
		case DecisionReclassify:
			if !a.Tier.Valid() {
				in.sem.add(p+"/tier", "%q is not a tier: a reclassification names the new tier", a.Tier)
			} else if a.Tier.Blocking() && in.exists(a.Finding) {
				if lacks := in.lacksToBlock(in.find(a.Finding).Input); len(lacks) > 0 {
					in.sem.add(p+"/tier", "finding %s cannot become %s, because as recorded it does not meet the rules of a blocking finding (%s): uphold it at its tier and say why it should block, so that a reviewer opens a blocking finding against the contract, and recommend a contract change if nothing in the contract covers it",
						a.Finding, a.Tier, strings.Join(lacks, "; "))
				}
			}
		default:
			in.sem.add(p+"/decision", "%q is not %s, %s or %s", a.Decision, DecisionUphold, DecisionWithdraw, DecisionReclassify)
		}
		if blank(a.Rationale) {
			in.sem.add(p+"/rationale", "the rationale is blank: justify the decision")
		}
	}
	for _, id := range in.pk.Adjudicable {
		if _, ok := seen[id]; !ok {
			in.sem.add("/body/adjudications", "finding %s has no decision: uphold, withdraw or reclassify every adjudicable finding", id)
		}
	}
	if b.Suspend != "" && blank(b.Suspend) {
		in.sem.add("/body/suspend", "suspend is blank: say why governance must decide before the campaign continues, or remove the member")
	}
	statements(&in.sem, "/body/directives", b.Directives, "directive")
	statements(&in.sem, "/body/contract_recommendations", b.ContractRecommendations, "recommendation")
}

// lacksToBlock says what a recorded finding lacks to be a blocking finding
// under the pinned contract. An audit changes a finding's tier and nothing
// else, so it can make blocking only what already has all of it.
func (in *ingestion) lacksToBlock(fi FindingInput) []string {
	var lacks []string
	if !in.citesContract(fi) {
		lacks = append(lacks, "it cites neither an applicable requirement nor an unacceptable risk of the pinned contract")
	}
	for _, g := range blockingGaps(fi) {
		what, _, _ := strings.Cut(g.message, ":")
		lacks = append(lacks, what)
	}
	return lacks
}

func (in *ingestion) auditorTransition(b AuditorBody) {
	for _, id := range in.pk.Escalations {
		if j := in.s.escalationIndex(id); j < 0 || !in.s.escalations[j].Open {
			in.tr.add("/packet", "escalation %s is no longer open: request a new packet", id)
		}
	}
	for i, a := range b.Adjudications {
		x := in.find(a.Finding)
		switch {
		case a.Decision == DecisionWithdraw && !x.Status.Unresolved():
			in.tr.add(ptr("body", "adjudications", i, "decision"), "finding %s is %s: only an unresolved finding can be withdrawn", x.ID, x.Status)
		case a.Decision == DecisionReclassify && x.Status == StatusWithdrawn:
			in.tr.add(ptr("body", "adjudications", i, "decision"), "finding %s is withdrawn and cannot be reclassified", x.ID)
		case a.Decision == DecisionReclassify && a.Tier == x.Tier:
			in.tr.add(ptr("body", "adjudications", i, "tier"), "finding %s is already %s: give a different tier or uphold it", x.ID, x.Tier)
		}
	}
}

func (in *ingestion) auditorEvents(b AuditorBody) {
	pk := in.pk
	for _, a := range b.Adjudications {
		switch a.Decision {
		case DecisionWithdraw:
			in.add(EventFindingWithdrawn, FindingWithdrawn{ID: a.Finding, Packet: pk.Packet, Rationale: a.Rationale})
		case DecisionReclassify:
			in.add(EventFindingReclassified, FindingReclassified{ID: a.Finding, Packet: pk.Packet, From: in.find(a.Finding).Tier, To: a.Tier, Rationale: a.Rationale})
		}
	}
	for _, t := range b.Directives {
		in.add(EventDirectiveIssued, DirectiveIssued{Packet: pk.Packet, Text: t})
	}
	ids := slices.Clone(pk.Escalations)
	slices.SortFunc(ids, compareIDs)
	for _, id := range ids {
		for _, r := range b.Resolutions {
			if r.Escalation == id {
				in.add(EventEscalationResolved, EscalationResolved{ID: id, By: RoleAuditor, Actor: in.r.Author.ID, Packet: pk.Packet, Resolution: r.Conclusion})
			}
		}
	}
	in.suspend(b)
}
