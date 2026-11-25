package core

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

// rule is one case of a validation table: a report built against an open
// packet, and the layer and path that must refuse it ("" accepts it).
type rule struct {
	name   string
	prep   func(h *harness)
	body   func(h *harness) any
	report func(r *Report)
	layer  Layer
	path   string
	check  func(t *testing.T, h *harness, drafts []Draft)
}

func runRules(t *testing.T, setup func(*testing.T) (*harness, Packet), author string, rules []rule) {
	t.Helper()
	for _, tc := range rules {
		t.Run(tc.name, func(t *testing.T) {
			h, p := setup(t)
			if tc.prep != nil {
				tc.prep(h)
			}
			r := h.report(p, author, tc.body(h))
			if tc.report != nil {
				tc.report(&r)
			}
			drafts, dup, err := h.ingest(r)
			if tc.layer == "" {
				if err != nil || dup {
					t.Fatalf("refused (duplicate %t): %v", dup, err)
				}
				h.seal(drafts)
				if tc.check != nil {
					tc.check(t, h, drafts)
				}
				return
			}
			var rej *Rejection
			if !errors.As(err, &rej) || rej.Layer != tc.layer || len(drafts) != 0 || dup {
				t.Fatalf("got %d drafts, duplicate %t, error %v; want a %s rejection", len(drafts), dup, err, tc.layer)
			}
			if !hasProblem(rej.Problems, tc.path) {
				t.Fatalf("no problem at %q: %v", tc.path, err)
			}
		})
	}
}

// reviewerSetup leaves F-0001 (T1) resolved, F-0002 (T2) open and the review
// packet of round 2 open.
func reviewerSetup(t *testing.T) (*harness, Packet) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated},
		blocker("crash", "REQ-1"), improvement("long")))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	h.accept(h.packet(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.checks(true)
	return h, h.packet()
}

// withFinding reviews with one finding; a blocking one makes the verdict RED
// and the requirement it cites violated.
func withFinding(h *harness, f FindingInput) ReviewerBody {
	if !f.Tier.Blocking() {
		return review(h.contract, VerdictGreen, nil, f)
	}
	statuses := map[string]string{}
	if f.Requirement != "" {
		statuses[f.Requirement] = RequirementViolated
	}
	return review(h.contract, VerdictRed, statuses, f)
}

func TestReviewerRules(t *testing.T) {
	g := func(mut func(b *ReviewerBody)) func(h *harness) any {
		return func(h *harness) any {
			b := green(h.contract)
			mut(&b)
			return b
		}
	}
	f := func(mut func(f *FindingInput)) func(h *harness) any {
		return func(h *harness) any {
			x := blocker("k", "REQ-1")
			mut(&x)
			return withFinding(h, x)
		}
	}
	reopen := func(rs ...Reopen) func(h *harness) any {
		return func(h *harness) any {
			b := review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated})
			b.Reopen = rs
			return b
		}
	}
	perf := slices.Index(Domains(), DomainPerformance)
	runRules(t, reviewerSetup, "rev-1", []rule{
		{name: "green", body: g(func(b *ReviewerBody) {})},
		{name: "indeterminate", body: func(h *harness) any {
			return review(h.contract, VerdictIndeterminate, map[string]string{"REQ-3": RequirementUnverified})
		}},
		{name: "coverage misses a domain", body: g(func(b *ReviewerBody) { b.Coverage = b.Coverage[1:] }), layer: LayerSemantic, path: "/body/coverage"},
		{name: "domain covered twice", body: g(func(b *ReviewerBody) { b.Coverage = append(b.Coverage, b.Coverage[0]) }), layer: LayerSemantic, path: "/body/coverage/18/domain"},
		{name: "unknown domain", body: g(func(b *ReviewerBody) { b.Coverage[0].Domain = "vibes" }), layer: LayerSemantic, path: "/body/coverage/0/domain"},
		{name: "unknown coverage status", body: g(func(b *ReviewerBody) { b.Coverage[1].Status = "skipped" }), layer: LayerSemantic, path: "/body/coverage/1/status"},
		{name: "not-applicable with an applicable requirement", body: g(func(b *ReviewerBody) { b.Coverage[0].Status = CoverageNotApplicable }), layer: LayerSemantic, path: "/body/coverage/0/status"},
		{name: "not-applicable without a requirement", body: g(func(b *ReviewerBody) { b.Coverage[perf].Status = CoverageNotApplicable })},
		{name: "blank coverage summary", body: g(func(b *ReviewerBody) { b.Coverage[2].Summary = "" }), layer: LayerSemantic, path: "/body/coverage/2/summary"},
		{name: "requirement not assessed", body: g(func(b *ReviewerBody) { b.Requirements = b.Requirements[1:] }), layer: LayerSemantic, path: "/body/requirements"},
		{name: "inapplicable requirement assessed", body: g(func(b *ReviewerBody) {
			b.Requirements = append(b.Requirements, RequirementAssessment{Requirement: "REQ-4", Status: RequirementSatisfied, Evidence: "e"})
		}), layer: LayerSemantic, path: "/body/requirements/3/requirement"},
		{name: "requirement assessed twice", body: g(func(b *ReviewerBody) { b.Requirements = append(b.Requirements, b.Requirements[0]) }), layer: LayerSemantic, path: "/body/requirements/3/requirement"},
		{name: "unknown requirement status", body: g(func(b *ReviewerBody) { b.Requirements[0].Status = "fine" }), layer: LayerSemantic, path: "/body/requirements/0/status"},
		{name: "blank requirement evidence", body: g(func(b *ReviewerBody) { b.Requirements[0].Evidence = " " }), layer: LayerSemantic, path: "/body/requirements/0/evidence"},
		{name: "violated requirement not cited", body: func(h *harness) any {
			return review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated, "REQ-2": RequirementViolated}, blocker("k", "REQ-1"))
		}, layer: LayerSemantic, path: "/body/requirements/1/status"},
		{name: "verdict not implied", body: g(func(b *ReviewerBody) { b.Verdict = VerdictRed }), layer: LayerSemantic, path: "/body/verdict"},
		{name: "unverified requirement with GREEN", body: func(h *harness) any {
			return review(h.contract, VerdictGreen, map[string]string{"REQ-2": RequirementUnverified})
		}, layer: LayerSemantic, path: "/body/verdict"},
		{name: "unknown verdict", body: g(func(b *ReviewerBody) { b.Verdict = "AMBER" }), layer: LayerSemantic, path: "/body/verdict"},
		{name: "blank justification", body: g(func(b *ReviewerBody) { b.Justification = "" }), layer: LayerSemantic, path: "/body/justification"},
		{name: "blank residual risk", body: g(func(b *ReviewerBody) { b.ResidualRisks = []string{""} }), layer: LayerSemantic, path: "/body/residual_risks/0"},
		{name: "blocking finding citing a requirement", body: f(func(x *FindingInput) {})},
		{name: "cited requirement marked satisfied", body: func(h *harness) any {
			return review(h.contract, VerdictRed, nil, blocker("k", "REQ-1"))
		}, layer: LayerSemantic, path: "/body/requirements/0/status"},
		{name: "cited requirement marked unverified", body: func(h *harness) any {
			return review(h.contract, VerdictRed, map[string]string{"REQ-2": RequirementUnverified, "REQ-1": RequirementViolated}, blocker("a", "REQ-1"), blocker("b", "REQ-2"))
		}, layer: LayerSemantic, path: "/body/requirements/1/status"},
		{name: "requirement of a reopened finding marked satisfied", body: func(h *harness) any {
			b := review(h.contract, VerdictRed, nil)
			b.Reopen = []Reopen{{Finding: "F-0001", Reason: ReopenRegression, Justification: "j"}}
			return b
		}, layer: LayerSemantic, path: "/body/requirements/0/status"},
		{name: "blocking finding citing a risk", body: f(func(x *FindingInput) { x.Requirement, x.Risk = "", "RISK-1" })},
		{name: "blocking finding citing nothing", body: f(func(x *FindingInput) { x.Requirement = "" }), layer: LayerSemantic, path: "/body/findings/0"},
		{name: "inapplicable requirement cited", body: f(func(x *FindingInput) { x.Requirement = "REQ-4" }), layer: LayerSemantic, path: "/body/findings/0/requirement"},
		{name: "unknown risk cited", body: f(func(x *FindingInput) { x.Requirement, x.Risk = "", "RISK-9" }), layer: LayerSemantic, path: "/body/findings/0/risk"},
		{name: "accepted risk cited", body: f(func(x *FindingInput) { x.Requirement, x.Risk = "", "ACC-1" }), layer: LayerSemantic, path: "/body/findings/0/risk"},
		{name: "blank blocking rationale", body: f(func(x *FindingInput) { x.Risk, x.Requirement, x.BlockingRationale = "RISK-1", "", "" }), layer: LayerSemantic, path: "/body/findings/0/blocking_rationale"},
		{name: "blank verification", body: f(func(x *FindingInput) { x.Risk, x.Requirement, x.Verification = "RISK-1", "", "" }), layer: LayerSemantic, path: "/body/findings/0/verification"},
		{name: "proportionality missing", body: f(func(x *FindingInput) { x.Risk, x.Requirement, x.Domain = "RISK-1", "", DomainPerformance }), layer: LayerSemantic, path: "/body/findings/0/proportionality"},
		{name: "proportionality incomplete", body: f(func(x *FindingInput) {
			x.Risk, x.Requirement, x.Domain = "RISK-1", "", DomainPerformance
			x.Proportionality = &Proportionality{ProposedChange: "cache it", DeferralCost: "rewrite later"}
		}), layer: LayerSemantic, path: "/body/findings/0/proportionality/regression_risk"},
		{name: "proportionality complete", body: f(func(x *FindingInput) {
			x.Risk, x.Requirement, x.Domain = "RISK-1", "", DomainPerformance
			x.Proportionality = &Proportionality{ProposedChange: "cache it", RegressionRisk: "stale reads", DeferralCost: "rewrite later"}
		})},
		{name: "T2 without benefit", body: func(h *harness) any {
			x := improvement("k")
			x.ExpectedBenefit = ""
			return withFinding(h, x)
		}, layer: LayerSemantic, path: "/body/findings/0/expected_benefit"},
		{name: "T3 without reassessment", body: func(h *harness) any {
			x := improvement("k")
			x.Tier, x.EvidenceNeeded = TierCalibration, "load data"
			return withFinding(h, x)
		}, layer: LayerSemantic, path: "/body/findings/0/reassess_when"},
		{name: "T4", body: func(h *harness) any {
			x := improvement("k")
			x.Tier = TierPreference
			return withFinding(h, x)
		}},
		{name: "unknown tier", body: f(func(x *FindingInput) { x.Tier = "T9" }), layer: LayerSemantic, path: "/body/findings/0/tier"},
		{name: "unknown finding domain", body: f(func(x *FindingInput) { x.Domain = "vibes" }), layer: LayerSemantic, path: "/body/findings/0/domain"},
		{name: "blank title", body: f(func(x *FindingInput) { x.Title = "" }), layer: LayerSemantic, path: "/body/findings/0/title"},
		{name: "blank component", body: f(func(x *FindingInput) { x.Components = []string{""} }), layer: LayerSemantic, path: "/body/findings/0/components/0"},
		{name: "no component", body: f(func(x *FindingInput) { x.Components = nil }), check: func(t *testing.T, h *harness, d []Draft) {
			if c := d[1].Data.(FindingOpened).Input.Components; c == nil {
				t.Fatal("a missing component list is written as null")
			}
		}},
		{name: "duplicate keys", body: func(h *harness) any {
			return review(h.contract, VerdictGreen, nil, improvement("k"), improvement("k"))
		}, layer: LayerSemantic, path: "/body/findings/1/key"},
		{name: "blank key", body: func(h *harness) any { return withFinding(h, improvement("")) }, layer: LayerSemantic, path: "/body/findings/0/key"},
		{name: "related to an existing finding", body: func(h *harness) any {
			x := improvement("k")
			x.Related = []string{"F-0001"}
			return withFinding(h, x)
		}},
		{name: "related to an unknown finding", body: func(h *harness) any {
			x := improvement("k")
			x.Related = []string{"F-0001", "F-0042"}
			return withFinding(h, x)
		}, layer: LayerSemantic, path: "/body/findings/0/related/1"},
		{name: "introduced by an unknown finding", body: func(h *harness) any {
			x := improvement("k")
			x.IntroducedBy = "F-0042"
			return withFinding(h, x)
		}, layer: LayerSemantic, path: "/body/findings/0/introduced_by"},
		{name: "contradicts an unknown finding", body: func(h *harness) any {
			x := improvement("k")
			x.Contradicts = "F-0042"
			return withFinding(h, x)
		}, layer: LayerSemantic, path: "/body/findings/0/contradicts"},
		{name: "contradicts a resolved finding", body: func(h *harness) any {
			x := improvement("k")
			x.Contradicts = "F-0001"
			return withFinding(h, x)
		}, check: func(t *testing.T, h *harness, d []Draft) {
			if k := raisedKinds(d); !slices.Equal(k, []EscalationKind{EscalationContradiction}) {
				t.Fatalf("raised %v", k)
			}
			if a := h.next(); a.Step != StepAudit {
				t.Fatalf("next %s", a.Step)
			}
		}},
		{name: "reopen a resolved finding", body: reopen(Reopen{Finding: "F-0001", Reason: ReopenRegression, Justification: "it crashes again"}),
			check: func(t *testing.T, h *harness, d []Draft) {
				eqTypes(t, d, EventReportAccepted, EventFindingReopened, EventVerdictRecorded)
				if x, _ := lookup(h.state, "F-0001"); x.Status != StatusOpen || x.Recurrences != 1 {
					t.Fatalf("finding %+v", x)
				}
			}},
		{name: "reopen implies RED", body: func(h *harness) any {
			b := green(h.contract)
			b.Reopen = []Reopen{{Finding: "F-0001", Reason: ReopenNewEvidence, Justification: "j"}}
			return b
		}, layer: LayerSemantic, path: "/body/verdict"},
		{name: "reopen an open finding", body: func(h *harness) any {
			b := green(h.contract)
			b.Reopen = []Reopen{{Finding: "F-0002", Reason: ReopenNewEvidence, Justification: "j"}}
			return b
		}, layer: LayerTransition, path: "/body/reopen/0/finding"},
		{name: "reopen an unknown finding", body: reopen(Reopen{Finding: "F-0042", Reason: ReopenRegression, Justification: "j"}), layer: LayerSemantic, path: "/body/reopen/0/finding"},
		{name: "reopen twice", body: reopen(
			Reopen{Finding: "F-0001", Reason: ReopenRegression, Justification: "j"},
			Reopen{Finding: "F-0001", Reason: ReopenNewEvidence, Justification: "j"},
		), layer: LayerSemantic, path: "/body/reopen/1/finding"},
		{name: "unknown reopen reason", body: reopen(Reopen{Finding: "F-0001", Reason: "because", Justification: "j"}), layer: LayerSemantic, path: "/body/reopen/0/reason"},
		{name: "blank reopen justification", body: reopen(Reopen{Finding: "F-0001", Reason: ReopenRegression}), layer: LayerSemantic, path: "/body/reopen/0/justification"},
		{name: "contract gap", body: g(func(b *ReviewerBody) {
			b.ContractGaps = []ContractGap{{Title: "no rate limit", Description: "a flood exhausts memory"}}
		}), check: func(t *testing.T, h *harness, d []Draft) {
			if k := raisedKinds(d); !slices.Equal(k, []EscalationKind{EscalationContractGap}) {
				t.Fatalf("raised %v", k)
			}
			if a := h.next(); a.Step != StepAdjudicate || h.gate().Open {
				t.Fatalf("next %s with the gap open", a.Step)
			}
		}},
		{name: "blank contract gap", body: g(func(b *ReviewerBody) { b.ContractGaps = []ContractGap{{Description: "d"}} }), layer: LayerSemantic, path: "/body/contract_gaps/0/title"},
		{name: "wrong schema", body: g(func(b *ReviewerBody) {}), report: func(r *Report) { r.Schema = "metron.report/v0" }, layer: LayerSemantic, path: "/schema"},
		{name: "blank author", body: g(func(b *ReviewerBody) {}), report: func(r *Report) { r.Author.ID = "" }, layer: LayerSemantic, path: "/author/id"},
		{name: "unknown author kind", body: g(func(b *ReviewerBody) {}), report: func(r *Report) { r.Author.Kind = "robot" }, layer: LayerSemantic, path: "/author/kind"},
		{name: "wrong role", body: g(func(b *ReviewerBody) {}), report: func(r *Report) { r.Role = RoleVerifier }, layer: LayerTransition, path: "/role"},
		{name: "unknown role", body: g(func(b *ReviewerBody) {}), report: func(r *Report) { r.Role = "critic" }, layer: LayerSemantic, path: "/role"},
		{name: "unknown body member", body: g(func(b *ReviewerBody) {}), report: func(r *Report) { r.Body = json.RawMessage(`{"verdict":"GREEN","mood":"good"}`) }, layer: LayerSemantic, path: "/body"},
		{name: "body not an object", body: g(func(b *ReviewerBody) {}), report: func(r *Report) { r.Body = json.RawMessage(`[]`) }, layer: LayerSemantic, path: "/body"},
		{name: "unknown packet", body: g(func(b *ReviewerBody) {}), report: func(r *Report) { r.Packet = "P-0099" }, layer: LayerTransition, path: "/packet"},
		{name: "candidate changed", prep: func(h *harness) { h.commit(true) }, body: g(func(b *ReviewerBody) {}), layer: LayerTransition, path: "/packet"},
		{name: "tree dirty", prep: func(h *harness) { h.clean = false }, body: g(func(b *ReviewerBody) {}), layer: LayerTransition, path: "/packet"},
		{name: "ledger-only commit", prep: func(h *harness) { h.commit(false) }, body: g(func(b *ReviewerBody) {})},
	})
}

// implementerSetup leaves the fix packet open with F-0001 (T1) as subject and
// F-0002 (T2) as backlog.
func implementerSetup(t *testing.T) (*harness, Packet) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated},
		blocker("crash", "REQ-1"), improvement("long")))
	p := h.packet()
	if !slices.Equal(p.Subjects, []string{"F-0001"}) || !slices.Equal(p.Backlog, []string{"F-0002"}) {
		t.Fatalf("fix packet subjects %v backlog %v", p.Subjects, p.Backlog)
	}
	return h, p
}

func TestImplementerRules(t *testing.T) {
	fixes := func(mut func(h *harness, c string) ImplementerBody) func(h *harness) any {
		return func(h *harness) any { return mut(h, h.commit(true)) }
	}
	one := func(mut func(x *Fix)) func(h *harness) any {
		return fixes(func(h *harness, c string) ImplementerBody {
			x := fix("F-0001", c)
			mut(&x)
			return ImplementerBody{Fixes: []Fix{x}}
		})
	}
	dispute := Dispute{Finding: "F-0001", Rationale: "outside the envelope"}
	runRules(t, implementerSetup, "impl-1", []rule{
		{name: "fix the subject", body: one(func(x *Fix) {}), check: func(t *testing.T, h *harness, d []Draft) {
			eqTypes(t, d, EventReportAccepted, EventFindingFixClaimed)
		}},
		{name: "fix the subject and the backlog", body: fixes(func(h *harness, c string) ImplementerBody {
			return ImplementerBody{Fixes: []Fix{fix("F-0001", c), fix("F-0002", c)}}
		}), check: func(t *testing.T, h *harness, d []Draft) {
			if a := h.next(); !slices.Equal(a.Findings, []string{"F-0001", "F-0002"}) {
				t.Fatalf("verify %+v", a)
			}
		}},
		{name: "dispute only, without a commit", body: func(h *harness) any { return ImplementerBody{Disputes: []Dispute{dispute}} }},
		{name: "subject not addressed", body: fixes(func(h *harness, c string) ImplementerBody {
			return ImplementerBody{Fixes: []Fix{fix("F-0002", c)}}
		}), layer: LayerSemantic, path: "/body"},
		{name: "unknown finding fixed", body: fixes(func(h *harness, c string) ImplementerBody {
			return ImplementerBody{Fixes: []Fix{fix("F-0001", c), fix("F-0042", c)}}
		}), layer: LayerSemantic, path: "/body/fixes/1/finding"},
		{name: "fixed twice", body: fixes(func(h *harness, c string) ImplementerBody {
			return ImplementerBody{Fixes: []Fix{fix("F-0001", c), fix("F-0001", c)}}
		}), layer: LayerSemantic, path: "/body/fixes/1/finding"},
		{name: "fixed and disputed", body: fixes(func(h *harness, c string) ImplementerBody {
			return ImplementerBody{Fixes: []Fix{fix("F-0001", c)}, Disputes: []Dispute{dispute}}
		}), layer: LayerSemantic, path: "/body/disputes/0/finding"},
		{name: "backlog disputed", body: fixes(func(h *harness, c string) ImplementerBody {
			return ImplementerBody{Fixes: []Fix{fix("F-0001", c)}, Disputes: []Dispute{{Finding: "F-0002", Rationale: "r"}}}
		}), layer: LayerSemantic, path: "/body/disputes/0/finding"},
		{name: "blank dispute rationale", body: func(h *harness) any {
			return ImplementerBody{Disputes: []Dispute{{Finding: "F-0001"}}}
		}, layer: LayerSemantic, path: "/body/disputes/0/rationale"},
		{name: "no commit", body: one(func(x *Fix) { x.Commits = nil }), layer: LayerSemantic, path: "/body/fixes/0/commits"},
		{name: "abbreviated commit", body: one(func(x *Fix) { x.Commits[0] = x.Commits[0][:12] }), layer: LayerSemantic, path: "/body/fixes/0/commits/0"},
		{name: "commit listed twice", body: one(func(x *Fix) { x.Commits = append(x.Commits, x.Commits[0]) }), layer: LayerSemantic, path: "/body/fixes/0/commits/1"},
		{name: "no component", body: one(func(x *Fix) { x.Components = nil }), layer: LayerSemantic, path: "/body/fixes/0/components"},
		{name: "blank component", body: one(func(x *Fix) { x.Components = []string{" "} }), layer: LayerSemantic, path: "/body/fixes/0/components/0"},
		{name: "blank summary", body: one(func(x *Fix) { x.Summary = "" }), layer: LayerSemantic, path: "/body/fixes/0/summary"},
		{name: "commit not in the candidate's history", body: one(func(x *Fix) { x.Commits = []string{commitHash(999)} }), layer: LayerTransition, path: "/body/fixes/0/commits/0"},
		{name: "commit already in the round's candidate", body: one(func(x *Fix) { x.Commits = append(x.Commits, commitHash(0)) }), layer: LayerTransition, path: "/body/fixes/0/commits/1"},
		{name: "round's candidate rewritten away", body: one(func(x *Fix) { x.Commits = append(x.Commits, commitHash(0)) }), prep: func(h *harness) {
			h.amend = func(f *Facts) {
				for i := range f.Commits {
					f.Commits[i].InBase = false // the base commit no longer exists
				}
			}
		}},
		{name: "commit not examined", body: one(func(x *Fix) {}), prep: func(h *harness) {
			h.amend = func(f *Facts) { f.Commits = nil }
		}, layer: LayerTransition, path: "/body/fixes/0/commits/0"},
		{name: "commit touching only .metron", body: func(h *harness) any {
			h.commit(true)
			return ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(false))}}
		}, layer: LayerTransition, path: "/body/fixes/0/commits/0"},
		{name: "candidate unchanged", body: func(h *harness) any {
			return ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(false))}}
		}, layer: LayerTransition, path: "/body/fixes/0"},
		{name: "tree dirty at submission", body: func(h *harness) any {
			c := h.commit(true)
			h.clean = false
			return ImplementerBody{Fixes: []Fix{fix("F-0001", c)}}
		}, layer: LayerTransition, path: ""},
	})
}

// verifierSetup leaves the verify packet open with the claimed fix of F-0001
// as subject and F-0002 open.
func verifierSetup(t *testing.T) (*harness, Packet) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated},
		blocker("crash", "REQ-1"), improvement("long")))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	return h, h.packet()
}

func TestVerifierRules(t *testing.T) {
	v := func(mut func(b *VerifierBody)) func(h *harness) any {
		return func(h *harness) any {
			b := verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001")
			mut(&b)
			return b
		}
	}
	reg := func(mut func(x *FindingInput)) func(h *harness) any {
		return v(func(b *VerifierBody) {
			x := blocker("leak", "REQ-1")
			x.IntroducedBy = "F-0001"
			mut(&x)
			b.Regressions = []FindingInput{x}
		})
	}
	runRules(t, verifierSetup, "ver-1", []rule{
		{name: "confirm", body: v(func(b *VerifierBody) {})},
		{name: "reject", body: v(func(b *VerifierBody) { b.Verifications[0].Result = VerificationRejected }), check: func(t *testing.T, h *harness, d []Draft) {
			if x, _ := lookup(h.state, "F-0001"); x.Status != StatusOpen || x.Recurrences != 1 {
				t.Fatalf("finding %+v", x)
			}
		}},
		{name: "verification missing", body: v(func(b *VerifierBody) { b.Verifications = nil }), layer: LayerSemantic, path: "/body/verifications"},
		{name: "unclaimed finding verified", body: v(func(b *VerifierBody) {
			b.Verifications = append(b.Verifications, Verification{Finding: "F-0002", Result: VerificationConfirmed, Evidence: "e"})
		}), layer: LayerSemantic, path: "/body/verifications/1/finding"},
		{name: "verified twice", body: v(func(b *VerifierBody) { b.Verifications = append(b.Verifications, b.Verifications[0]) }), layer: LayerSemantic, path: "/body/verifications/1/finding"},
		{name: "unknown result", body: v(func(b *VerifierBody) { b.Verifications[0].Result = "maybe" }), layer: LayerSemantic, path: "/body/verifications/0/result"},
		{name: "blank evidence", body: v(func(b *VerifierBody) { b.Verifications[0].Evidence = "" }), layer: LayerSemantic, path: "/body/verifications/0/evidence"},
		{name: "regression", body: reg(func(x *FindingInput) {})},
		{name: "regression without origin", body: reg(func(x *FindingInput) { x.IntroducedBy = "" }), layer: LayerSemantic, path: "/body/regressions/0/introduced_by"},
		{name: "regression from a finding not verified here", body: reg(func(x *FindingInput) { x.IntroducedBy = "F-0002" }), layer: LayerSemantic, path: "/body/regressions/0/introduced_by"},
		{name: "regression breaking the tier rules", body: reg(func(x *FindingInput) { x.BlockingRationale = "" }), layer: LayerSemantic, path: "/body/regressions/0/blocking_rationale"},
		{name: "candidate changed", prep: func(h *harness) { h.commit(true) }, body: v(func(b *VerifierBody) {}), layer: LayerTransition, path: "/packet"},
	})
}

// auditorSetup leaves the audit packet open for the dispute of F-0001, with
// F-0002 (T2) open.
func auditorSetup(t *testing.T) (*harness, Packet) {
	h := newHarness(t)
	h.checks(true)
	h.accept(h.packet(), "rev-1", review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated},
		blocker("crash", "REQ-1"), improvement("long")))
	h.accept(h.packet(), "impl-1", ImplementerBody{Disputes: []Dispute{{Finding: "F-0001", Rationale: "outside the envelope"}}})
	return h, h.packet()
}

func TestAuditorRules(t *testing.T) {
	a := func(decision string, tier Tier, mut func(b *AuditorBody)) func(h *harness) any {
		return func(h *harness) any {
			b := AuditorBody{Resolutions: []EscalationResolution{{Escalation: "E-0001", Analysis: "a", Conclusion: "c"}},
				Adjudications: []Adjudication{{Finding: "F-0001", Decision: decision, Tier: tier, Rationale: "r"}}}
			mut(&b)
			return b
		}
	}
	nop := func(b *AuditorBody) {}
	runRules(t, auditorSetup, "aud-1", []rule{
		{name: "uphold", body: a(DecisionUphold, "", func(b *AuditorBody) { b.Directives = []string{"test empty input"} }),
			check: func(t *testing.T, h *harness, d []Draft) {
				eqTypes(t, d, EventReportAccepted, EventDirectiveIssued, EventEscalationResolved)
			}},
		{name: "withdraw", body: a(DecisionWithdraw, "", nop), check: func(t *testing.T, h *harness, d []Draft) {
			eqTypes(t, d, EventReportAccepted, EventFindingWithdrawn, EventEscalationResolved)
		}},
		{name: "reclassify", body: a(DecisionReclassify, TierImprovement, nop), check: func(t *testing.T, h *harness, d []Draft) {
			if x, _ := lookup(h.state, "F-0001"); x.Tier != TierImprovement || x.Input.Tier != TierMaterial {
				t.Fatalf("finding %+v", x)
			}
		}},
		{name: "adjudication missing", body: a(DecisionUphold, "", func(b *AuditorBody) { b.Adjudications = nil }), layer: LayerSemantic, path: "/body/adjudications"},
		{name: "finding not adjudicable", body: a(DecisionUphold, "", func(b *AuditorBody) {
			b.Adjudications = append(b.Adjudications, Adjudication{Finding: "F-0002", Decision: DecisionUphold, Rationale: "r"})
		}), layer: LayerSemantic, path: "/body/adjudications/1/finding"},
		{name: "decided twice", body: a(DecisionUphold, "", func(b *AuditorBody) { b.Adjudications = append(b.Adjudications, b.Adjudications[0]) }), layer: LayerSemantic, path: "/body/adjudications/1/finding"},
		{name: "reclassify without tier", body: a(DecisionReclassify, "", nop), layer: LayerSemantic, path: "/body/adjudications/0/tier"},
		{name: "uphold with tier", body: a(DecisionUphold, TierImprovement, nop), layer: LayerSemantic, path: "/body/adjudications/0/tier"},
		{name: "unknown decision", body: a("defer", "", nop), layer: LayerSemantic, path: "/body/adjudications/0/decision"},
		{name: "blank rationale", body: a(DecisionUphold, "", func(b *AuditorBody) { b.Adjudications[0].Rationale = "" }), layer: LayerSemantic, path: "/body/adjudications/0/rationale"},
		{name: "reclassify to the same tier", body: a(DecisionReclassify, TierMaterial, nop), layer: LayerTransition, path: "/body/adjudications/0/tier"},
		{name: "resolution missing", body: a(DecisionUphold, "", func(b *AuditorBody) { b.Resolutions = nil }), layer: LayerSemantic, path: "/body/resolutions"},
		{name: "escalation not in the packet", body: a(DecisionUphold, "", func(b *AuditorBody) {
			b.Resolutions = append(b.Resolutions, EscalationResolution{Escalation: "E-0042", Analysis: "a", Conclusion: "c"})
		}), layer: LayerSemantic, path: "/body/resolutions/1/escalation"},
		{name: "escalation resolved twice", body: a(DecisionUphold, "", func(b *AuditorBody) { b.Resolutions = append(b.Resolutions, b.Resolutions[0]) }), layer: LayerSemantic, path: "/body/resolutions/1/escalation"},
		{name: "blank analysis", body: a(DecisionUphold, "", func(b *AuditorBody) { b.Resolutions[0].Analysis = "" }), layer: LayerSemantic, path: "/body/resolutions/0/analysis"},
		{name: "conclusion of Unicode spaces", body: a(DecisionUphold, "", func(b *AuditorBody) { b.Resolutions[0].Conclusion = "\u00a0\u2003\u3000" }), layer: LayerSemantic, path: "/body/resolutions/0/conclusion"},
		{name: "analysis of invisible format characters", body: a(DecisionUphold, "", func(b *AuditorBody) { b.Resolutions[0].Analysis = "\u200b\ufeff\u2060" }), layer: LayerSemantic, path: "/body/resolutions/0/analysis"},
		{name: "blank directive", body: a(DecisionUphold, "", func(b *AuditorBody) { b.Directives = []string{""} }), layer: LayerSemantic, path: "/body/directives/0"},
		{name: "blank recommendation", body: a(DecisionUphold, "", func(b *AuditorBody) { b.ContractRecommendations = []string{""} }), layer: LayerSemantic, path: "/body/contract_recommendations/0"},
	})
}

// TestIngestEventOrder checks the order of section 7 on a reviewer report
// that exercises every kind of event a review yields.
func TestIngestEventOrder(t *testing.T) {
	h := newHarness(t)
	c := testContract()
	c.Convergence = &Convergence{RootCauseAfter: 1, ConvergenceAuditAfter: 2, ProcessAuditAfter: 4, SuspendAfter: 20}
	h.contract = c
	h.must(Pin(h.state, h.facts(), "faster escalation", "gov"))
	h.checks(true)
	h.accept(h.packet(), "rev-1", red(h.contract, "crash"))
	h.accept(h.packet(), "impl-1", ImplementerBody{Fixes: []Fix{fix("F-0001", h.commit(true))}})
	h.accept(h.packet(), "ver-1", verify(map[string]string{"F-0001": VerificationConfirmed}, "F-0001"))
	h.checks(true)

	contradicting := improvement("b")
	contradicting.Contradicts = "F-0001"
	b := review(h.contract, VerdictRed, map[string]string{"REQ-1": RequirementViolated}, blocker("a", "REQ-1"), contradicting)
	b.Reopen = []Reopen{{Finding: "F-0001", Reason: ReopenRegression, Justification: "it is back"}}
	b.ContractGaps = []ContractGap{{Title: "t1", Description: "d1"}, {Title: "t0", Description: "d0"}}
	d := h.accept(h.packet(), "rev-1", b)
	eqTypes(t, d, EventReportAccepted, EventFindingReopened, EventFindingOpened, EventFindingOpened, EventVerdictRecorded,
		EventEscalationRaised, EventEscalationRaised, EventEscalationRaised, EventEscalationRaised)
	if !d[1].Data.(FindingReopened).RootCauseRequired {
		t.Fatal("the reopening reached the threshold but root_cause_required is false")
	}
	if ids := []string{d[2].Data.(FindingOpened).ID, d[3].Data.(FindingOpened).ID}; !slices.Equal(ids, []string{"F-0002", "F-0003"}) {
		t.Fatalf("finding ids %v", ids)
	}
	want := []EscalationKind{EscalationContradiction, EscalationNonConvergence, EscalationContractGap, EscalationContractGap}
	if k := raisedKinds(d); !slices.Equal(k, want) {
		t.Fatalf("raised %v, want %v", k, want)
	}
	for i, x := range d[5:] {
		e := x.Data.(EscalationRaised)
		if wantID := seqID("E", i+1); e.ID != wantID || e.Round != 2 {
			t.Fatalf("escalation %d: %+v", i, e)
		}
	}
	if e := d[7].Data.(EscalationRaised); e.Reason != "t1: d1" {
		t.Fatalf("contract gaps are not in report order: %q", e.Reason)
	}
	if e := d[5].Data.(EscalationRaised); !slices.Equal(e.Findings, []string{"F-0003"}) {
		t.Fatalf("contradiction subjects %v", e.Findings)
	}
}

func TestReopenNeedsACurrentCitation(t *testing.T) {
	h, _ := reviewerSetup(t)
	amended := testContract()
	amended.Requirements = append([]Requirement{{ID: "REQ-5", Domain: DomainFunctionalCorrectness,
		Statement: "valid and empty input parse", Criterion: "the tests pass", Command: "go test ./..."}}, amended.Requirements[1:]...)
	h.contract = amended
	h.must(Pin(h.state, h.facts(), "replace REQ-1", "gov"))
	h.checks(true)
	p := h.packet()
	b := review(h.contract, VerdictRed, nil)
	b.Reopen = []Reopen{{Finding: "F-0001", Reason: ReopenRequirementChanged, Justification: "still crashes"}}
	if ps := h.refused(p, "rev-1", b, LayerSemantic); !hasProblem(ps, "/body/reopen/0/finding") {
		t.Fatalf("problems %+v", ps)
	}
	b = review(h.contract, VerdictRed, map[string]string{"REQ-5": RequirementViolated}, blocker("again", "REQ-5"))
	h.accept(p, "rev-1", b)
}

// TestReportPacketDigest checks that a report is bound to the packet the
// ledger issued, not only to its identifier.
func TestReportPacketDigest(t *testing.T) {
	h := newHarness(t)
	h.checks(true)
	first := h.packet()
	accepted := h.report(first, "rev-1", red(h.contract, "crash"))
	drafts, _, err := h.ingest(accepted)
	if err != nil {
		t.Fatal(err)
	}
	h.seal(drafts)
	fp := h.packet()
	c := h.commit(true)
	body := ImplementerBody{Fixes: []Fix{fix("F-0001", c)}}
	for _, tc := range []struct {
		name   string
		digest string
	}{
		{"the digest of another packet", accepted.PacketDigest},
		{"no digest", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := h.report(fp, "impl-1", body)
			r.PacketDigest = tc.digest
			d, dup, err := h.ingest(r)
			rejection(t, d, err, LayerTransition, "/packet_digest")
			if dup {
				t.Fatal("refused report reported as a duplicate")
			}
			if tc.digest == "" && !strings.Contains(err.Error(), "from the report skeleton, from metron next --json or from the note metron packet prints") {
				t.Fatalf("the message does not say where the digest comes from: %v", err)
			}
		})
	}
	t.Run("a duplicate whose digest was changed", func(t *testing.T) {
		r := accepted
		r.PacketDigest = digestOf("elsewhere")
		d, dup, err := h.ingest(r)
		rejection(t, d, err, LayerTransition, "/packet_digest")
		if dup {
			t.Fatal("refused report reported as a duplicate")
		}
		if _, dup, err := h.ingest(accepted); !dup || err != nil {
			t.Fatalf("the true duplicate: %t, %v", dup, err)
		}
	})
	t.Run("a report from another ledger", func(t *testing.T) {
		a, b := newHarness(t), newHarness(t)
		other := testContract()
		other.Authorizes = "another repository"
		b.contract = other
		b.must(Pin(b.state, b.facts(), "its own contract", "gov"))
		for _, x := range []*harness{a, b} {
			x.checks(true)
		}
		pa, pb := a.packet(), b.packet()
		if pa.ID != pb.ID {
			t.Fatalf("identifiers %s and %s", pa.ID, pb.ID)
		}
		foreign := a.report(pa, "rev-1", green(a.contract))
		d, _, err := b.ingest(foreign)
		rejection(t, d, err, LayerTransition, "/packet_digest")
		if !strings.Contains(err.Error(), "another ledger") {
			t.Fatalf("%v", err)
		}
		b.accept(pb, "rev-1", green(b.contract))
	})
}

func TestBlank(t *testing.T) {
	for _, tc := range []struct {
		text  string
		blank bool
	}{
		{"", true},
		{" \t\n", true},
		{"\u00a0", true},
		{"\u200b", true},
		{"\ufeff", true},
		{"\u2060", true},
		{"\u00a0\u200b \ufeff\u2060\u3000", true},
		{"a", false},
		{"\u200bok\u200b", false},
		{"\u00e9", false},
		{"\u2014", false},
	} {
		if got := blank(tc.text); got != tc.blank {
			t.Errorf("blank(%+q) = %t, want %t", tc.text, got, tc.blank)
		}
	}
}
